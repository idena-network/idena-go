package protocol

import (
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/pkg/errors"
)

const (
	BatchSize                          = 200
	MaxAttemptsCountPerBatch           = 10
	ForceFullSyncOnLastBlocksThreshold = 100
)

type Syncer interface {
	IsSyncing() bool
}

type chainLoader interface {
	Load(toHeight uint64)
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

		syncer, toHeight := d.createSyncer()
		syncer.Load(toHeight)
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

func (d *Downloader) createSyncer() (loader chainLoader, toHeight uint64) {

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

	var best *snapshot.Manifest

	for _, m := range manifests {
		if best == nil || best.Height < m.Height {
			best = m
		}
	}
	return best
}

func (d *Downloader) shouldRetrieveBlock(header *types.Header) bool {
	knownHeights := d.pm.GetKnownHeights()
	top := getTopHeight(knownHeights)
	if top-header.Height() < ForceFullSyncOnLastBlocksThreshold {
		return true
	}
	if header.EmptyBlockHeader != nil {
		return header.ProposedHeader.Flags.HasFlag(types.IdentityUpdate)
	}
	return false
}
