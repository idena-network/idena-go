package common

import (
	"github.com/stretchr/testify/require"
	db "github.com/tendermint/tm-db"
	"testing"
)

func TestCopy(t *testing.T) {
	original := db.NewMemDB()
	db1 := db.NewPrefixDB(original, []byte{0x1})
	db2 := db.NewPrefixDB(original, []byte{0x2})
	for k := byte(0); k < 255; k++ {
		require.NoError(t, db1.Set([]byte{k}, []byte{k}))
	}

	Copy(db1, db2)

	for k := byte(0); k < 255; k++ {
		has, _ := db2.Has([]byte{k})
		require.True(t, has)
	}
}

func TestClearDb(t *testing.T) {
	db1 := db.NewPrefixDB(db.NewMemDB(), []byte{0x1})
	for k := byte(0); k < 255; k++ {
		require.NoError(t, db1.Set([]byte{k}, []byte{k}))
	}

	ClearDb(db1)

	for k := byte(0); k < 255; k++ {
		has, _ := db1.Has([]byte{k})
		require.False(t, has)
	}
}
