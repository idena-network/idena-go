package state

import (
	"bytes"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/database"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-db"
	"math/big"
	"math/rand"
	"sort"
	"testing"
	"time"
)

func createDb(name string) *database.BackedMemDb {
	db, _ := db.NewGoLevelDB(name, "datadir")
	return database.NewBackedMemDb(db)
}

func TestStateDB_Version(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := NewLazy(database)
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

	stateDb, _ := NewLazy(db)
	stateDb2, _ := NewLazy(db2)

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

		_, _, _, err := forCheck.Commit(true)
		require.Nil(err)
	}

	require.Equal(stateDb.Root(), forCheck.Root())

	stateDb2, _ = NewLazy(db2)
	stateDb2.Load(100)
	require.Equal(originalHash, stateDb2.Root())

}

func TestStateDB_IterateIdentities(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := NewLazy(database)
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
	stateDb, _ := NewLazy(database)
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
	stateDb, _ := NewLazy(database)

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
	stateDb, _ := NewLazy(database)

	require.Equal(t, uint16(0), stateDb.Epoch())

	stateDb.IncEpoch()
	stateDb.IncEpoch()

	stateDb.Commit(false)
	stateDb.Clear()

	require.Equal(t, uint16(2), stateDb.Epoch())
}

func TestStateGlobal_VrfProposerThreshold(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := NewLazy(database)

	value := 0.95

	stateDb.SetVrfProposerThreshold(value)
	_, _, _, err := stateDb.Commit(false)
	require.NoError(t, err)
	stateDb.Clear()

	require.Equal(t, value, stateDb.VrfProposerThreshold())
}

