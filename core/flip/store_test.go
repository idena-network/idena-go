package flip

import (
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
	"testing"
)

func TestFlipStore_GetFlip(t *testing.T) {
	require := require.New(t)

	flipStore := NewStore(db.NewMemDB())

	left := []byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x10, 0x50}

	right := []byte{0x3, 0x23, 0x31, 0x4, 0x5, 0x6, 0x10, 0x50}

	hash, err := flipStore.PrepareFlip(1, 10, left, right)

	require.NoError(err)

	flip, err := flipStore.GetFlip(hash)

	require.NoError(err)
	require.Equal(uint16(1), flip.Epoch)
	require.Equal(left, flip.Data.Left)
	require.Equal(right, flip.Data.Right)
	require.Equal(uint16(10), flip.Data.Category)
}
