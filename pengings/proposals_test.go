package pengings

import (
	"github.com/idena-network/idena-go/common"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
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
