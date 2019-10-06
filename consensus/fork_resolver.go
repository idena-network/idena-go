package consensus

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/protocol"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	"sort"
	"time"
)

type ForkResolver struct {
	forkDetectors  []ForkDetector
	downloader     *protocol.Downloader
	chain          *blockchain.Blockchain
	log            log.Logger
	triedPeers     mapset.Set
	applicableFork *applicableFork
	statsCollector collector.StatsCollector
}

type applicableFork struct {
	commonHeight uint64
	blocks       []types.BlockBundle
}

func NewForkResolver(forkDetectors []ForkDetector, downloader *protocol.Downloader, chain *blockchain.Blockchain,
	statsCollector collector.StatsCollector) *ForkResolver {
	return &ForkResolver{
		forkDetectors:  forkDetectors,
		downloader:     downloader,
		chain:          chain,
		log:            log.New(),
		triedPeers:     mapset.NewSet(),
		statsCollector: statsCollector,
	}
}

func (resolver *ForkResolver) loadAndVerifyFork() {

	var peerId peer.ID
	for _, d := range resolver.forkDetectors {
		if d.HasPotentialFork() {
			peers := d.GetForkedPeers().Difference(resolver.triedPeers)
			if peers.Cardinality() > 0 {
				peerId = peers.Pop().( peer.ID)
				break
			}
		}
	}
	if peerId == "" {
		return
	}

	resolver.log.Info("Start loading fork", "peerId", peerId)

	resolver.triedPeers.Add(peerId)

	lastBlocksHashes := resolver.chain.GetTopBlockHashes(100)

	blocks := resolver.downloader.SeekForkedBlocks(lastBlocksHashes, peerId)
	if err := resolver.processBlocks(blocks, peerId); err != nil {
		resolver.downloader.BanPeer(peerId, err)
		resolver.log.Warn("invalid fork", err, err)
	}
}

func (resolver *ForkResolver) processBlocks(blocks chan types.BlockBundle, peerId  peer.ID) error {
	forkBlocks := make([]types.BlockBundle, 0)
	for {
		bundle, ok := <-blocks
		if !ok {
			// all requested blocks have been consumed or a timeout has been reached
			break
		}
		forkBlocks = append(forkBlocks, bundle)
	}
	if len(forkBlocks) == 0 {
		return errors.Errorf("common height is not found, peerId=%v", peerId)
	}

	resolver.log.Info("common block is found", "peerId", peerId)
	forkBlocks = sortBlocks(forkBlocks)
	if err := resolver.checkForkSize(forkBlocks); err == nil {
		commonHeight := forkBlocks[0].Block.Height() - 1
		if err := resolver.chain.ValidateSubChain(commonHeight, forkBlocks); err != nil {
			return errors.Errorf("unacceptable fork, peerId=%v, err=%v", peerId, err)
		} else {
			resolver.log.Info("applicable fork is detected", "peerId", peerId, "commonHeight", commonHeight, "size", len(forkBlocks))
			resolver.applicableFork = &applicableFork{
				commonHeight: commonHeight,
				blocks:       forkBlocks,
			}
		}
	} else {
		return errors.Errorf("fork is smaller, peerId=%v, err=%v", peerId, err)
	}
	return nil
}

func sortBlocks(blocks []types.BlockBundle) []types.BlockBundle {
	sort.SliceStable(blocks, func(i, j int) bool {
		return blocks[i].Block.Height() < blocks[j].Block.Height()
	})
	return blocks
}

func (resolver *ForkResolver) checkForkSize(fork []types.BlockBundle) error {

	if len(fork) == 0 {
		return errors.New("fork is empty")
	}
	forkLastBlock := fork[len(fork)-1].Block

	if forkLastBlock.Height() > resolver.chain.Head.Height() {
		return nil
	}

	firstForkBlockHeight := fork[0].Block.Height()

	forkProposedBlocks := 0
	ownProposedBlocks := 0

	for j, i := 0, firstForkBlockHeight; i <= forkLastBlock.Height(); i, j = i+1, j+1 {
		if !fork[j].Block.IsEmpty() {
			forkProposedBlocks++
		}
		if !resolver.chain.GetBlockByHeight(i).IsEmpty() {
			ownProposedBlocks++
		}
	}
	if forkProposedBlocks < ownProposedBlocks {
		return errors.New("fork has less proposed blocks")
	}
	forkHasBestSeed := bytes.Compare(fork[0].Block.Seed().Bytes(), resolver.chain.GetBlockByHeight(firstForkBlockHeight).Seed().Bytes()) > 0
	if forkHasBestSeed {
		return nil
	}
	return errors.New("fork has worse seed")
}

func (resolver *ForkResolver) applyFork(commonHeight uint64, fork []types.BlockBundle) error {

	defer func() {
		resolver.applicableFork = nil
		resolver.triedPeers.Clear()

		for _, d := range resolver.forkDetectors {
			d.ClearPotentialForks()
		}
	}()

	if err := resolver.chain.ResetTo(commonHeight); err != nil {
		return err
	}
	for _, bundle := range fork {
		if err := resolver.chain.AddBlock(bundle.Block, nil, resolver.statsCollector); err != nil {
			return err
		}
		resolver.chain.WriteCertificate(bundle.Block.Hash(), bundle.Cert, false)
	}

	return nil
}

func (resolver *ForkResolver) ApplyFork() error {
	if !resolver.HasLoadedFork() {
		panic("resolver hasn't applicable fork")
	}
	return resolver.applyFork(resolver.applicableFork.commonHeight, resolver.applicableFork.blocks)
}

func (resolver *ForkResolver) Start() {
	go func() {
		for {
			time.Sleep(time.Second * 30)
			if resolver.HasLoadedFork() {
				continue
			}
			resolver.loadAndVerifyFork()
		}
	}()
	go func() {
		for {
			time.Sleep(time.Minute * 10)
			resolver.triedPeers.Clear()
			resolver.log.Debug("Tried peers has been cleared")
		}
	}()
}

func (resolver *ForkResolver) HasLoadedFork() bool {
	return resolver.applicableFork != nil
}
