package node

import (
	"crypto/ecdsa"
	"idena-go/blockchain"
	"idena-go/config"
	"idena-go/consensus"
	"idena-go/idenadb"
	"idena-go/p2p"
	"idena-go/pengings"
	"idena-go/protocol"
)

type Node struct {
	config     *config.Config
	blockchain *blockchain.Blockchain
	key        *ecdsa.PrivateKey
	pm         *protocol.ProtocolManager
	stop       chan struct{}
	proposals  *pengings.Proposals
}

func NewNode(config *config.Config) (*Node, error) {

	db, err := OpenDatabase(config, "idenachain", 16, 16)

	if err != nil {
		return nil, err
	}

	chain := blockchain.NewBlockchain(config, db)
	proposals := pengings.NewProposals(chain)
	pm := protocol.NetProtocolManager(chain, proposals)

	return &Node{
		config:     config,
		blockchain: chain,
		pm:         pm,
		proposals:  proposals,
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
		},
	}
	srv := &p2p.Server{
		Config: *config,
	}
	node.blockchain.InitializeChain(node.key)

	consensusEngine := consensus.NewEngine(node.blockchain, node.pm, node.proposals, node.config.Consensus, node.key)
	consensusEngine.Start()
	srv.Start()
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
