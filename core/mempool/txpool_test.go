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

	pool.Add(getTx(3, key1))
	pool.Add(getTx(1, key1))
	pool.Add(getTx(2, key1))
	pool.Add(getTx(6, key2))
	pool.Add(getTx(5, key2))

	result := pool.BuildBlockTransactions()

	require.Equal(t, 3, len(result))

	for i := uint64(0); i < uint64(len(result)); i++ {
		require.Equal(t, i+1, result[i].AccountNonce)
	}
}

func getAppState() *appstate.AppState {
	database := db.NewMemDB()
	stateDb, _ := state.NewLatest(database)

	key, _ := crypto.GenerateKey()

	// need at least 1 network size
	id := stateDb.GetOrNewIdentityObject(crypto.PubkeyToAddress(key.PublicKey))
	id.Approve()

	stateDb.Commit(false)

	res := appstate.NewAppState(stateDb)
	res.ValidatorsCache.Load()

	return res
}

func getTx(nonce uint64, key *ecdsa.PrivateKey) *types.Transaction {

	addr := crypto.PubkeyToAddress(key.PublicKey)

	tx := types.Transaction{
		AccountNonce: nonce,
		Type:         types.SendTx,
		To:           &addr,
		Amount:       new(big.Int),
	}

	signedTx, _ := types.SignTx(&tx, key)

	return signedTx
}
