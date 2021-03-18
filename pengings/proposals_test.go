package pengings

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestProposals_GetBlock(t *testing.T) {
	proposals := &Proposals{
		blockCache: cache.New(time.Minute, time.Minute),
	}

	block := &types.Block{
		Header: &types.Header{
			ProposedHeader: &types.ProposedHeader{
				Height: 2,
			},
		},
	}

	proposals.ApproveBlock(block.Hash())

	proposals.AddBlock(block)

	require.Equal(t, block, proposals.GetBlock(block.Hash()))

	notApprovedBlock := &types.Block{
		Header: &types.Header{
			ProposedHeader: &types.ProposedHeader{
				Height: 3,
			},
		},
	}
	proposals.AddBlock(notApprovedBlock)

	require.True(t, proposals.GetBlock(notApprovedBlock.Hash()) == nil)
}

func TestProposals_setBestHash(t *testing.T) {
	proposals := &Proposals{
		bestProofs: map[uint64]bestHash{},
	}
	pubKey := []byte{0x1}

	proposals.setBestHash(1, common.Hash{0x1}, pubKey, 1)
	proposals.setBestHash(2, common.Hash{0x1}, pubKey, 5)

	require.Equal(t, common.Hash{0x1}, proposals.bestProofs[1].Hash)

	proposals.setBestHash(1, common.Hash{0x2}, pubKey, 1)

	require.Equal(t, common.Hash{0x2}, proposals.bestProofs[1].Hash)
	proposals.setBestHash(1, common.Hash{0x1}, pubKey, 1)
	require.Equal(t, common.Hash{0x2}, proposals.bestProofs[1].Hash)

	require.False(t, proposals.compareWithBestHash(1, common.Hash{0x1}, 1))
	require.True(t, proposals.compareWithBestHash(1, common.Hash{0x3}, 1))

	hash1 := common.Hash{0x8, 0x1, 0x1, 0xFF}

	proposals.setBestHash(1, hash1, pubKey, 1)
	require.Equal(t, hash1, proposals.bestProofs[1].Hash)

	hash2 := common.Hash{0x3, 0x1, 0x1, 0xFF}
	require.True(t, proposals.compareWithBestHash(1, hash2, 15))
	require.False(t, proposals.compareWithBestHash(1, hash2, 1))

	proposals.setBestHash(1, hash2, pubKey, 15)
	require.Equal(t, hash2, proposals.bestProofs[1].Hash)
	require.Equal(t, common.Hash{0x1}, proposals.bestProofs[2].Hash)
}
