package state

import (
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/common"
	"idena-go/crypto"
	"math/big"
	"testing"
	"time"
)

func TestStateDB_Version(t *testing.T) {
	database := db.NewMemDB()
	stateDb:= NewLazy(database)
	require.Equal(t, int64(0), stateDb.Version())

	addr := common.Address{}

	stateDb.SetBalance(addr, new(big.Int).SetInt64(10))

	stateDb.Commit(true)

	require.Equal(t, int64(1), stateDb.Version())
}

func TestStateDB_IterateIdentities(t *testing.T) {
	database := db.NewMemDB()
	stateDb:= NewLazy(database)
	require.Equal(t, int64(0), stateDb.Version())

	const accountsCount = 10001
	const identitiesCount = 9999

	for j := 0; j < accountsCount; j++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)

		stateDb.GetOrNewAccountObject(addr)
	}

	for j := 0; j < identitiesCount; j++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)

		stateDb.GetOrNewIdentityObject(addr)
	}

	stateDb.Commit(false)
	stateDb.Clear()

	require.Equal(t, int64(1), stateDb.Version())

	s := time.Now()
	counter := 0
	stateDb.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		counter++
		return false
	})

	t.Log(time.Since(s))

	require.Equal(t, identitiesCount, counter)
}

func TestStateDB_AddBalance(t *testing.T) {
	database := db.NewMemDB()
	stateDb := NewLazy(database)
	require.Equal(t, int64(0), stateDb.Version())

	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	balance := new(big.Int).SetInt64(int64(100))
	account := stateDb.GetOrNewAccountObject(addr)
	account.SetBalance(balance)

	stateDb.Commit(false)
	stateDb.Clear()

	fromDb := stateDb.GetOrNewAccountObject(addr)

	require.Equal(t, balance, fromDb.Balance())
}

func TestStateDB_GetOrNewIdentityObject(t *testing.T) {
	database := db.NewMemDB()
	stateDb := NewLazy(database)

	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	identity := stateDb.GetOrNewIdentityObject(addr)
	identity.SetState(Verified)

	stateDb.Commit(false)
	stateDb.Clear()

	fromDb := stateDb.GetOrNewIdentityObject(addr)

	require.Equal(t, Verified, fromDb.State())
}

func TestStateGlobal_IncEpoch(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := NewLatest(database)

	require.Equal(t, uint16(0), stateDb.GetEpoch())

	stateDb.IncEpoch()
	stateDb.IncEpoch()

	stateDb.Commit(false)
	stateDb.Clear()

	require.Equal(t, uint16(2), stateDb.GetEpoch())
}
