package protocol

import (
	"errors"
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"time"
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

func (fs *fullSync) Load(toHeight uint64) {
	term := make(chan interface{})
	completed := make(chan interface{})
	go fs.consumeBlocks(term, completed)
	head := fs.chain.Head
	from := head.Height() + 1
	knownHeights := fs.pm.GetKnownHeights()
loop:
	for from <= toHeight {
		for peer, height := range knownHeights {
			if height < from {
				continue
			}
			to := math.Min(from+BatchSize, height)
			if err, batch := fs.pm.GetBlocksRange(peer, from, to); err != nil {
				continue
			} else {
				select {
				case fs.batches <- batch:
				case <-term:
					break loop
				}
			}
			from = to + 1
		}
	}
	fs.log.Info(fmt.Sprintf("All blocks was requested. Wait applying of blocks"))
	close(completed)
	<-term
}

func (fs *fullSync) consumeBlocks(term chan interface{}, completed chan interface{}) {
	defer close(term)
	for {
		timeout := time.After(time.Second * 15)

		select {
		case batch := <-fs.batches:
			if err := fs.processBatch(batch, 1); err != nil {
				fs.log.Warn("failed to process batch", "err", err)
				return
			}
			continue
		default:
		}

		select {
		case batch := <-fs.batches:
			if err := fs.processBatch(batch, 1); err != nil {
				fs.log.Warn("failed to process batch", "err", err)
				return
			}
			break
		case <-completed:
			return
		case <-timeout:
			return
		}
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

func (fs *fullSync) processBatch(batch *batch, attemptNum int) error {
	fs.log.Info("Start process batch", "from", batch.from, "to", batch.to)
	if attemptNum > MaxAttemptsCountPerBatch {
		return errors.New("number of attempts exceeded limit")
	}

	checkState, _ := fs.appState.ForCheckWithNewCache(fs.chain.Head.Height())

	for i := batch.from; i <= batch.to; i++ {
		timeout := time.After(time.Second * 10)

		reload := func() error {
			b := fs.requestBatch(i, batch.to, batch.p.id)
			if b == nil {
				return errors.New(fmt.Sprintf("Batch (%v-%v) can't be loaded", i, batch.to))
			}
			return fs.processBatch(b, attemptNum+1)
		}

		select {
		case block := <-batch.headers:
			if block, err := fs.GetBlock(block.header); err != nil {
				fs.log.Error("fail to retrieve block", "err", err)
				return reload()
			} else {
				if err := fs.chain.AddBlock(block, checkState); err != nil {
					fs.appState.ResetTo(fs.chain.Head.Height())

					if err == blockchain.ParentHashIsInvalid {
						fs.potentialForkedPeers.Add(batch.p.id)
					}
					fs.log.Warn(fmt.Sprintf("Block from peer %v is invalid: %v", batch.p.id, err))
					time.Sleep(time.Second)
					// TODO: ban bad peer
					return reload()
				}
				checkState.Commit(block)
			}
		case <-timeout:
			fs.log.Warn("process batch - timeout was reached")
			return reload()
		}
	}
	fs.log.Info("Finish process batch", "from", batch.from, "to", batch.to)
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
					if block, err := fs.GetBlock(header.header); err != nil {
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
