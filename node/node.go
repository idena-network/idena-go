package node

import (
	"crypto/ecdsa"
	"idena-go/blockchain"
	"idena-go/config"
	"idena-go/consensus"
	"idena-go/core/mempool"
	"idena-go/core/state"
	"idena-go/core/validators"
	"idena-go/idenadb"
	"idena-go/p2p"
	"idena-go/pengings"
	"idena-go/protocol"
)

type Node struct {
	config          *config.Config
	blockchain      *blockchain.Blockchain
	key             *ecdsa.PrivateKey
	pm              *protocol.ProtocolManager
	stop            chan struct{}
	proposals       *pengings.Proposals
	state           state.Database
	votes           *pengings.Votes
	consensusEngine *consensus.Engine
	txpool          *mempool.TxPool
}

func NewNode(config *config.Config) (*Node, error) {

	db, err := OpenDatabase(config, "idenachain", 16, 16)

	if err != nil {
		return nil, err
	}

	validators := validators.NewValidatorsSet(db)
	votes := pengings.NewVotes()
	txpool := mempool.NewTxPool(validators)
	state := state.NewDatabase(db)
	chain := blockchain.NewBlockchain(config, db, txpool, validators)
	proposals := pengings.NewProposals(chain)
	pm := protocol.NetProtocolManager(chain, proposals, votes, txpool)
	consensusEngine := consensus.NewEngine(chain, pm, proposals, config.Consensus, state, validators, votes, txpool)

	return &Node{
		config:          config,
		blockchain:      chain,
		pm:              pm,
		proposals:       proposals,
		state:           state,
		consensusEngine: consensusEngine,
		txpool:          txpool,
	}, nil
}

func (node *Node) Start() {
	node.key = node.config.NodeKey()

	config := node.config.P2P
	config.PrivateKey = node.key
	config.Protocols = [] p2p.Protocol{
		{
			Name:    "AppName",
			Version: 1,
			Run:     node.pm.HandleNewPeer,
			Length:  35,
		},
	}
	srv := &p2p.Server{
		Config: *config,
	}
	node.blockchain.InitializeChain(node.key)
	node.consensusEngine.SetKey(node.key)
	node.consensusEngine.Start()
	srv.Start()
	node.pm.Start()
}

func (node *Node) Wait() {
	<-node.stop
}

func OpenDatabase(c *config.Config, name string, cache int, handles int) (idenadb.Database, error) {
	db, err := idenadb.NewLDBDatabase(c.ResolvePath(name), cache, handles)
	if err != nil {
		return nil, err
	}
	return db, nil
}
