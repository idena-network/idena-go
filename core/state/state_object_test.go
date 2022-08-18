package state

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestApprovedIdentity_migrateToUpgrade8(t *testing.T) {
	v := &ApprovedIdentity{
		Validated:     true,
		Online:        true,
		Discriminated: true,
	}

	b, err := v.ToBytes(false)
	require.NoError(t, err)

	v = new(ApprovedIdentity)
	require.NoError(t, v.FromBytes(b))

	require.True(t, v.Validated)
	require.True(t, v.Online)
	require.False(t, v.Discriminated)

	v.Discriminated = true

	b, err = v.ToBytes(true)
	require.NoError(t, err)

	v = new(ApprovedIdentity)
	require.NoError(t, v.FromBytes(b))

	require.True(t, v.Validated)
	require.True(t, v.Online)
	require.True(t, v.Discriminated)
}

func TestStateIdentity_SubPenaltySeconds(t *testing.T) {
	identity := &stateIdentity{data: Identity{penaltySeconds: 10}}
	identity.SubPenaltySeconds(4)
	require.Equal(t, uint16(6), identity.GetPenaltySeconds())
	identity.SubPenaltySeconds(7)
	require.Zero(t, identity.GetPenaltySeconds())
	identity.SubPenaltySeconds(1)
	require.Zero(t, identity.GetPenaltySeconds())
}
