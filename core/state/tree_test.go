package state

import (
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/common"
	"testing"
)

func TestMutableTree_Hash(t *testing.T) {
	db := dbm.NewMemDB()
	tree := NewMutableTree(db)
	tree.Set([]byte{0x1}, []byte{0x2})
	tree.Set([]byte{0x3}, []byte{0x4})
	require.Equal(t, common.HashLength, len(tree.WorkingHash()))
	require.NotEqual(t, common.Hash{}, tree.WorkingHash())
}
