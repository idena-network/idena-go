package state

import (
	"github.com/idena-network/idena-go/common"
	"github.com/stretchr/testify/require"
	"math/big"
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

func TestIdentity_Clear(t *testing.T) {
	identity := Identity{}
	identity.State = Killed
	identity.ShardId = common.ShardId(7)
	identity.ProfileHash = []byte{0x1}
	identity.DelegationNonce = 2
	identity.Penalty = big.NewInt(1)
	identity.penaltyTimestamp = 3
	identity.penaltySeconds = 4

	identity.Clear(true, true, true)
	require.Equal(t, Undefined, identity.State)
	require.Zero(t, identity.ShardId)
	require.Equal(t, []byte{0x1}, identity.ProfileHash)
	require.Equal(t, uint32(2), identity.DelegationNonce)
	require.Zero(t, big.NewInt(1).Cmp(identity.Penalty))
	require.Equal(t, int64(3), identity.penaltyTimestamp)
	require.Equal(t, uint16(4), identity.penaltySeconds)

	identity.Clear(false, true, true)
	require.Equal(t, Undefined, identity.State)
	require.Nil(t, identity.ProfileHash)
	require.Equal(t, uint32(2), identity.DelegationNonce)
	require.Zero(t, big.NewInt(1).Cmp(identity.Penalty))
	require.Equal(t, int64(3), identity.penaltyTimestamp)
	require.Equal(t, uint16(4), identity.penaltySeconds)

	identity.Clear(true, false, true)
	require.Equal(t, Undefined, identity.State)
	require.Nil(t, identity.ProfileHash)
	require.Equal(t, uint32(2), identity.DelegationNonce)
	require.Nil(t, identity.Penalty)
	require.Zero(t, identity.penaltyTimestamp)
	require.Zero(t, identity.penaltySeconds)

	identity.Clear(true, true, false)
	require.Equal(t, Identity{}, identity)
}
