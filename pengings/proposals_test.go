package pengings

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestProposals_GetProposerPubKey(t *testing.T) {
	proposals := &Proposals{
		proofsByRound: &sync.Map{},
	}
	byRound := &sync.Map{}
	proposals.proofsByRound.Store(uint64(1), byRound)

	byRound.Store(common.Hash{0x1}, &Proof{PubKey: []byte{0x1}})
	byRound.Store(common.Hash{0x1, 0x1}, &Proof{PubKey: []byte{0x2}})
	byRound.Store(common.Hash{0x1, 0x2}, &Proof{PubKey: []byte{0x3}})

	require.Equal(t, []byte{0x3}, proposals.GetProposerPubKey(1))

}

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
