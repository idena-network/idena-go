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

func TestMutableTree_Root(t *testing.T){
	db := dbm.NewMemDB()
	tree := NewMutableTree(db)
	tree.Set([]byte{0x1}, []byte{0x1})
	tree.Set([]byte{0x11}, []byte{0x11})
	tree.Set([]byte{0x1, 0x1}, []byte{0x1, 0x1})
	tree.Set([]byte{0x1, 0x2}, []byte{0x1, 0x1})
	tree.Set([]byte{0x1, 0x3}, []byte{0x1, 0x1})
	tree.SaveVersion()

	tree.Set([]byte{0x1}, []byte{0x2})
	tree.Set([]byte{0x11}, []byte{0x17})
	tree.SaveVersion()

	root := tree.WorkingHash()

	tree.LoadVersion(1)
	_, v :=tree.Get([]byte{0x1})

	require.Equal(t,[]byte{0x1},  v)

	tree.Set([]byte{0x1}, []byte{0x2})
	tree.Set([]byte{0x11}, []byte{0x17})

	require.Equal(t,root,  tree.WorkingHash())
}

