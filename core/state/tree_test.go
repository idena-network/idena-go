package state

import (
	"github.com/idena-network/idena-go/common"
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-cmn/db"
	"math/rand"
	"testing"
	"time"
)

func TestMutableTree_Hash(t *testing.T) {
	db := dbm.NewMemDB()
	tree := NewMutableTree(db)
	tree.Set([]byte{0x1}, []byte{0x2})
	tree.Set([]byte{0x3}, []byte{0x4})
	require.Equal(t, common.HashLength, len(tree.WorkingHash()))
	require.NotEqual(t, common.Hash{}, tree.WorkingHash())
}

func TestMutableTree_Root(t *testing.T) {
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
	_, v := tree.Get([]byte{0x1})

	require.Equal(t, []byte{0x1}, v)

	tree.Set([]byte{0x1}, []byte{0x2})
	tree.Set([]byte{0x11}, []byte{0x17})

	require.Equal(t, root, tree.WorkingHash())
}

func TestImmutableTree_LoadVersionForOverwriting(t *testing.T) {
	db := dbm.NewMemDB()
	tree := NewMutableTree(db)
	tree.Set([]byte{0x1}, []byte{0x1})
	tree.Set([]byte{0x11}, []byte{0x11})
	tree.SaveVersion()

	tree.Set([]byte{0x1}, []byte{0x2})
	tree.Set([]byte{0x11}, []byte{0x17})
	tree.SaveVersion()

	_, err := tree.LoadVersionForOverwriting(1)
	require.Nil(t, err)

	require.False(t, tree.ExistVersion(2))

	tree.tree.Set([]byte{0x1}, []byte{0x3})
	_, _, err = tree.SaveVersion()
	require.Nil(t, err)

	_, v := tree.Get([]byte{0x1})

	require.Equal(t, []byte{0x3}, v)

	require.True(t, tree.ExistVersion(2))
}

func TestMutableTree_LoadVersion(t *testing.T) {
	db := dbm.NewMemDB()
	tree := NewMutableTree(db)
	const assertVersion = 6
	var hash common.Hash

	var repeat [][]byte
	rnd := rand.New(rand.NewSource(time.Now().Unix()))
	for i := 1; i < 70; i++ {

		for _, key := range repeat {
			value := common.ToBytes(rnd.Uint64())
			tree.Set(key, value)
		}
		repeat = make([][]byte, 0)

		for j := 0; j < 100; j++ {
			key := common.ToBytes(rnd.Int31n(10000000))
			value := common.ToBytes(rnd.Uint64())
			tree.Set(key, value)

			if j > 90 {
				repeat = append(repeat, key)
			}
		}
		tree.SaveVersion()
		if i == assertVersion {
			hash = tree.WorkingHash()
		}
	}
	tree.LoadVersion(assertVersion)

	require.Equal(t, hash, tree.WorkingHash())
}