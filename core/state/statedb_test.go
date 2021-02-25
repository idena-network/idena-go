package state

import (
	"bytes"
	"crypto/rand"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/database"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-db"
	"math/big"
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

		_, _, err := forCheck.Commit(true)
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
	_, _, err := stateDb.Commit(false)
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

func TestStateDB_WriteSnapshot(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := NewLazy(database)

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
	stateDb.WriteSnapshot(Height, buffer)
	require.True(t, buffer.Len() > 0)

	require.Nil(t, stateDb.RecoverSnapshot(Height, expectedRoot, buffer))

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


func TestAAA(t *testing.T) {
	data1 := []byte{10, 9, 10, 147, 247, 153, 91, 236, 16, 0, 0, 32, 7, 58, 65, 4, 218, 240, 136, 202, 167, 186, 132, 61, 191, 176, 72, 35, 197, 54, 195, 147, 183, 42, 27, 152, 176, 18, 178, 174, 40, 186, 71, 205, 184, 152, 123, 160, 166, 17, 88, 152, 71, 63, 7, 50, 219, 125, 150, 192, 29, 215, 39, 151, 214, 44, 245, 104, 15, 19, 78, 229, 36, 26, 19, 100, 134, 174, 149, 4, 64, 3, 74, 38, 10, 36, 1, 85, 18, 32, 54, 125, 26, 57, 207, 98, 50, 198, 182, 40, 9, 114, 14, 244, 92, 176, 23, 114, 28, 54, 21, 99, 93, 245, 111, 241, 245, 230, 36, 153, 217, 28, 80, 1, 90, 12, 2, 141, 60, 249, 54, 26, 72, 38, 107, 172, 50, 53, 98, 56, 10, 32, 151, 245, 123, 138, 228, 101, 76, 180, 227, 55, 228, 250, 36, 52, 180, 42, 194, 92, 235, 248, 233, 37, 53, 116, 217, 119, 232, 21, 83, 176, 242, 232, 18, 20, 47, 62, 202, 28, 183, 32, 48, 137, 213, 70, 242, 188, 238, 215, 223, 169, 194, 69, 163, 30, 98, 56, 10, 32, 197, 159, 60, 133, 40, 212, 208, 67, 141, 40, 144, 249, 133, 133, 231, 234, 209, 6, 144, 41, 103, 1, 78, 83, 208, 97, 5, 224, 45, 217, 237, 226, 18, 20, 92, 49, 101, 43, 69, 175, 78, 114, 49, 103, 53, 116, 71, 98, 4, 255, 173, 235, 146, 32, 98, 56, 10, 32, 219, 219, 54, 83, 123, 78, 187, 191, 37, 25, 116, 53, 208, 190, 184, 142, 246, 130, 117, 120, 181, 195, 239, 216, 140, 186, 189, 44, 77, 72, 228, 167, 18, 20, 169, 94, 143, 234, 188, 112, 211, 61, 126, 0, 4, 105, 92, 27, 252, 30, 83, 114, 220, 129, 98, 56, 10, 32, 131, 157, 184, 227, 184, 46, 13, 253, 171, 165, 236, 42, 81, 55, 80, 116, 106, 247, 134, 104, 244, 61, 70, 103, 105, 59, 25, 118, 114, 85, 177, 221, 18, 20, 98, 94, 177, 64, 21, 47, 109, 111, 62, 160, 101, 170, 194, 207, 228, 30, 213, 103, 72, 139, 98, 56, 10, 32, 193, 180, 72, 15, 29, 28, 127, 212, 196, 223, 233, 233, 219, 143, 223, 224, 116, 135, 169, 224, 151, 245, 191, 125, 191, 33, 132, 196, 46, 6, 83, 34, 18, 20, 125, 206, 53, 134, 93, 159, 40, 37, 221, 92, 175, 66, 114, 206, 183, 148, 41, 42, 55, 23, 98, 56, 10, 32, 158, 151, 174, 104, 126, 40, 192, 211, 183, 210, 201, 108, 237, 34, 209, 39, 233, 57, 183, 78, 181, 211, 212, 12, 238, 26, 216, 120, 89, 135, 67, 201, 18, 20, 86, 69, 58, 228, 189, 33, 228, 112, 240, 241, 9, 8, 86, 23, 120, 21, 203, 44, 158, 154, 98, 56, 10, 32, 83, 141, 194, 129, 115, 175, 100, 132, 150, 137, 178, 252, 112, 124, 169, 217, 207, 29, 128, 38, 31, 56, 185, 19, 114, 131, 219, 142, 74, 47, 168, 101, 18, 20, 148, 120, 187, 126, 160, 30, 126, 201, 131, 179, 147, 190, 101, 62, 153, 253, 1, 34, 186, 210, 98, 56, 10, 32, 151, 33, 187, 16, 129, 50, 185, 246, 119, 233, 124, 116, 149, 111, 206, 54, 15, 81, 51, 110, 240, 129, 170, 205, 83, 242, 30, 215, 48, 254, 180, 20, 18, 20, 167, 32, 158, 184, 92, 207, 154, 1, 166, 146, 243, 122, 203, 238, 101, 97, 93, 119, 236, 178, 98, 56, 10, 32, 193, 188, 39, 87, 56, 252, 198, 239, 55, 232, 203, 30, 138, 247, 201, 36, 77, 111, 159, 50, 167, 23, 70, 10, 55, 111, 97, 114, 162, 2, 154, 65, 18, 20, 11, 253, 110, 58, 97, 138, 168, 29, 139, 21, 157, 161, 103, 244, 145, 124, 69, 129, 229, 37, 98, 56, 10, 32, 82, 247, 199, 179, 124, 77, 126, 40, 7, 214, 108, 62, 142, 242, 245, 100, 58, 127, 251, 102, 67, 192, 210, 37, 94, 223, 193, 4, 90, 102, 100, 220, 18, 20, 53, 2, 141, 60, 249, 54, 26, 72, 38, 107, 172, 50, 98, 14, 86, 206, 141, 161, 231, 155, 106, 56, 10, 32, 82, 247, 199, 179, 124, 77, 126, 40, 7, 214, 108, 62, 142, 242, 245, 100, 58, 127, 251, 102, 67, 192, 210, 37, 94, 223, 193, 4, 90, 102, 100, 220, 18, 20, 53, 2, 141, 60, 249, 54, 26, 72, 38, 107, 172, 50, 98, 14, 86, 206, 141, 161, 231, 155, 146, 1, 1, 33}
	data2 := []byte{10, 9, 10, 147, 247, 153, 91, 236, 16, 0, 0, 32, 7, 58, 65, 4, 218, 240, 136, 202, 167, 186, 132, 61, 191, 176, 72, 35, 197, 54, 195, 147, 183, 42, 27, 152, 176, 18, 178, 174, 40, 186, 71, 205, 184, 152, 123, 160, 166, 17, 88, 152, 71, 63, 7, 50, 219, 125, 150, 192, 29, 215, 39, 151, 214, 44, 245, 104, 15, 19, 78, 229, 36, 26, 19, 100, 134, 174, 149, 4, 64, 3, 74, 38, 10, 36, 1, 85, 18, 32, 54, 125, 26, 57, 207, 98, 50, 198, 182, 40, 9, 114, 14, 244, 92, 176, 23, 114, 28, 54, 21, 99, 93, 245, 111, 241, 245, 230, 36, 153, 217, 28, 80, 1, 90, 12, 2, 141, 60, 249, 54, 26, 72, 38, 107, 172, 50, 53, 98, 56, 10, 32, 151, 245, 123, 138, 228, 101, 76, 180, 227, 55, 228, 250, 36, 52, 180, 42, 194, 92, 235, 248, 233, 37, 53, 116, 217, 119, 232, 21, 83, 176, 242, 232, 18, 20, 47, 62, 202, 28, 183, 32, 48, 137, 213, 70, 242, 188, 238, 215, 223, 169, 194, 69, 163, 30, 98, 56, 10, 32, 197, 159, 60, 133, 40, 212, 208, 67, 141, 40, 144, 249, 133, 133, 231, 234, 209, 6, 144, 41, 103, 1, 78, 83, 208, 97, 5, 224, 45, 217, 237, 226, 18, 20, 92, 49, 101, 43, 69, 175, 78, 114, 49, 103, 53, 116, 71, 98, 4, 255, 173, 235, 146, 32, 98, 56, 10, 32, 219, 219, 54, 83, 123, 78, 187, 191, 37, 25, 116, 53, 208, 190, 184, 142, 246, 130, 117, 120, 181, 195, 239, 216, 140, 186, 189, 44, 77, 72, 228, 167, 18, 20, 169, 94, 143, 234, 188, 112, 211, 61, 126, 0, 4, 105, 92, 27, 252, 30, 83, 114, 220, 129, 98, 56, 10, 32, 131, 157, 184, 227, 184, 46, 13, 253, 171, 165, 236, 42, 81, 55, 80, 116, 106, 247, 134, 104, 244, 61, 70, 103, 105, 59, 25, 118, 114, 85, 177, 221, 18, 20, 98, 94, 177, 64, 21, 47, 109, 111, 62, 160, 101, 170, 194, 207, 228, 30, 213, 103, 72, 139, 98, 56, 10, 32, 193, 180, 72, 15, 29, 28, 127, 212, 196, 223, 233, 233, 219, 143, 223, 224, 116, 135, 169, 224, 151, 245, 191, 125, 191, 33, 132, 196, 46, 6, 83, 34, 18, 20, 125, 206, 53, 134, 93, 159, 40, 37, 221, 92, 175, 66, 114, 206, 183, 148, 41, 42, 55, 23, 98, 56, 10, 32, 158, 151, 174, 104, 126, 40, 192, 211, 183, 210, 201, 108, 237, 34, 209, 39, 233, 57, 183, 78, 181, 211, 212, 12, 238, 26, 216, 120, 89, 135, 67, 201, 18, 20, 86, 69, 58, 228, 189, 33, 228, 112, 240, 241, 9, 8, 86, 23, 120, 21, 203, 44, 158, 154, 98, 56, 10, 32, 83, 141, 194, 129, 115, 175, 100, 132, 150, 137, 178, 252, 112, 124, 169, 217, 207, 29, 128, 38, 31, 56, 185, 19, 114, 131, 219, 142, 74, 47, 168, 101, 18, 20, 148, 120, 187, 126, 160, 30, 126, 201, 131, 179, 147, 190, 101, 62, 153, 253, 1, 34, 186, 210, 98, 56, 10, 32, 151, 33, 187, 16, 129, 50, 185, 246, 119, 233, 124, 116, 149, 111, 206, 54, 15, 81, 51, 110, 240, 129, 170, 205, 83, 242, 30, 215, 48, 254, 180, 20, 18, 20, 167, 32, 158, 184, 92, 207, 154, 1, 166, 146, 243, 122, 203, 238, 101, 97, 93, 119, 236, 178, 98, 56, 10, 32, 193, 188, 39, 87, 56, 252, 198, 239, 55, 232, 203, 30, 138, 247, 201, 36, 77, 111, 159, 50, 167, 23, 70, 10, 55, 111, 97, 114, 162, 2, 154, 65, 18, 20, 11, 253, 110, 58, 97, 138, 168, 29, 139, 21, 157, 161, 103, 244, 145, 124, 69, 129, 229, 37, 98, 56, 10, 32, 82, 247, 199, 179, 124, 77, 126, 40, 7, 214, 108, 62, 142, 242, 245, 100, 58, 127, 251, 102, 67, 192, 210, 37, 94, 223, 193, 4, 90, 102, 100, 220, 18, 20, 53, 2, 141, 60, 249, 54, 26, 72, 38, 107, 172, 50, 98, 14, 86, 206, 141, 161, 231, 155, 106, 56, 10, 32, 82, 247, 199, 179, 124, 77, 126, 40, 7, 214, 108, 62, 142, 242, 245, 100, 58, 127, 251, 102, 67, 192, 210, 37, 94, 223, 193, 4, 90, 102, 100, 220, 18, 20, 53, 2, 141, 60, 249, 54, 26, 72, 38, 107, 172, 50, 98, 14, 86, 206, 141, 161, 231, 155, 146, 1, 1, 33, 160, 1, 1}

	identity1 := Identity{}
	identity1.FromBytes(data1)

	identity2 := Identity{}
	identity2.FromBytes(data2)
}
