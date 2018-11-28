package state

import (
	"github.com/stretchr/testify/require"
	db2 "github.com/tendermint/tendermint/libs/db"
	"idena-go/common"
	"math/big"
	"testing"
)

func TestStateDB_Version(t *testing.T) {
	db := db2.NewMemDB()
	stateDb, _ := NewLazy(db)
	require.Equal(t, int64(0), stateDb.Version())

	addr := common.Address{}

	stateDb.SetBalance(addr, new(big.Int).SetInt64(10))

	stateDb.Commit(true)

	require.Equal(t, int64(1), stateDb.Version())
}
