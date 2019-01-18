package node

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/api"
	"idena-go/blockchain"
	"idena-go/config"
	"idena-go/consensus"
	"idena-go/core/appstate"
	"idena-go/core/mempool"
	"idena-go/core/state"
	"idena-go/keystore"
	"idena-go/log"
	"idena-go/p2p"
	"idena-go/pengings"
	"idena-go/protocol"
	"idena-go/rpc"
	"net"
	"os"
	"strings"
)

type Node struct {
	config          *config.Config
	blockchain      *blockchain.Blockchain
	appState        *appstate.AppState
	key             *ecdsa.PrivateKey
	pm              *protocol.ProtocolManager
	stop            chan struct{}
	proposals       *pengings.Proposals
	votes           *pengings.Votes
	consensusEngine *consensus.Engine
	txpool          *mempool.TxPool
	rpcAPIs         []rpc.API
	httpListener    net.Listener // HTTP RPC listener socket to server API requests
	httpHandler     *rpc.Server  // HTTP RPC request handler to process the API requests
	log             log.Logger
	srv             *p2p.Server
	keyStore        *keystore.KeyStore
}

func StartDefaultNode(path string) string {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stdout, log.LogfmtFormat())))

	c := config.GetDefaultConfig(
		path,
		config.DefaultPort,
		false,
		config.DefaultRpcHost,
		config.DefaultRpcPort,
		config.DefaultBootnode)

	n, err := NewNode(c)

	if err != nil {
		return err.Error()
	}

	n.Start()

	return "done"
}

func NewNode(config *config.Config) (*Node, error) {

	db, err := OpenDatabase(config, "idenachain", 16, 16)

	if err != nil {
		return nil, err
	}

	stateDb := state.NewLazy(db)

	keyStoreDir, err := config.KeyStoreDataDir()
	if err != nil {
		return nil, err
	}

	keyStore := keystore.NewKeyStore(keyStoreDir, keystore.StandardScryptN, keystore.StandardScryptP)

	appState := appstate.NewAppState(stateDb)
	votes := pengings.NewVotes(appState)

	txpool := mempool.NewTxPool(appState)
	chain := blockchain.NewBlockchain(config, db, txpool, appState)
	proposals := pengings.NewProposals(chain)
	pm := protocol.NetProtocolManager(chain, proposals, votes, txpool)
	consensusEngine := consensus.NewEngine(chain, pm, proposals, config.Consensus, appState, votes, txpool)
	return &Node{
		config:          config,
		blockchain:      chain,
		pm:              pm,
		proposals:       proposals,
		appState:        appState,
		consensusEngine: consensusEngine,
		txpool:          txpool,
		log:             log.New(),
		keyStore:        keyStore,
	}, nil
}

func (node *Node) Start() {
	node.key = node.config.NodeKey()

	config := node.config.P2P
	config.PrivateKey = node.key
	config.Protocols = []p2p.Protocol{
		{
			Name:    "AppName",
			Version: 1,
			Run:     node.pm.HandleNewPeer,
			Length:  35,
		},
	}
	node.srv = &p2p.Server{
		Config: *config,
	}
	if err := node.blockchain.InitializeChain(node.key); err != nil {
		node.log.Error("Cannot initialize blockchain", "error", err.Error())
		return
	}
	node.appState.Initialize(node.blockchain.Head.Height())

	node.consensusEngine.SetKey(node.key)
	node.consensusEngine.Start()
	node.srv.Start()
	node.pm.Start()

	// Configure RPC
	if err := node.startRPC(); err != nil {
		node.log.Error("Cannot start RPC endpoint", "error", err.Error())
	}
}

func (node *Node) WaitForStop() {
	<-node.stop
}

// startRPC is a helper method to start all the various RPC endpoint during node
// startup. It's not meant to be called at any time afterwards as it makes certain
// assumptions about the state of the node.
func (node *Node) startRPC() error {
	// Gather all the possible APIs to surface
	apis := node.apis()

	if err := node.startHTTP(node.config.RPC.HTTPEndpoint(), apis, node.config.RPC.HTTPModules, node.config.RPC.HTTPCors, node.config.RPC.HTTPVirtualHosts, node.config.RPC.HTTPTimeouts); err != nil {
		return err
	}

	node.rpcAPIs = apis
	return nil
}

// startHTTP initializes and starts the HTTP RPC endpoint.
func (node *Node) startHTTP(endpoint string, apis []rpc.API, modules []string, cors []string, vhosts []string, timeouts rpc.HTTPTimeouts) error {
	// Short circuit if the HTTP endpoint isn't being exposed
	if endpoint == "" {
		return nil
	}
	listener, handler, err := rpc.StartHTTPEndpoint(endpoint, apis, modules, cors, vhosts, timeouts)
	if err != nil {
		return err
	}
	node.log.Info("HTTP endpoint opened", "url", fmt.Sprintf("http://%s", endpoint), "cors", strings.Join(cors, ","), "vhosts", strings.Join(vhosts, ","))

	node.httpListener = listener
	node.httpHandler = handler

	return nil
}

// stopHTTP terminates the HTTP RPC endpoint.
func (node *Node) stopHTTP() {
	if node.httpListener != nil {
		node.httpListener.Close()
		node.httpListener = nil

		node.log.Info("HTTP endpoint closed", "url", fmt.Sprintf("http://%s", node.config.RPC.HTTPEndpoint()))
	}
	if node.httpHandler != nil {
		node.httpHandler.Stop()
		node.httpHandler = nil
	}
}

func OpenDatabase(c *config.Config, name string, cache int, handles int) (db.DB, error) {
	return db.NewGoLevelDBWithOpts(name, c.DataDir, &opt.Options{
		OpenFilesCacheCapacity: handles,
		BlockCacheCapacity:     cache / 2 * opt.MiB,
		WriteBuffer:            cache / 4 * opt.MiB,
		Filter:                 filter.NewBloomFilter(10),
	})
}

// apis returns the collection of RPC descriptors this node offers.
func (node *Node) apis() []rpc.API {
	return []rpc.API{
		{
			Namespace: "net",
			Version:   "1.0",
			Service:   api.NewNetApi(node.pm, node.srv),
			Public:    true,
		},
		{
			Namespace: "dna",
			Version:   "1.0",
			Service:   api.NewDnaApi(node.consensusEngine, node.blockchain, node.txpool, node.keyStore),
			Public:    true,
		},
		{
			Namespace: "account",
			Version:   "1.0",
			Service:   api.NewAccountApi(node.keyStore),
			Public:    true,
		},
	}
}
