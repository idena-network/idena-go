package mempool

import (
	"crypto/ecdsa"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/appstate"
	"idena-go/core/state"
	"idena-go/crypto"
	"math/big"
	"testing"
)

func TestTxPool_BuildBlockTransactions(t *testing.T) {
	app := getAppState()
	pool := NewTxPool(app)

	key1, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()

	balance := new(big.Int).Mul(common.DnaBase, big.NewInt(100))
	app.State.AddBalance(crypto.PubkeyToAddress(key1.PublicKey), balance)
	app.State.AddBalance(crypto.PubkeyToAddress(key2.PublicKey), balance)

	pool.Add(getTx(3, 0, key1))
	pool.Add(getTx(1, 0, key1))
	pool.Add(getTx(2, 0, key1))
	pool.Add(getTx(6, 0, key2))
	pool.Add(getTx(5, 0, key2))

	result := pool.BuildBlockTransactions()

	require.Equal(t, 3, len(result))

	for i := uint32(0); i < uint32(len(result)); i++ {
		require.Equal(t, i+1, result[i].AccountNonce)
	}

	app.State.IncEpoch()
	app.State.Commit(true)

	pool.Add(getTx(3, 0, key1))
	pool.Add(getTx(1, 0, key1))
	pool.Add(getTx(2, 0, key1))
	pool.Add(getTx(6, 1, key2))
	pool.Add(getTx(5, 1, key2))
	pool.Add(getTx(1, 1, key2))

	result = pool.BuildBlockTransactions()

	require.Equal(t, 1, len(result))
	require.Equal(t, uint16(1), result[0].Epoch)
	require.Equal(t, uint32(1), result[0].AccountNonce)
}

func TestTxPool_InvalidEpoch(t *testing.T) {
	app := getAppState()
	pool := NewTxPool(app)

	key, _ := crypto.GenerateKey()

	balance := new(big.Int).Mul(common.DnaBase, big.NewInt(100))
	app.State.AddBalance(crypto.PubkeyToAddress(key.PublicKey), balance)

	app.State.IncEpoch()

	app.State.Commit(true)

	err := pool.Add(getTx(1, 1, key))
	require.NoError(t, err)

	err = pool.Add(getTx(1, 0, key))
	require.Error(t, err)
}

func getAppState() *appstate.AppState {
	database := db.NewMemDB()
	stateDb, _ := state.NewLatest(database)

	key, _ := crypto.GenerateKey()

	// need at least 1 network size
	id := stateDb.GetOrNewIdentityObject(crypto.PubkeyToAddress(key.PublicKey))
	id.SetState(state.Verified)

	stateDb.Commit(false)

	res := appstate.NewAppState(stateDb)
	res.ValidatorsCache.Load()

	return res
}

func getTx(nonce uint32, epoch uint16, key *ecdsa.PrivateKey) *types.Transaction {

	addr := crypto.PubkeyToAddress(key.PublicKey)

	tx := types.Transaction{
		AccountNonce: nonce,
		Type:         types.RegularTx,
		To:           &addr,
		Amount:       new(big.Int),
		Epoch:        epoch,
	}

	signedTx, _ := types.SignTx(&tx, key)

	return signedTx
}
