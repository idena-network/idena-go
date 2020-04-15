package state

import (
	"bytes"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/database"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-db"
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

	originalHash := stateDb2.Root()

	forCheck, _ := stateDb2.ForCheckWithOverwrite(50)
	for i := 0; i < len(saved); i++ {

		acc := forCheck.GetOrNewAccountObject(saved[i].address)
		acc.SetBalance(saved[i].balance)

		_, _, err := forCheck.Commit(true)
		require.Nil(err)
	}

	require.Equal(stateDb.Root(), forCheck.Root())

	stateDb2 = NewLazy(db2)
	stateDb2.Load(100)
	require.Equal(originalHash, stateDb2.Root())

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

func TestStateGlobal_VrfProposerThreshold(t *testing.T) {
	database := db.NewMemDB()
	stateDb := NewLazy(database)

	value := 0.95

	stateDb.SetVrfProposerThreshold(value)
	_, _, err := stateDb.Commit(false)
	require.NoError(t, err)
	stateDb.Clear()

	require.Equal(t, value, stateDb.VrfProposerThreshold())
}

func TestStateGlobal_EmptyBlocksRatio(t *testing.T) {
	database := db.NewMemDB()
	stateDb := NewLazy(database)

	for i := 0; i < 15; i++ {
		stateDb.AddBlockBit(false)
	}
	stateDb.Commit(true)
	require.Equal(t, 10, stateDb.EmptyBlocksCount())

	for i := 0; i < 100; i++ {
		stateDb.AddBlockBit(true)
	}
	stateDb.Commit(true)
	require.Equal(t, 25, stateDb.EmptyBlocksCount())

	for i := 0; i < 1000; i++ {
		stateDb.AddBlockBit(false)
	}
	stateDb.Commit(true)
	require.Equal(t, 0, stateDb.EmptyBlocksCount())
	require.Len(t, stateDb.GetOrNewGlobalObject().data.EmptyBlocksBits.Bytes(), 4)
}

func TestStateDB_WriteSnapshot(t *testing.T) {
	database := db.NewMemDB()
	stateDb := NewLazy(database)

	stateDb.AddInvite(common.Address{}, 1)
	stateDb.AddInvite(common.Address{0x1}, 1)

	stateDb.Commit(true)

	buffer := new(bytes.Buffer)

	stateDb.WriteSnapshot(1, buffer)

	require.True(t, buffer.Len() > 0)
}

func TestStateDB_RecoverSnapshot(t *testing.T) {
	//arrange
	database := db.NewMemDB()
	stateDb := NewLazy(database)

	prevStateDb := stateDb.db

	identity := common.Address{}
	stateDb.AddInvite(identity, 1)

	stateDb.Commit(true)
	const AddrsCount = 50000
	const Height = uint64(2)

	for i := 0; i < AddrsCount; i++ {
		addr := common.Address{}
		addr.SetBytes(common.ToBytes(uint64(i)))
		stateDb.SetNonce(addr, uint32(i+1))
	}

	stateDb.Commit(true)

	var keys [][]byte
	var values [][]byte

	stateDb.IterateAccounts(func(key []byte, value []byte) bool {
		keys = append(keys, key)
		values = append(values, value)
		return false
	})

	require.Equal(t, AddrsCount, len(keys))

	expectedRoot := stateDb.Root()
	stateDb.AddInvite(common.Address{}, 2)

	stateDb.Commit(true)
	stateDb = NewLazy(database)
	stateDb.Load(3)
	stateDb.tree.Hash()

	//act

	buffer := new(bytes.Buffer)
	stateDb.WriteSnapshot(Height, buffer)
	require.True(t, buffer.Len() > 0)

	require.Nil(t, stateDb.RecoverSnapshot(&snapshot.Manifest{
		Height: Height,
		Root:   expectedRoot,
	}, buffer))

	batch := stateDb.original.NewBatch()

	dropDb := stateDb.CommitSnapshot(&snapshot.Manifest{
		Height: Height,
		Root:   expectedRoot,
	}, batch)
	common.ClearDb(dropDb)
	batch.WriteSync()
	//assert

	require.Equal(t, int64(Height), stateDb.tree.Version())
	require.Equal(t, expectedRoot, stateDb.Root())

	i := 0
	stateDb.IterateAccounts(func(key []byte, value []byte) bool {

		require.Equal(t, keys[i], key)
		require.Equal(t, values[i], value)
		i++
		return false
	})

	require.Equal(t, AddrsCount, i)

	cnt := 0

	stateDb.IterateIdentities(func(key []byte, value []byte) bool {
		addr := common.Address{}
		addr.SetBytes(key[1:])
		require.Equal(t, addr, identity)
		cnt++
		return false
	})
	require.Equal(t, 1, cnt)

	it, _ := prevStateDb.Iterator(nil, nil)
	defer it.Close()
	require.False(t, it.Valid())
}

func TestIdentityStateDB_SwitchTree(t *testing.T) {
	database := db.NewMemDB()
	stateDb := NewLazy(database)

	stateDb.Commit(true)

	require.NoError(t, stateDb.SwitchTree(100, 2))

	for i := 0; i < 150; i++ {
		stateDb.AddBalance(common.Address{}, big.NewInt(1))
		stateDb.Commit(true)
	}

	require.NoError(t, stateDb.FlushToDisk())

	stateDb = NewLazy(database)
	stateDb.Load(0)

	require.Equal(t, []int{1, 100, 150, 151}, stateDb.tree.AvailableVersions())

	stateDb.Commit(false)
	stateDb.Commit(false)
}

func TestStateDB_Set_Has_ValidationTxBit(t *testing.T) {
	database := db.NewMemDB()
	stateDb := NewLazy(database)

	addr := common.Address{0x1}
	stateDb.SetValidationTxBit(addr, types.SubmitAnswersHashTx)
	stateDb.Commit(true)

	require.True(t, stateDb.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.False(t, stateDb.HasValidationTx(addr, types.SubmitShortAnswersTx))
	require.False(t, stateDb.HasValidationTx(addr, types.EvidenceTx))
	require.False(t, stateDb.HasValidationTx(addr, types.SubmitLongAnswersTx))
	require.False(t, stateDb.HasValidationTx(addr, types.SendTx))

	stateDb.SetValidationTxBit(addr, types.SubmitShortAnswersTx)
	stateDb.Commit(true)

	require.True(t, stateDb.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.True(t, stateDb.HasValidationTx(addr, types.SubmitShortAnswersTx))
	require.False(t, stateDb.HasValidationTx(addr, types.EvidenceTx))
	require.False(t, stateDb.HasValidationTx(addr, types.SubmitLongAnswersTx))
	require.False(t, stateDb.HasValidationTx(addr, types.SendTx))

	stateDb.SetValidationTxBit(addr, types.EvidenceTx)
	stateDb.Commit(true)

	require.True(t, stateDb.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.True(t, stateDb.HasValidationTx(addr, types.SubmitShortAnswersTx))
	require.True(t, stateDb.HasValidationTx(addr, types.EvidenceTx))
	require.False(t, stateDb.HasValidationTx(addr, types.SubmitLongAnswersTx))
	require.False(t, stateDb.HasValidationTx(addr, types.SendTx))

	stateDb.SetValidationTxBit(addr, types.SubmitLongAnswersTx)
	stateDb.Commit(true)

	require.True(t, stateDb.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.True(t, stateDb.HasValidationTx(addr, types.SubmitShortAnswersTx))
	require.True(t, stateDb.HasValidationTx(addr, types.EvidenceTx))
	require.True(t, stateDb.HasValidationTx(addr, types.SubmitLongAnswersTx))
	require.False(t, stateDb.HasValidationTx(addr, types.SendTx))

	stateDb.SetValidationTxBit(addr, types.SendTx)
	stateDb.Commit(true)

	require.True(t, stateDb.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.True(t, stateDb.HasValidationTx(addr, types.SubmitShortAnswersTx))
	require.True(t, stateDb.HasValidationTx(addr, types.EvidenceTx))
	require.True(t, stateDb.HasValidationTx(addr, types.SubmitLongAnswersTx))
	require.False(t, stateDb.HasValidationTx(addr, types.SendTx))
}
