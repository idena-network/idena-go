package consensus

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/protocol"
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
}

type applicableFork struct {
	commonHeight uint64
	blocks       []*types.Block
}

func NewForkResolver(forkDetectors []ForkDetector, downloader *protocol.Downloader, chain *blockchain.Blockchain) *ForkResolver {
	return &ForkResolver{
		forkDetectors: forkDetectors,
		downloader:    downloader,
		chain:         chain,
		log:           log.New(),
		triedPeers:    mapset.NewSet(),
	}
}

func (resolver *ForkResolver) loadAndVerifyFork() {

	var peerId string
	for _, d := range resolver.forkDetectors {
		if d.HasPotentialFork() {
			peers := d.GetForkedPeers().Difference(resolver.triedPeers)
			if peers.Cardinality() > 0 {
				peerId = peers.Pop().(string)
				break
			}
		}
	}
	if peerId == "" {
		return
	}

	resolver.log.Info("Start loading fork", "peerId", peerId)

	resolver.triedPeers.Add(peerId)
	batchSize := uint64(5)

	to := resolver.chain.Head.Height() + 2
	from := to - batchSize
	oldestBlock := math.Max(2, to-100)

	forkBlocks := make([]*types.Block, 0)
	commonHeight := uint64(1)

	for commonHeight == 1 {
		blocks := resolver.downloader.PeekBlocks(from, to, []string{peerId})
		batchBlocks := make([]*types.Block, 0)
		for {
			block, ok := <-blocks
			if !ok {
				// all requested blocks have been consumed or a timeout has been reached
				break
			}
			existingBlock := resolver.chain.GetBlockByHeight(block.Height())
			if existingBlock == nil || existingBlock.Hash() != block.Hash() {
				batchBlocks = append(batchBlocks, block)
			} else {
				commonHeight = block.Height()
			}
		}
		forkBlocks = append(forkBlocks, batchBlocks...)

		to = from - 1
		if to < oldestBlock {
			resolver.log.Warn("no common block has been found", "peerId", peerId)
			break
		}
		from = math.Max(oldestBlock, to-batchSize)
	}
	if commonHeight > 1 {
		resolver.log.Info("common block is found", "peerId", peerId)
		forkBlocks = sortAndFilterBlocks(forkBlocks, commonHeight)
		if resolver.isForkBigger(forkBlocks) {
			if err := resolver.chain.ValidateSubChain(commonHeight, forkBlocks); err != nil {
				resolver.log.Warn("unacceptable fork", "peerId", peerId, "err", err)
			} else {
				resolver.log.Info("applicable fork is detected", "peerId", peerId, "commonHeight", commonHeight)
				resolver.applicableFork = &applicableFork{
					commonHeight: commonHeight,
					blocks:       forkBlocks,
				}
			}
		} else {
			resolver.log.Warn("fork is smaller", "peerId", peerId)
		}
	} else {
		resolver.log.Warn("Common height is not found", "peerId", peerId)
	}
}

func sortAndFilterBlocks(blocks []*types.Block, minHeight uint64) []*types.Block {

	sort.SliceStable(blocks, func(i, j int) bool {
		return blocks[i].Height() < blocks[j].Height()
	})

	result := make([]*types.Block, 0)
	for _, b := range blocks {
		if b.Height() > minHeight {
			result = append(result, b)
		}
	}
	return result
}

func (resolver *ForkResolver) isForkBigger(fork []*types.Block) bool {

	if len(fork) == 0 {
		return false
	}
	forkLastBlock := fork[len(fork)-1]

	if forkLastBlock.Height() > resolver.chain.Head.Height() {
		return true
	}

	firstForkBlockHeight := fork[0].Height()

	forkProposedBlocks := 0
	ownProposedBlocks := 0

	for j, i := 0, firstForkBlockHeight; i <= forkLastBlock.Height(); i, j = i+1, j+1 {
		if !fork[j].IsEmpty() {
			forkProposedBlocks++
		}
		if !resolver.chain.GetBlockByHeight(i).IsEmpty() {
			ownProposedBlocks++
		}
	}
	if forkProposedBlocks > ownProposedBlocks {
		return true
	}
	return bytes.Compare(fork[0].Seed().Bytes(), resolver.chain.GetBlockByHeight(firstForkBlockHeight).Seed().Bytes()) > 0
}

func (resolver *ForkResolver) applyFork(commonHeight uint64, fork []*types.Block) error {

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
	for _, block := range fork {
		if err := resolver.chain.AddBlock(block, nil); err != nil {
			return err
		}
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
