package protocol

import (
	"errors"
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/libp2p/go-libp2p-core/peer"
	"time"
)

const (
	MaxAttemptsCountPerBatch = 10
)

var (
	BanReasonTimeout = errors.New("timeout")
)

type Syncer interface {
	IsSyncing() bool
}

type blockApplier interface {
	batchSize() uint64
	processBatch(batch *batch, attemptNum int) error
	postConsuming() (err error)
	preConsuming(head *types.Header) (uint64, error)
}

type ForkResolver interface {
	HasLoadedFork() bool
}

type BlockSeeker interface {
	SeekBlocks(fromBlock, toBlock uint64, peers []peer.ID) chan *types.BlockBundle
}

type Downloader struct {
	pm                   *IdenaGossipHandler
	cfg                  *config.Config
	log                  log.Logger
	chain                *blockchain.Blockchain
	batches              chan *batch
	ipfs                 ipfs.Proxy
	isSyncing            bool
	appState             *appstate.AppState
	top                  uint64
	potentialForkedPeers mapset.Set
	sm                   *state.SnapshotManager
	bus                  eventbus.Bus
	secStore             *secstore.SecStore
	statsCollector       collector.StatsCollector
}

func (d *Downloader) IsSyncing() bool {
	return d.isSyncing
}

func (d *Downloader) SyncProgress() (head uint64, top uint64) {
	height := d.chain.Head.Height()
	if d.chain.PreliminaryHead != nil {
		height = math.Max(height, d.chain.PreliminaryHead.Height())
	}
	return height, d.top
}

func NewDownloader(
	pm *IdenaGossipHandler,
	cfg *config.Config,
	chain *blockchain.Blockchain,
	ipfs ipfs.Proxy,
	appState *appstate.AppState,
	sm *state.SnapshotManager,
	bus eventbus.Bus,
	secStore *secstore.SecStore,
	statsCollector collector.StatsCollector,
) *Downloader {
	return &Downloader{
		pm:                   pm,
		cfg:                  cfg,
		chain:                chain,
		log:                  log.New("component", "downloader"),
		ipfs:                 ipfs,
		appState:             appState,
		isSyncing:            false,
		potentialForkedPeers: mapset.NewSet(),
		sm:                   sm,
		bus:                  bus,
		secStore:             secStore,
		statsCollector:       statsCollector,
	}
}

func getTopHeight(heights map[peer.ID]uint64) uint64 {
	max := uint64(0)
	for _, value := range heights {
		if value > max {
			max = value
		}
	}
	return max
}

func (d *Downloader) filterForkedPeers(peers map[peer.ID]uint64) {
	for _, p := range d.potentialForkedPeers.ToSlice() {
		delete(peers, p.(peer.ID))
	}
}

func (d *Downloader) SyncBlockchain(forkResolver ForkResolver) error {

	for {
		if forkResolver.HasLoadedFork() {
			return errors.New("loaded fork is detected")
		}
		knownHeights := d.pm.GetKnownHeights()
		if knownHeights == nil {
			d.log.Info(fmt.Sprintf("Peers are not found. Assume node is synchronized"))
			return nil
		}

		d.filterForkedPeers(knownHeights)

		if len(knownHeights) == 0 {
			return errors.New("all connected peers are in fork")
		}

		head := d.chain.Head
		d.top = getTopHeight(knownHeights)
		if head.Height() >= d.top {
			d.log.Info(fmt.Sprintf("Node is synchronized"))
			return nil
		}
		if !d.isSyncing {
			d.startSync()
			defer d.stopSync()
		}
		d.Load()
	}
}

