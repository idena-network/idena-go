package state

import (
	"github.com/stretchr/testify/require"
	db "github.com/tendermint/tm-db"
	"testing"
)

func TestSnapshotManager_IsInvalidManifest(t *testing.T) {
	m := SnapshotManager{
		db: db.NewMemDB(),
	}
	m.AddInvalidManifest([]byte{0x1})

	m.AddTimeoutManifest([]byte{0x3})
	m.AddTimeoutManifest([]byte{0x3})
	m.AddTimeoutManifest([]byte{0x3})
	m.AddTimeoutManifest([]byte{0x3})
	m.AddTimeoutManifest([]byte{0x3})

	m.AddTimeoutManifest([]byte{0x4})
	m.AddTimeoutManifest([]byte{0x4})
	m.AddTimeoutManifest([]byte{0x4})
	m.AddTimeoutManifest([]byte{0x4})

	require.True(t, m.IsInvalidManifest([]byte{0x1}))
	require.False(t, m.IsInvalidManifest([]byte{0x2}))
	require.True(t, m.IsInvalidManifest([]byte{0x3}))
	require.False(t, m.IsInvalidManifest([]byte{0x4}))
}
