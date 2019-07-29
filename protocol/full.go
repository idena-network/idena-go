package protocol

import (
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/pkg/errors"
	"time"
)

const FullSyncBatchSize = 200

var (
	BlockCertIsMissing = errors.New("block cert is missing")
)

type fullSync struct {
	pm                   *ProtocolManager
	log                  log.Logger
	chain                *blockchain.Blockchain
	batches              chan *batch
	ipfs                 ipfs.Proxy
	isSyncing            bool
	appState             *appstate.AppState
	potentialForkedPeers mapset.Set
	deferredHeaders      []*block
}

func (fs *fullSync) batchSize() uint64 {
	return FullSyncBatchSize
}

func NewFullSync(pm *ProtocolManager, log log.Logger,
	chain *blockchain.Blockchain,
	ipfs ipfs.Proxy,
	appState *appstate.AppState,
	potentialForkedPeers mapset.Set) *fullSync {

	return &fullSync{
		appState:             appState,
		log:                  log,
		potentialForkedPeers: potentialForkedPeers,
		chain:                chain,
		batches:              make(chan *batch, 10),
		pm:                   pm,
		isSyncing:            true,
		ipfs:                 ipfs,
	}
}

func (fs *fullSync) requestBatch(from, to uint64, ignoredPeer string) *batch {
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

func (fs *fullSync) applyDeferredBlocks(checkState *appstate.AppState) (uint64, error) {
	defer func() {
		fs.deferredHeaders = []*block{}
	}()

	for _, b := range fs.deferredHeaders {
		if block, err := fs.GetBlock(b.Header); err != nil {
			fs.log.Error("fail to retrieve block", "err", err)
			return b.Header.Height(), err
		} else {
			if err := fs.chain.AddBlock(block, checkState); err != nil {
				if err := fs.appState.ResetTo(fs.chain.Head.Height()); err != nil {
					return block.Height(), err
				}

				fs.log.Warn(fmt.Sprintf("Block %v is invalid: %v", block.Height(), err))
				time.Sleep(time.Second)
				// TODO: ban bad peer
				return block.Height(), err
			}
			if !b.Cert.Empty() {
				fs.chain.WriteCertificate(block.Hash(), b.Cert, true)
			}
			if checkState.Commit(block) != nil {
				return block.Height(), err
			}
		}
	}
	return 0, nil
}

func (fs *fullSync) postConsuming() error {
	if len(fs.deferredHeaders) > 0 {
		fs.log.Warn(fmt.Sprintf("All blocks was consumed but last headers have not been added to chain"))
	}
	return nil
}

func (fs *fullSync) processBatch(batch *batch, attemptNum int) error {
	fs.log.Info("Start process batch", "from", batch.from, "to", batch.to)
	if attemptNum > MaxAttemptsCountPerBatch {
		return errors.New("number of attempts exceeded limit")
	}

	checkState, _ := fs.appState.ForCheckWithNewCache(fs.chain.Head.Height())

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
				fs.log.Error("Block header is invalid", "err", err)
				return reload(i)
			}
			fs.deferredHeaders = append(fs.deferredHeaders, block)
			if block.Cert != nil && block.Cert.Len() > 0 {
				if from, err := fs.applyDeferredBlocks(checkState); err != nil {
					return reload(from)
				}
			}
		case <-timeout:
			fs.log.Warn("process batch - timeout was reached")
			return reload(i)
		}
	}
	fs.log.Info("Finish process batch", "from", batch.from, "to", batch.to)
	return nil
}

func (fs *fullSync) preConsuming(head *types.Header) (uint64, error) {
	return head.Height() + 1, nil
}

func (fs *fullSync) validateHeader(block *block) error {
	prevBlock := fs.chain.Head
	if len(fs.deferredHeaders) > 0 {
		prevBlock = fs.deferredHeaders[len(fs.deferredHeaders)-1].Header
	}

	err := fs.chain.ValidateHeader(block.Header, prevBlock)
	if err != nil {
		return err
	}

	if block.Header.Flags().HasFlag(types.IdentityUpdate) {
		if block.Cert.Empty() {
			return BlockCertIsMissing
		}
	}
	if !block.Cert.Empty() {
		return fs.chain.ValidateBlockCert(prevBlock, block.Header, block.Cert, fs.appState.ValidatorsCache)
	}

	return nil
}

func (fs *fullSync) GetBlock(header *types.Header) (*types.Block, error) {
	if header.EmptyBlockHeader != nil {
		return &types.Block{
			Header: header,
			Body:   &types.Body{},
		}, nil
	}
	if txs, err := fs.ipfs.Get(header.ProposedHeader.IpfsHash); err != nil {
		return nil, err
	} else {
		if len(txs) > 0 {
			fs.log.Debug("Retrieve block body from ipfs", "hash", header.Hash().Hex())
		}
		body := &types.Body{}
		body.FromBytes(txs)
		return &types.Block{
			Header: header,
			Body:   body,
		}, nil
	}
}

func (fs *fullSync) PeekBlocks(fromBlock, toBlock uint64, peers []string) chan *types.Block {
	var batches []*batch
	blocks := make(chan *types.Block, len(peers))

	for _, peerId := range peers {
		if err, batch := fs.pm.GetBlocksRange(peerId, fromBlock, toBlock); err != nil {
			continue
		} else {
			batches = append(batches, batch)
		}
	}

	go func() {
		for _, batch := range batches {
			for i := batch.from; i <= batch.to; i++ {
				timeout := time.After(time.Second * 10)
				select {
				case header := <-batch.headers:
					if block, err := fs.GetBlock(header.Header); err != nil {
						fs.log.Warn("fail to retrieve block while peeking", "err", err)
						continue
					} else {
						blocks <- block
					}
				case <-timeout:
					fs.log.Warn("timeout was reached while peeking block")
					continue
				}
			}
		}
		close(blocks)
	}()

	return blocks
}