func TestStateGlobal_EmptyBlocksRatio(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := NewLazy(database)

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

func TestStateDB_RecoverSnapshot2(t *testing.T) {
	//arrange
	database := db.NewMemDB()
	stateDb, _ := NewLazy(database)

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
	stateDb, _ = NewLazy(database)
	stateDb.Load(3)
	stateDb.tree.Hash()

	//act

	buffer := new(bytes.Buffer)
	stateDb.WriteSnapshot2(Height, buffer)
	require.True(t, buffer.Len() > 0)

	println(buffer.Len())

	require.Nil(t, stateDb.RecoverSnapshot2(Height, expectedRoot, buffer))

	batch := stateDb.original.NewBatch()

	dropDb := stateDb.CommitSnapshot(Height, batch)
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

func TestStateDB_Set_Has_ValidationTxBit(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := NewLazy(database)

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

func TestStateDB_GetContractValue(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := NewLazy(database)

	addr := common.Address{0x1}

	stateDb.SetContractValue(addr, []byte{0x1}, []byte{0x1})

	require.Equal(t, []byte{0x1}, stateDb.GetContractValue(addr, []byte{0x1}))

	stateDb.SetContractValue(addr, []byte{0x1}, []byte{0x2})

	require.Equal(t, []byte{0x2}, stateDb.GetContractValue(addr, []byte{0x1}))

	stateDb.Reset()
	require.Equal(t, []byte(nil), stateDb.GetContractValue(addr, []byte{0x1}))
}

func TestStateDB_IterateContractStore(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := NewLazy(database)

	stateDb.SetBalance(common.Address{0x1}, common.DnaBase)
	stateDb.SetFeePerGas(common.DnaBase)
	stateDb.SetState(common.Address{0x2}, 1)
	stateDb.DeployContract(common.Address{0x3}, common.Hash{0x02}, common.DnaBase)

	type keyValue struct {
		key   []byte
		value []byte
	}
	var stored []keyValue
	var addr1Values []keyValue
	for i := uint64(0); i < 100; i++ {
		addr := common.Address{}
		addr.SetBytes(common.ToBytes(i))
		for j := 0; j < 255; j++ {
			key := make([]byte, 20)
			rand.Read(key)
			value := make([]byte, 10)
			rand.Read(value)
			stateDb.SetContractValue(addr, key, value)
			stored = append(stored, keyValue{
				key: StateDbKeys.ContractStoreKey(addr, key), value: value,
			})
			if i == 1 {
				addr1Values = append(addr1Values, keyValue{key, value})
			}
		}
	}

	sort.SliceStable(stored, func(i, j int) bool {
		return bytes.Compare(stored[i].key, stored[j].key) > 0
	})

	sort.SliceStable(addr1Values, func(i, j int) bool {
		return bytes.Compare(addr1Values[i].key, addr1Values[j].key) > 0
	})

	stateDb.Commit(true)

	var iterated []keyValue
	stateDb.IterateContractValues(func(key []byte, value []byte) bool {
		iterated = append(iterated, keyValue{key, value})
		return false
	})

	sort.SliceStable(iterated, func(i, j int) bool {
		return bytes.Compare(iterated[i].key, iterated[j].key) > 0
	})
	require.Equal(t, stored, iterated)

	iterated = []keyValue{}
	addr := common.Address{}
	addr.SetBytes(common.ToBytes(uint64(1)))
	stateDb.IterateContractStore(addr, nil, nil, func(key []byte, value []byte) bool {
		iterated = append(iterated, keyValue{key, value})
		return false
	})

	sort.SliceStable(iterated, func(i, j int) bool {
		return bytes.Compare(iterated[i].key, iterated[j].key) > 0
	})
	require.Equal(t, len(addr1Values), len(iterated))
	require.Equal(t, addr1Values, iterated)
}

func TestStateDb_ShardId(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := NewLazy(database)

	require.Equal(t, common.ShardId(1), stateDb.ShardId(common.Address{0x1}))
	require.Equal(t, common.ShardId(1), stateDb.ShardId(common.Address{0x2}))

	stateDb.SetShardsNum(2)
	require.Equal(t, common.ShardId(2), stateDb.ShardId(common.Address{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1}))
	require.Equal(t, common.ShardId(1), stateDb.ShardId(common.Address{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2}))

	stateDb.SetShardsNum(4)
	require.Equal(t, common.ShardId(2), stateDb.ShardId(common.Address{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1}))
	require.Equal(t, common.ShardId(3), stateDb.ShardId(common.Address{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2}))

	identity := common.Address{0x1}
	stateDb.SetState(identity, Newbie)
	stateDb.SetShardId(identity, common.ShardId(3))
	require.Equal(t, common.ShardId(3), stateDb.ShardId(identity))
}

func TestStateDb_AddNewScore(t *testing.T) {
	stateDb, _ := NewLazy(db.NewMemDB())
	addr := common.Address{0x1}

	stateDb.AddNewScore(addr, common.EncodeScore(1, 6))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 1)

	stateDb.AddNewScore(addr, common.EncodeScore(2, 6))
	stateDb.AddNewScore(addr, common.EncodeScore(3, 6))
	stateDb.AddNewScore(addr, common.EncodeScore(4, 6))
	stateDb.AddNewScore(addr, common.EncodeScore(5, 6))
	stateDb.AddNewScore(addr, common.EncodeScore(6, 6))
	stateDb.AddNewScore(addr, common.EncodeScore(6, 6))
	stateDb.AddNewScore(addr, common.EncodeScore(6, 6))
	stateDb.AddNewScore(addr, common.EncodeScore(6, 6))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 9)
	stateDb.AddNewScore(addr, common.EncodeScore(6, 6))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 10)
	score, _ := common.DecodeScore(stateDb.GetIdentity(addr).Scores[0])
	require.Equal(t, float32(1), score)
	score, _ = common.DecodeScore(stateDb.GetIdentity(addr).Scores[9])
	require.Equal(t, float32(6), score)

	stateDb.AddNewScore(addr, common.EncodeScore(5, 6))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 10)

	score, _ = common.DecodeScore(stateDb.GetIdentity(addr).Scores[0])
	require.Equal(t, float32(2), score)
	score, _ = common.DecodeScore(stateDb.GetIdentity(addr).Scores[9])
	require.Equal(t, float32(5), score)

	stateDb.AddNewScore(addr, common.EncodeScore(1, 1))

	stateDb.AddNewScore(addr, common.EncodeScore(2, 2))

	stateDb.AddNewScore(addr, common.EncodeScore(1, 1))

	stateDb.AddNewScore(addr, common.EncodeScore(1, 1))

	stateDb.AddNewScore(addr, common.EncodeScore(1, 1))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 10)

	stateDb.AddNewScore(addr, common.EncodeScore(1, 1))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 11)

	stateDb.AddNewScore(addr, common.EncodeScore(1, 1))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 11)

	stateDb.AddNewScore(addr, common.EncodeScore(1, 2))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 12)

	stateDb.AddNewScore(addr, common.EncodeScore(5.5, 6))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 12)

	stateDb.AddNewScore(addr, common.EncodeScore(5.5, 6))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 12)

	stateDb.AddNewScore(addr, common.EncodeScore(5.5, 6))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 12)

	stateDb.AddNewScore(addr, common.EncodeScore(5.5, 6))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 11)

	stateDb.AddNewScore(addr, common.EncodeScore(5.5, 6))
	require.Len(t, stateDb.GetIdentity(addr).Scores, 10)

	for i := 0; i < 4; i++ {
		score, flips := common.DecodeScore(stateDb.GetIdentity(addr).Scores[i])
		require.Equal(t, float32(1), score)
		require.Equal(t, uint32(1), flips)
	}
	score, flips := common.DecodeScore(stateDb.GetIdentity(addr).Scores[4])
	require.Equal(t, float32(1), score)
	require.Equal(t, uint32(2), flips)
	for i := 5; i < 10; i++ {
		score, flips := common.DecodeScore(stateDb.GetIdentity(addr).Scores[i])
		require.Equal(t, float32(5.5), score)
		require.Equal(t, uint32(6), flips)
	}
}
