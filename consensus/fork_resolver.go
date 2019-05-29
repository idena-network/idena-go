package consensus

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common/math"
	"idena-go/log"
	"idena-go/pengings"
	"idena-go/protocol"
	"sort"
	"time"
)

type ForkResolver struct {
	proposals  *pengings.Proposals
	downloader *protocol.Downloader
	chain      *blockchain.Blockchain
	isLoading  bool
	log        log.Logger
	triedPeers mapset.Set
}

func NewForkResolver(proposals *pengings.Proposals, downloader *protocol.Downloader, chain *blockchain.Blockchain) *ForkResolver {
	return &ForkResolver{
		proposals:  proposals,
		downloader: downloader,
		chain:      chain,
		log:        log.New(),
		triedPeers: mapset.NewSet(),
	}
}

func (resolver *ForkResolver) loadAndVerifyFork() {

	peers := resolver.proposals.GetForkedPeers().Difference(resolver.triedPeers)
	if peers.Cardinality() == 0 {
		return
	}

	peerId := peers.Pop().(string)
	resolver.triedPeers.Add(peerId)

	batchSize := uint64(5)

	resolver.isLoading = true

	defer func() {
		resolver.isLoading = false
	}()

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
			break
		}
		from = math.Max(oldestBlock, to-batchSize)

		if commonHeight == 1 && uint64(len(batchBlocks)) < to-from+1 {
			break
		}
		forkBlocks = append(forkBlocks, batchBlocks...)
	}
	if commonHeight > 1 {
		sortBlocks(forkBlocks)
		if resolver.isForkBigger(forkBlocks) && resolver.chain.ValidateSubChain(commonHeight, forkBlocks) {

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

func (resolver *ForkResolver) HasApplicableFork() bool {
	panic("not implemeted")
}

func (resolver *ForkResolver) IsLoaded() bool {
	panic("not implemeted")
}

func (resolver *ForkResolver) ApplyFork() {
	panic("not implemeted")
}

func (resolver *ForkResolver) Start() {
	go func() {
		for {
			time.Sleep(time.Minute * 1)
			if resolver.isLoading {
				continue
			}
			if !resolver.proposals.HasFork() {
				continue
			}
			resolver.loadAndVerifyFork()
		}

	}()
}
