package protocol

import (
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"time"
)

const (
	BatchSize                          = 200
	MaxAttemptsCountPerBatch           = 10
	ForceFullSyncOnLastBlocksThreshold = 100
)

type Syncer interface {
	IsSyncing() bool
}

type blockApplier interface {
	processBatch(batch *batch, attemptNum int) error
	postConsuming()
	preConsuming(head *types.Header) (uint64, error)
}

type ForkResolver interface {
	HasLoadedFork() bool
}

type Downloader struct {
	pm                   *ProtocolManager
	log                  log.Logger
	chain                *blockchain.Blockchain
	batches              chan *batch
	ipfs                 ipfs.Proxy
	isSyncing            bool
	appState             *appstate.AppState
	top                  uint64
	potentialForkedPeers mapset.Set
}

func (d *Downloader) IsSyncing() bool {
	return d.isSyncing
}

func (d *Downloader) SyncProgress() (head uint64, top uint64) {
	return d.chain.Head.Height(), d.top
}

func NewDownloader(pm *ProtocolManager, chain *blockchain.Blockchain, ipfs ipfs.Proxy, appState *appstate.AppState) *Downloader {
	return &Downloader{
		pm:                   pm,
		chain:                chain,
		log:                  log.New("component", "downloader"),
		ipfs:                 ipfs,
		appState:             appState,
		isSyncing:            true,
		potentialForkedPeers: mapset.NewSet(),
	}
}

func getTopHeight(heights map[string]uint64) uint64 {
	max := uint64(0)
	for _, value := range heights {
		if value > max {
			max = value
		}
	}
	return max
}

func (d *Downloader) SyncBlockchain(forkResolver ForkResolver) {
	d.isSyncing = true
	defer func() {
		d.isSyncing = false
		d.top = 0
	}()
	for {
		if forkResolver.HasLoadedFork() {
			return
		}
		knownHeights := d.pm.GetKnownHeights()
		if knownHeights == nil {
			d.log.Info(fmt.Sprintf("Peers are not found. Assume node is synchronized"))
			break
		}
		head := d.chain.Head
		d.top = getTopHeight(knownHeights)
		if head.Height() >= d.top {
			d.log.Info(fmt.Sprintf("Node is synchronized"))
			return
		}
		d.Load()
	}
}

func (d *Downloader) Load() {

	head := d.chain.Head

	applier, toHeight := d.createBlockApplier()
	from, _ := applier.preConsuming(head)

	term := make(chan interface{})
	completed := make(chan interface{})
	go d.consumeBlocks(applier, term, completed)

	knownHeights := d.pm.GetKnownHeights()
loop:
	for from <= toHeight {
		for peer, height := range knownHeights {
			if height < from {
				continue
			}
			to := math.Min(from+BatchSize, height)
			if err, batch := d.pm.GetBlocksRange(peer, from, to); err != nil {
				continue
			} else {
				select {
				case d.batches <- batch:
				case <-term:
					break loop
				}
			}
			from = to + 1
		}
	}
	d.log.Info("All blocks was requested. Wait applying of blocks")
	close(completed)
	<-term
	applier.postConsuming()
}

func (d *Downloader) consumeBlocks(applier blockApplier, term chan interface{}, completed chan interface{}) {
	defer close(term)
	for {
		timeout := time.After(time.Second * 15)

		select {
		case batch := <-d.batches:
			if err := applier.processBatch(batch, 1); err != nil {
				d.log.Warn("failed to process batch", "err", err)
				return
			}
			continue
		default:
		}

		select {
		case batch := <-d.batches:
			if err := applier.processBatch(batch, 1); err != nil {
				d.log.Warn("failed to process batch", "err", err)
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

func (d *Downloader) GetBlock(header *types.Header) (*types.Block, error) {
	if header.EmptyBlockHeader != nil {
		return &types.Block{
			Header: header,
			Body:   &types.Body{},
		}, nil
	}
	if txs, err := d.ipfs.Get(header.ProposedHeader.IpfsHash); err != nil {
		return nil, err
	} else {
		if len(txs) > 0 {
			d.log.Debug("Retrieve block body from ipfs", "hash", header.Hash().Hex())
		}
		body := &types.Body{}
		body.FromBytes(txs)
		return &types.Block{
			Header: header,
			Body:   body,
		}, nil
	}
}

func (d *Downloader) PeekBlocks(fromBlock, toBlock uint64, peers []string) chan *types.Block {
	return NewFullSync(d.pm, d.log, d.chain, d.ipfs, d.appState, d.potentialForkedPeers).PeekBlocks(fromBlock, toBlock, peers)
}

func (d *Downloader) HasPotentialFork() bool {
	return d.potentialForkedPeers.Cardinality() > 0
}

func (d *Downloader) GetForkedPeers() mapset.Set {
	return d.potentialForkedPeers
}

func (d *Downloader) ClearPotentialForks() {
	d.potentialForkedPeers.Clear()
}

func (d *Downloader) createBlockApplier() (loader blockApplier, toHeight uint64) {

	canUseFastSync := true

	//knownHeights := d.pm.GetKnownHeights()
	//top := getTopHeight(knownHeights)
	if d.top-d.chain.Head.Height() < ForceFullSyncOnLastBlocksThreshold {
		canUseFastSync = false
	}

	manifest := d.getBestManifest()
	if manifest == nil || manifest.Height-d.chain.Head.Height() < ForceFullSyncOnLastBlocksThreshold {
		canUseFastSync = false
	}

	if canUseFastSync {
		return NewFastSync(d.pm, d.log, d.chain, d.ipfs, d.appState, d.potentialForkedPeers, manifest), manifest.Height
	} else {
		return NewFullSync(d.pm, d.log, d.chain, d.ipfs, d.appState, d.potentialForkedPeers), d.top
	}
}

func (d *Downloader) getBestManifest() *snapshot.Manifest {

	manifests := d.pm.GetKnownManifests()

	timeout := time.Second * 30
	if len(manifests) == 0 {
		d.log.Info("Wait for snapshot manifests")
		for start := time.Now(); time.Since(start) < timeout && len(manifests) == 0; {
			time.Sleep(2 * time.Second)
			manifests = d.pm.GetKnownManifests()
		}
	}

	var best *snapshot.Manifest

	for _, m := range manifests {
		if best == nil || best.Height < m.Height {
			best = m
		}
	}
	if best == nil {
		d.log.Info("Snapshot manifest is not found")
	}
	return best
}
