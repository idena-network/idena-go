package protocol

import (
	"github.com/deckarep/golang-set"
	"idena-go/blockchain"
	"idena-go/core/appstate"
	"idena-go/core/state/snapshot"
	"idena-go/ipfs"
	"idena-go/log"
)

type fastSync struct {
	pm                   *ProtocolManager
	log                  log.Logger
	chain                *blockchain.Blockchain
	batches              chan *batch
	ipfs                 ipfs.Proxy
	isSyncing            bool
	appState             *appstate.AppState
	potentialForkedPeers mapset.Set
	manifest             *snapshot.Manifest
}

func (fastSync) Load(toHeight uint64) {
	panic("implement me")
}

func NewFastSync(pm *ProtocolManager, log log.Logger,
	chain *blockchain.Blockchain,
	ipfs ipfs.Proxy,
	appState *appstate.AppState,
	potentialForkedPeers mapset.Set,
	manifest *snapshot.Manifest) *fastSync {

	return &fastSync{
		appState:             appState,
		log:                  log,
		potentialForkedPeers: potentialForkedPeers,
		chain:                chain,
		batches:              make(chan *batch, 10),
		pm:                   pm,
		isSyncing:            true,
		ipfs:                 ipfs,
		manifest:             manifest,
	}
}
