package node

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"github.com/idena-network/idena-go/api"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	util "github.com/idena-network/idena-go/common/ulimit"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/consensus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/ceremony"
	"github.com/idena-network/idena-go/core/flip"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/core/profile"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/keystore"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/pengings"
	"github.com/idena-network/idena-go/protocol"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/rpc"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/pkg/errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/tendermint/tm-db"
)

type Node struct {
	config          *config.Config
	blockchain      *blockchain.Blockchain
	appState        *appstate.AppState
	secStore        *secstore.SecStore
	pm              *protocol.IdenaGossipHandler
	stop            chan struct{}
	proposals       *pengings.Proposals
	votes           *pengings.Votes
	consensusEngine *consensus.Engine
	txpool          *mempool.TxPool
	flipKeyPool     *mempool.KeysPool
	rpcAPIs         []rpc.API
	httpListener    net.Listener // HTTP RPC listener socket to server API requests
	httpHandler     *rpc.Server  // HTTP RPC request handler to process the API requests
	log             log.Logger
	keyStore        *keystore.KeyStore
	fp              *flip.Flipper
	ipfsProxy       ipfs.Proxy
	bus             eventbus.Bus
	ceremony        *ceremony.ValidationCeremony
	downloader      *protocol.Downloader
	offlineDetector *blockchain.OfflineDetector
	appVersion      string
	profileManager  *profile.Manager
}

type NodeCtx struct {
	Node            *Node
	AppState        *appstate.AppState
	Ceremony        *ceremony.ValidationCeremony
	Blockchain      *blockchain.Blockchain
	Flipper         *flip.Flipper
	KeysPool        *mempool.KeysPool
	OfflineDetector *blockchain.OfflineDetector
	ProofsByRound   *sync.Map
	PendingProofs   *sync.Map
}

func StartMobileNode(path string, cfg string) string {
	fileHandler, _ := log.FileHandler(filepath.Join(path, "output.log"), log.TerminalFormat(false))
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.MultiHandler(log.StreamHandler(os.Stdout, log.LogfmtFormat()), fileHandler)))

	c, err := config.MakeMobileConfig(path, cfg)

	if err != nil {
		return err.Error()
	}

	n, err := NewNode(c, "mobile")

	if err != nil {
		return err.Error()
	}

	n.Start()

	return "started"
}

func ProvideMobileKey(path string, cfg string, key string, password string) string {
	c, err := config.MakeMobileConfig(path, cfg)

	if err != nil {
		return err.Error()
	}

	if err := c.ProvideNodeKey(key, password, false); err != nil {
		return err.Error()
	}
	return "done"
}

func NewNode(config *config.Config, appVersion string) (*Node, error) {
	nodeCtx, err := NewNodeWithInjections(config, eventbus.New(), collector.NewStatsCollector(), appVersion)
	if err != nil {
		return nil, err
	}
	return nodeCtx.Node, err
}

func NewNodeWithInjections(config *config.Config, bus eventbus.Bus, statsCollector collector.StatsCollector, appVersion string) (*NodeCtx, error) {

	db, err := OpenDatabase(config.DataDir, "idenachain", 16, 16)

	if err != nil {
		return nil, err
	}

	keyStoreDir, err := config.KeyStoreDataDir()
	if err != nil {
		return nil, err
	}

	err = config.SetApiKey()
	if err != nil {
		return nil, errors.Wrap(err, "cannot set API key")
	}

	ipfsProxy, err := ipfs.NewIpfsProxy(config.IpfsConf, bus)
	if err != nil {
		return nil, err
	}

	keyStore := keystore.NewKeyStore(keyStoreDir, keystore.StandardScryptN, keystore.StandardScryptP)
	secStore := secstore.NewSecStore()
	appState := appstate.NewAppState(db, bus)

	offlineDetector := blockchain.NewOfflineDetector(config, db, appState, secStore, bus)
	votes := pengings.NewVotes(appState, bus, offlineDetector)

	txpool := mempool.NewTxPool(appState, bus, config.Mempool, config.Consensus.MinFeePerByte)
	flipKeyPool := mempool.NewKeysPool(db, appState, bus, secStore)

	chain := blockchain.NewBlockchain(config, db, txpool, appState, ipfsProxy, secStore, bus, offlineDetector, keyStore)
	proposals, proofsByRound, pendingProofs := pengings.NewProposals(chain, appState, offlineDetector)
	flipper := flip.NewFlipper(db, ipfsProxy, flipKeyPool, txpool, secStore, appState, bus)
	pm := protocol.NewIdenaGossipHandler(ipfsProxy.Host(), config.P2P, chain, proposals, votes, txpool, flipper, bus, flipKeyPool, appVersion)
	sm := state.NewSnapshotManager(db, appState.State, bus, ipfsProxy, config)
	downloader := protocol.NewDownloader(pm, config, chain, ipfsProxy, appState, sm, bus, secStore, statsCollector)
	consensusEngine := consensus.NewEngine(chain, pm, proposals, config.Consensus, appState, votes, txpool, secStore,
		downloader, offlineDetector, statsCollector)
	ceremony := ceremony.NewValidationCeremony(appState, bus, flipper, secStore, db, txpool, chain, downloader, flipKeyPool, config)
	profileManager := profile.NewProfileManager(ipfsProxy)
	node := &Node{
		config:          config,
		blockchain:      chain,
		pm:              pm,
		proposals:       proposals,
		appState:        appState,
		consensusEngine: consensusEngine,
		txpool:          txpool,
		log:             log.New(),
		keyStore:        keyStore,
		fp:              flipper,
		ipfsProxy:       ipfsProxy,
		secStore:        secStore,
		bus:             bus,
		flipKeyPool:     flipKeyPool,
		ceremony:        ceremony,
		downloader:      downloader,
		offlineDetector: offlineDetector,
		votes:           votes,
		appVersion:      appVersion,
		profileManager:  profileManager,
	}
	return &NodeCtx{
		Node:            node,
		AppState:        appState,
		Ceremony:        ceremony,
		Blockchain:      chain,
		Flipper:         flipper,
		KeysPool:        flipKeyPool,
		OfflineDetector: offlineDetector,
		ProofsByRound:   proofsByRound,
		PendingProofs:   pendingProofs,
	}, nil
}

