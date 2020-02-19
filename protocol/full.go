package protocol

import (
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	"time"
)

const FullSyncBatchSize = 200
const FlushToDiskLastStates = 200

var (
	BlockCertIsMissing = errors.New("block cert is missing")
)

type fullSync struct {
	pm                   *IdenaGossipHandler
	log                  log.Logger
	chain                *blockchain.Blockchain
	batches              chan *batch
	ipfs                 ipfs.Proxy
	isSyncing            bool
	appState             *appstate.AppState
	potentialForkedPeers mapset.Set
	deferredHeaders      []blockPeer
	targetHeight         uint64
	statsCollector       collector.StatsCollector
}

func (fs *fullSync) batchSize() uint64 {
	return FullSyncBatchSize
}

func NewFullSync(
	pm *IdenaGossipHandler,
	log log.Logger,
	chain *blockchain.Blockchain,
	ipfs ipfs.Proxy,
	appState *appstate.AppState,
	potentialForkedPeers mapset.Set,
	targetHeight uint64,
	statsCollector collector.StatsCollector,
) *fullSync {

	return &fullSync{
		appState:             appState,
		log:                  log,
		potentialForkedPeers: potentialForkedPeers,
		chain:                chain,
		batches:              make(chan *batch, 10),
		pm:                   pm,
		isSyncing:            true,
		ipfs:                 ipfs,
		targetHeight:         targetHeight,
		statsCollector:       statsCollector,
	}
}

func (fs *fullSync) requestBatch(from, to uint64, ignoredPeer peer.ID) *batch {
	knownHeights := fs.pm.GetKnownHeights()
	if knownHeights == nil {
		return nil
	}
	for peerId, height := range knownHeights {
		if (peerId != ignoredPeer || len(knownHeights) == 1) && height >= to {
			if batch, err := fs.pm.GetBlocksRange(peerId, from, to); err != nil {
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
		fs.deferredHeaders = []blockPeer{}
	}()

	for _, b := range fs.deferredHeaders {
		if block, err := fs.GetBlock(b.Header); err != nil {
			fs.log.Error("fail to retrieve block", "err", err)
			return b.Header.Height(), err
		} else {

			/*if fs.targetHeight-b.Header.Height() <= FlushToDiskLastStates {
				if err := fs.appState.UseDefaultTree(); err != nil {
					return block.Height(), errors.Wrap(err, "cannot switch state tree to defaults")
				}
			}*/
			if err := fs.chain.AddBlock(block, checkState, fs.statsCollector); err != nil {
				if err := fs.appState.ResetTo(fs.chain.Head.Height()); err != nil {
					return block.Height(), err
				}
				if errors.Cause(err) != blockchain.BlockInsertionErr {
					fs.pm.BanPeer(b.peerId, err)
				}
				fs.log.Warn(fmt.Sprintf("Block %v is invalid: %v", block.Height(), err))
				time.Sleep(time.Second)
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

func (fs *fullSync) preConsuming(head *types.Header) (uint64, error) {
	/*if fs.targetHeight-head.Height() > FlushToDiskLastStates {
		fs.log.Info("switch sync tree to fast version")
		if err := fs.appState.UseSyncTree(); err != nil {
			return 0, errors.Wrap(err, "cannot switch state tree to sync tree")
		}
	}*/
	return head.Height() + 1, nil
}

func (fs *fullSync) postConsuming() error {
	/*if err := fs.appState.UseDefaultTree(); err != nil {
		return err
	}*/
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

	checkState, err := fs.appState.ForCheckWithNewCache(fs.chain.Head.Height())
	if err != nil {
		return err
	}

	reload := func(from uint64) error {
		b := fs.requestBatch(from, batch.to, batch.p.id)
		if b == nil {
			return errors.New(fmt.Sprintf("Batch (%v-%v) can't be loaded", from, batch.to))
		}
		return fs.processBatch(b, attemptNum+1)
	}

	for i := batch.from; i <= batch.to; i++ {
		timeout := time.After(time.Second * 20)

		select {
		case block := <-batch.headers:
			if block == nil {
				err := errors.New("failed to load block header")
				fs.pm.BanPeer(batch.p.id, err)
				return err
			}
			batch.p.resetTimeouts()
			if err := fs.validateHeader(block, batch.p); err != nil {
				if err == blockchain.ParentHashIsInvalid {
					fs.potentialForkedPeers.Add(batch.p.id)
					return err
				}
				fs.pm.BanPeer(batch.p.id, err)
				fs.log.Error("Block header is invalid", "err", err)
				return reload(i)
			}
			fs.deferredHeaders = append(fs.deferredHeaders, blockPeer{*block, batch.p.id})
			if block.Cert != nil && block.Cert.Len() > 0 {
				if from, err := fs.applyDeferredBlocks(checkState); err != nil {
					return reload(from)
				}
			}
		case <-timeout:
			fs.log.Warn("process batch - timeout was reached", "peer", batch.p.id)
			if batch.p.addTimeout() {
				fs.pm.BanPeer(batch.p.id, BanReasonTimeout)
			}
			return reload(i)
		}
	}
	fs.log.Info("Finish process batch", "from", batch.from, "to", batch.to)
	return nil
}

func (fs *fullSync) validateHeader(block *block, p *protoPeer) error {
	prevBlock := fs.chain.Head
	if len(fs.deferredHeaders) > 0 {
		prevBlock = fs.deferredHeaders[len(fs.deferredHeaders)-1].Header
	}

	err := fs.chain.ValidateHeader(block.Header, prevBlock)
	if err != nil {
		return err
	}

	if block.Header.Flags().HasFlag(types.IdentityUpdate|types.Snapshot) || block.Header.Height() == p.knownHeight {
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

func (fs *fullSync) SeekBlocks(fromBlock, toBlock uint64, peers []peer.ID) chan *types.BlockBundle {
	var batches []*batch
	blocks := make(chan *types.BlockBundle, len(peers))

	for _, peerId := range peers {
		if batch, err := fs.pm.GetBlocksRange(peerId, fromBlock, toBlock); err != nil {
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
				case header, ok := <-batch.headers:
					if !ok || header == nil {
						fs.log.Warn("got nil block while seeking")
						continue
					}
					if block, err := fs.GetBlock(header.Header); err != nil {
						fs.log.Warn("fail to retrieve block while seeking", "err", err)
						continue
					} else {
						blocks <- &types.BlockBundle{Block: block, Cert: header.Cert}
					}
				case <-timeout:
					fs.log.Warn("timeout was reached while seeking block")
					continue
				}
			}
		}
		close(blocks)
	}()

	return blocks
}

func (fs *fullSync) SeekForkedBlocks(ownBlocks []common.Hash, peerId peer.ID) chan types.BlockBundle {
	blocks := make(chan types.BlockBundle, 100)
	batch, err := fs.pm.GetForkBlockRange(peerId, ownBlocks)
	if err != nil {
		close(blocks)
		return blocks
	}
	go func() {
		defer close(blocks)
		for {
			timeout := time.After(time.Second * 10)
			select {
			case header, ok := <-batch.headers:
				if !ok {
					return
				}
				if block, err := fs.GetBlock(header.Header); err != nil {
					fs.log.Warn("fail to retrieve block while seeking", "err", err)
					return
				} else {
					blocks <- types.BlockBundle{Block: block, Cert: header.Cert}
				}
			case <-timeout:
				fs.log.Warn("timeout was reached while seeking block")
				return
			}
		}
	}()
	return blocks
}
