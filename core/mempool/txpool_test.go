package mempool

import (
	"crypto/ecdsa"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
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

	app.State.SetNonce(crypto.PubkeyToAddress(key1.PublicKey), 0)
	app.State.SetNonce(crypto.PubkeyToAddress(key2.PublicKey), 0)

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

	return appstate.NewAppState(stateDb)
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
