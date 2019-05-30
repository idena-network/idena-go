package consensus

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common/math"
	"idena-go/log"
	"idena-go/protocol"
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
	resolver.triedPeers.Add(peerId)
	batchSize := uint64(5)

	to := resolver.chain.Head.Height() + 2
	from := to - batchSize
	oldestBlock := math.Max(2, to-100)

	forkBlocks := make([]*types.Block, 0)
	commonHeight := uint64(1)

	for ; commonHeight == 1; {
		blocks := resolver.downloader.PeekBlocks(from, to, []string{peerId})
		batchBlocks := make([]*types.Block, 0)
		for {
			block, ok := <-blocks
			if !ok {
				// all requested blocks have been consumed or a timeout has been reached
				break
			}
			existingBlock := resolver.chain.GetBlockByHeight(block.Height())
			if existingBlock.Hash() != block.Hash() {
				batchBlocks = append(batchBlocks, block)
			} else {
				commonHeight = block.Height()
				break
			}

		}
		to = from - 1
		if to < oldestBlock {
			//no common block has been found
			break
		}
		from = math.Max(oldestBlock, to-batchSize)

		if commonHeight == 1 && uint64(len(batchBlocks)) < to-from+1 {
			// peer didn't provide all requested blocks, ignore this one
			break
		}
		forkBlocks = append(forkBlocks, batchBlocks...)
	}
	if commonHeight > 1 {
		sortBlocks(forkBlocks)
		if resolver.isForkBigger(forkBlocks) {
			if err := resolver.chain.ValidateSubChain(commonHeight, forkBlocks); err != nil {
				resolver.log.Warn("unacceptable fork", "peerId", peerId, "err", err)
			} else {
				resolver.applicableFork = &applicableFork{
					commonHeight: commonHeight,
					blocks:       forkBlocks,
				}
			}
		}
	}
}

func sortBlocks(blocks []*types.Block) {

	sort.SliceStable(blocks, func(i, j int) bool {
		return blocks[i].Height() < blocks[j].Height()
	})
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
			ownProposedBlocks ++
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
		if err := resolver.chain.AddBlock(block); err != nil {
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
			time.Sleep(time.Minute * 1)
			if resolver.HasLoadedFork() {
				continue
			}
			resolver.loadAndVerifyFork()
		}
	}()
}

func (resolver *ForkResolver) HasLoadedFork() bool {
	return resolver.applicableFork != nil
}
