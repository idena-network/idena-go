package protocol

import (
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/core/validators"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/pkg/errors"
	"time"
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
	preliminaryHead      *types.Header
	stateDb              *state.IdentityStateDB
	validators           *validators.ValidatorsCache
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

func (fs *fastSync) createPreliminaryCopy() (*state.IdentityStateDB, error) {

}

func (fs *fastSync) dropPreliminaries() {

}

func (fs *fastSync) loadValidators() {
	fs.validators = validators.NewValidatorsCache(fs.stateDb, fs.appState.State.GodAddress())
	fs.validators.Load()
}

func (fs *fastSync) preConsuming(head *types.Header) (from uint64, err error) {
	fs.preliminaryHead = fs.chain.ReadPreliminaryHead()
	if fs.preliminaryHead == nil {
		fs.preliminaryHead = head
		fs.stateDb, err = fs.createPreliminaryCopy()
		from = head.Height() + 1
		fs.loadValidators()
		return from, err
	}
	fs.stateDb, err = fs.appState.IdentityState.LoadPreliminary(fs.preliminaryHead.Height())
	if err != nil {
		fs.dropPreliminaries()
		return fs.preConsuming(head)
	}
	fs.loadValidators()
	from = fs.preliminaryHead.Height() + 1
	return from, nil
}

func (fs *fastSync) processBatch(batch *batch, attemptNum int) error {
	if fs.manifest == nil {
		panic("manifest is required")
	}
	fs.log.Info("Start process batch", "from", batch.from, "to", batch.to)
	if attemptNum > MaxAttemptsCountPerBatch {
		return errors.New("number of attempts exceeded limit")
	}
	reload := func(from uint64) error {
		b := fs.requestBatch(from, batch.to, batch.p.id)
		if b == nil {
			return errors.New(fmt.Sprintf("Batch (%v-%v) can't be loaded", from, batch.to))
		}
		return fs.processBatch(b, attemptNum+1)
	}

	for i := batch.from; i <= batch.to; i++ {
		timeout := time.After(time.Second * 10)

		select {
		case block := <-batch.headers:
			if err := fs.validateHeader(block); err != nil {
				if err == blockchain.ParentHashIsInvalid {
					fs.potentialForkedPeers.Add(batch.p.id)
					return err
				}
				return reload(i)
			}

			if fs.chain.AddHeader(block.Header) != nil {
				reload(i)
			}


		case <-timeout:
			fs.log.Warn("process batch - timeout was reached")
			return reload(i)
		}
	}
	fs.log.Info("Finish process batch", "from", batch.from, "to", batch.to)
	return nil
}

func (fs *fastSync) validateIdentityState(block *block) error {

}

func (fs *fastSync) validateHeader(block *block) error {
	prevBlock := fs.preliminaryHead

	err := fs.chain.ValidateHeader(block.Header, prevBlock)
	if err != nil {
		return err
	}

	if block.Header.Flags().HasFlag(types.IdentityUpdate) {
		if len(block.Cert) == 0 {
			return BlockCertIsMissing
		}
	}
	if len(block.Cert) > 0 {
		return fs.chain.ValidateBlockCert(prevBlock, block.Header, block.Cert, fs.validators)
	}

	return nil
}

func (fs *fastSync) requestBatch(from, to uint64, ignoredPeer string) *batch {
	knownHeights := fs.pm.GetKnownHeights()
	if knownHeights == nil {
		return nil
	}
	for peerId, height := range knownHeights {
		if (peerId != ignoredPeer || len(knownHeights) == 1) && height >= to {
			if err, batch := fs.pm.GetBlocksRange(peerId, from, to); err != nil {
				continue
			} else {
				return batch
			}
		}
	}
	return nil
}

func (fs *fastSync) postConsuming() {
	panic("implement me")
}
