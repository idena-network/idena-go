package state

import (
	"github.com/stretchr/testify/require"
	"math/big"
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

func TestStateIdentity_Discrimination(t *testing.T) {
	identity := &stateIdentity{data: Identity{
		State: Newbie,
		Stake: big.NewInt(9),
	}}

	{
		require.True(t, identity.IsDiscriminated(big.NewInt(9), 3))

		flags := identity.data.DiscriminationFlags(big.NewInt(9), 3)
		require.True(t, flags.HasFlag(DiscriminatedNewbie))
		require.False(t, flags.HasFlag(DiscriminatedDelegation))
		require.False(t, flags.HasFlag(DiscriminatedStake))
	}

	{
		require.False(t, identity.IsDiscriminated(big.NewInt(9), 2))

		flags := identity.data.DiscriminationFlags(big.NewInt(9), 2)
		require.Zero(t, flags)
	}

	{
		require.True(t, identity.IsDiscriminated(big.NewInt(10), 2))

		flags := identity.data.DiscriminationFlags(big.NewInt(10), 2)
		require.False(t, flags.HasFlag(DiscriminatedNewbie))
		require.False(t, flags.HasFlag(DiscriminatedDelegation))
		require.True(t, flags.HasFlag(DiscriminatedStake))
	}

	{
		identity = &stateIdentity{data: Identity{
			undelegationEpoch: 1,
		}}

		require.True(t, identity.IsDiscriminated(nil, 100))

		flags := identity.data.DiscriminationFlags(nil, 100)
		require.False(t, flags.HasFlag(DiscriminatedNewbie))
		require.True(t, flags.HasFlag(DiscriminatedDelegation))
		require.False(t, flags.HasFlag(DiscriminatedStake))
	}

}