func (d *Downloader) Load() {

	head := d.chain.Head

	applier, toHeight := d.createBlockApplier()

	var from uint64
	var err error
	if from, err = applier.preConsuming(head); err != nil {
		d.log.Error("pre consuming error", "err", err)
		time.Sleep(5 * time.Second)
		return
	}

	d.batches = make(chan *batch, 10)
	term := make(chan interface{})
	completed := make(chan interface{})
	go d.consumeBlocks(applier, term, completed)

	knownHeights := d.pm.GetKnownHeights()
loop:
	for from <= toHeight && len(knownHeights) > 0 {
		for peer, height := range knownHeights {
			if height < from {
				delete(knownHeights, peer)
				continue
			}
			to := math.Min(from+applier.batchSize(), math.Min(toHeight, height))
			if batch, err := d.pm.GetBlocksRange(peer, from, to); err != nil {
				delete(knownHeights, peer)
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
	d.log.Info("All blocks were requested. Wait for applying of blocks")
	close(completed)
	<-term
	if err := applier.postConsuming(); err != nil {
		d.log.Error("Post consuming error", "err", err)
		time.Sleep(5 * time.Second)
	}
}

func (d *Downloader) consumeBlocks(applier blockApplier, term chan interface{}, completed chan interface{}) {
	defer close(term)

	consume := func(batch *batch) (stop bool) {
		if len(batch.headers) == 0 && !d.pm.IsConnected(batch.p.id) {
			batch = requestBatch(d.pm, batch.from, batch.to, batch.p.id)
			if batch == nil {
				d.log.Warn("failed to process batch", "err", "no peers")
				return true
			}
		}

		if err := applier.processBatch(batch, 1); err != nil {
			d.log.Warn("failed to process batch", "err", err)
			return true
		}
		return false
	}

	for {
		timeout := time.After(time.Second * 15)

		select {
		case batch := <-d.batches:
			if consume(batch) {
				return
			}
			continue
		default:
		}

		select {
		case batch := <-d.batches:
			if consume(batch) {
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

func (d *Downloader) SeekBlocks(fromBlock, toBlock uint64, peers []peer.ID) chan *types.BlockBundle {
	return NewFullSync(d.pm, d.log, d.chain, d.ipfs, d.appState, d.potentialForkedPeers, 0, d.statsCollector).SeekBlocks(fromBlock, toBlock, peers)
}

func (d *Downloader) SeekForkedBlocks(ownBlocks []common.Hash, peerId peer.ID) chan types.BlockBundle {
	return NewFullSync(d.pm, d.log, d.chain, d.ipfs, d.appState, d.potentialForkedPeers, 0, d.statsCollector).SeekForkedBlocks(ownBlocks, peerId)
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

	canUseFastSync := d.cfg.Sync.FastSync

	if d.top-d.chain.Head.Height() < d.cfg.Sync.ForceFullSync {
		canUseFastSync = false
	}
	var manifest *snapshot.Manifest
	if canUseFastSync {
		manifest = d.getBestManifest()
		if manifest == nil || d.chain.Head.Height() > manifest.Height || manifest.Height-d.chain.Head.Height() < d.cfg.Sync.ForceFullSync {
			canUseFastSync = false
		}
	}

	if canUseFastSync {
		d.log.Info("Fast sync will be used")
		return NewFastSync(d.pm, d.log, d.chain, d.ipfs, d.appState, d.potentialForkedPeers, manifest, d.sm, d.bus, d.secStore.GetAddress()), manifest.Height
	} else {
		d.log.Info("Full sync will be used")
		top := d.top
		return NewFullSync(d.pm, d.log, d.chain, d.ipfs, d.appState, d.potentialForkedPeers, top, d.statsCollector), top
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
		if (best == nil || best.Height < m.Height) && !d.sm.IsInvalidManifest(m.Cid) {
			best = m
		}
	}
	if best == nil {
		d.log.Info("Snapshot manifest is not found")
	} else {
		d.log.Info("Found manifest", "height", best.Height)
	}
	return best
}

func (d *Downloader) startSync() {
	d.isSyncing = true
	d.chain.StartSync()
	d.sm.StartSync()
}

func (d *Downloader) stopSync() {
	d.chain.StopSync()
	d.sm.StopSync()
	d.isSyncing = false
	d.top = 0
}

func (d *Downloader) BanPeer(peerId peer.ID, reason error) {
	if d.pm != nil {
		d.pm.BanPeer(peerId, reason)
	}
}

func requestBatch(pm *IdenaGossipHandler, from, to uint64, ignoredPeer peer.ID) *batch {
	knownHeights := pm.GetKnownHeights()
	if knownHeights == nil {
		return nil
	}
	for peerId, height := range knownHeights {
		if (peerId != ignoredPeer || len(knownHeights) == 1) && height >= to {
			if batch, err := pm.GetBlocksRange(peerId, from, to); err != nil {
				continue
			} else {
				return batch
			}
		}
	}
	return nil
}
