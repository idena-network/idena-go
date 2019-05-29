package state

import (
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/common"
	"idena-go/crypto"
	"idena-go/database"
	"math/big"
	"testing"
	"time"
)

func createDb(name string) *database.BackedMemDb {
	db, _ := db.NewGoLevelDB(name, "datadir")
	return database.NewBackedMemDb(db)
}

func TestStateDB_Version(t *testing.T) {
	database := db.NewMemDB()
	stateDb := NewLazy(database)
	require.Equal(t, int64(0), stateDb.Version())

	addr := common.Address{}

	stateDb.SetBalance(addr, new(big.Int).SetInt64(10))

	stateDb.Commit(true)

	require.Equal(t, int64(1), stateDb.Version())
}

func TestStateDB_CheckForkValidation(t *testing.T) {

	require := require.New(t)
	db := createDb("CheckForkValidation")
	db2 := createDb("CheckForkValidation2")

	stateDb := NewLazy(db)
	stateDb2 := NewLazy(db2)

	for i := 0; i < 50; i++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)

		balance := new(big.Int).SetInt64(int64(100))

		acc := stateDb.GetOrNewAccountObject(addr)
		acc2 := stateDb2.GetOrNewAccountObject(addr)

		acc.SetBalance(balance)
		acc2.SetBalance(balance)

		stateDb.Commit(true)
		stateDb2.Commit(true)
	}

	var saved []struct {
		address common.Address
		balance *big.Int
	}

	require.Equal(stateDb.Root(), stateDb2.Root())

	for i := 0; i < 50; i++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)

		b := new(big.Int).SetInt64(int64(100))

		acc := stateDb.GetOrNewAccountObject(addr)
		acc.SetBalance(b)

		saved = append(saved, struct {
			address common.Address
			balance *big.Int
		}{
			address: addr,
			balance: b,
		})

		stateDb.Commit(true)

		key2, _ := crypto.GenerateKey()
		addr2 := crypto.PubkeyToAddress(key2.PublicKey)

		b2 := new(big.Int).SetInt64(int64(100))
		acc2 := stateDb2.GetOrNewAccountObject(addr2)
		acc2.SetBalance(b2)

		stateDb2.Commit(true)
	}

	forCheck := stateDb2.ForCheck(50)

	for i := 0; i < len(saved); i++ {

		acc := forCheck.GetOrNewAccountObject(saved[i].address)
		acc.SetBalance(saved[i].balance)

		forCheck.Commit(true)
	}

	require.Equal(stateDb.Root(), forCheck.Root())
}

func TestStateDB_IterateIdentities(t *testing.T) {
	database := db.NewMemDB()
	stateDb := NewLazy(database)
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
	stateDb := NewLazy(database)

	require.Equal(t, uint16(0), stateDb.Epoch())

	stateDb.IncEpoch()
	stateDb.IncEpoch()

	stateDb.Commit(false)
	stateDb.Clear()

	require.Equal(t, uint16(2), stateDb.Epoch())
}
