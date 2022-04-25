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
