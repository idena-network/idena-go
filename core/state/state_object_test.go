package state

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStateIdentity_SubPenaltySeconds(t *testing.T) {
	identity := &stateIdentity{data: Identity{penaltySeconds: 10}}
	identity.SubPenaltySeconds(4)
	require.Equal(t, uint16(6), identity.GetPenaltySeconds())
	identity.SubPenaltySeconds(7)
	require.Zero(t, identity.GetPenaltySeconds())
	identity.SubPenaltySeconds(1)
	require.Zero(t, identity.GetPenaltySeconds())
}
