package pengings

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
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

	hashes := []common.Hash{{0x1}, {0x1, 0x2}, {0x3, 0x2}, {0x2, 0x2}, {0x2}}

	var pubKeys [][]byte
	for i := 0; i < 5; i++ {
		key, _ := crypto.GenerateKey()

		proofProposal := &types.ProofProposal{
			Proof: []byte{byte(i)},
			Round: 1,
		}

		hash := crypto.SignatureHash(proofProposal)
		sig, _ := crypto.Sign(hash[:], key)
		proofProposal.Signature = sig
		byRound.Store(hashes[i], proofProposal)

		pubKeys = append(pubKeys, crypto.FromECDSAPub(&key.PublicKey))
	}

	require.Equal(t, pubKeys[2], proposals.GetProposerPubKey(1))
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