func (node *Node) Start() {
	node.StartWithHeight(0)
}

func (node *Node) StartWithHeight(height uint64) {
	node.secStore.AddKey(crypto.FromECDSA(node.config.NodeKey()))

	if changed, value, err := util.ManageFdLimit(); changed {
		node.log.Info("Set new fd limit", "value", value)
	} else if err != nil {
		node.log.Warn("Failed to set new fd limit", "err", err)
	}

	if err := node.blockchain.InitializeChain(); err != nil {
		node.log.Error("Cannot initialize blockchain", "error", err.Error())
		return
	}

	if err := node.appState.Initialize(node.blockchain.Head.Height()); err != nil {
		if err := node.appState.Initialize(0); err != nil {
			node.log.Error("Cannot initialize state", "error", err.Error())
		}
	}

	if err := node.blockchain.EnsureIntegrity(); err != nil {
		node.log.Error("Failed to recover blockchain", "err", err)
		return
	}

	if height > 0 && node.blockchain.Head.Height() > height {
		if err := node.blockchain.ResetTo(height); err != nil {
			node.log.Error(fmt.Sprintf("Cannot reset blockchain to %d", height), "error", err.Error())
			return
		}
	}

	node.txpool.Initialize(node.blockchain.Head, node.secStore.GetAddress())
	node.flipKeyPool.Initialize(node.blockchain.Head)
	node.votes.Initialize(node.blockchain.Head)
	node.fp.Initialize()
	node.ceremony.Initialize(node.blockchain.GetBlock(node.blockchain.Head.Hash()))
	node.blockchain.ProvideApplyNewEpochFunc(node.ceremony.ApplyNewEpoch)
	node.offlineDetector.Start(node.blockchain.Head)
	node.consensusEngine.Start()
	node.pm.Start()

	// Configure RPC
	if err := node.startRPC(); err != nil {
		node.log.Error("Cannot start RPC endpoint", "error", err.Error())
	}
}

func (node *Node) WaitForStop() {
	<-node.stop
	node.secStore.Destroy()
}

// startRPC is a helper method to start all the various RPC endpoint during node
// startup. It's not meant to be called at any time afterwards as it makes certain
// assumptions about the state of the node.
func (node *Node) startRPC() error {
	// Gather all the possible APIs to surface
	apis := node.apis()

	if err := node.startHTTP(node.config.RPC.HTTPEndpoint(), apis, node.config.RPC.HTTPModules, node.config.RPC.HTTPCors, node.config.RPC.HTTPVirtualHosts, node.config.RPC.HTTPTimeouts, node.config.RPC.APIKey); err != nil {
		return err
	}

	node.rpcAPIs = apis
	return nil
}

// startHTTP initializes and starts the HTTP RPC endpoint.
func (node *Node) startHTTP(endpoint string, apis []rpc.API, modules []string, cors []string, vhosts []string, timeouts rpc.HTTPTimeouts, apiKey string) error {
	// Short circuit if the HTTP endpoint isn't being exposed
	if endpoint == "" {
		return nil
	}
	listener, handler, err := rpc.StartHTTPEndpoint(endpoint, apis, modules, cors, vhosts, timeouts, apiKey)
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

func OpenDatabase(datadir string, name string, cache int, handles int) (db.DB, error) {
	return db.NewGoLevelDBWithOpts(name, datadir, &opt.Options{
		OpenFilesCacheCapacity: handles,
		BlockCacheCapacity:     cache / 2 * opt.MiB,
		WriteBuffer:            cache / 4 * opt.MiB,
		Filter:                 filter.NewBloomFilter(10),
	})
}

// apis returns the collection of RPC descriptors this node offers.
func (node *Node) apis() []rpc.API {

	baseApi := api.NewBaseApi(node.consensusEngine, node.txpool, node.keyStore, node.secStore)

	return []rpc.API{
		{
			Namespace: "net",
			Version:   "1.0",
			Service:   api.NewNetApi(node.pm, node.ipfsProxy),
			Public:    true,
		},
		{
			Namespace: "dna",
			Version:   "1.0",
			Service:   api.NewDnaApi(baseApi, node.blockchain, node.ceremony, node.appVersion, node.profileManager),
			Public:    true,
		},
		{
			Namespace: "account",
			Version:   "1.0",
			Service:   api.NewAccountApi(baseApi),
			Public:    true,
		},
		{
			Namespace: "flip",
			Version:   "1.0",
			Service:   api.NewFlipApi(baseApi, node.fp, node.ipfsProxy, node.ceremony),
			Public:    true,
		},
		{
			Namespace: "bcn",
			Version:   "1.0",
			Service:   api.NewBlockchainApi(baseApi, node.blockchain, node.ipfsProxy, node.txpool, node.downloader, node.pm),
			Public:    true,
		},
	}
}

func (node *Node) generateSyntheticP2PKey() *ecdsa.PrivateKey {
	hash := common.Hash(rlp.Hash([]byte("node-p2p-key")))
	sig := node.secStore.Sign(hash.Bytes())
	p2pKey, _ := crypto.GenerateKeyFromSeed(bytes.NewReader(sig))
	return p2pKey
}
