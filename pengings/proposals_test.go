package pengings

import (
	"github.com/idena-network/idena-go/blockchain/types"
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
