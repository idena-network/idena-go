package tests

import (
	"crypto/ecdsa"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/config"
	"idena-go/core/appstate"
	"idena-go/core/mempool"
	"idena-go/core/state"
	"idena-go/crypto"
	"math/big"
	"testing"
)

func TestTransactions_EpochChanging(t *testing.T) {
	database := db.NewMemDB()
	coinbaseKey, _ := crypto.GenerateKey()
	app := getAppState(database, coinbaseKey)
	pool := mempool.NewTxPool(app)
	chain := blockchain.NewBlockchain(config.GetDefaultConfig("", 0, true, "", 0, ""), database, pool, app)

	chain.InitializeChain(coinbaseKey)

	key1, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()

	addr1 := crypto.PubkeyToAddress(key1.PublicKey)
	addr2 := crypto.PubkeyToAddress(key2.PublicKey)

	balance := getAmount(100000)
	app.State.AddBalance(addr1, balance)
	app.State.AddBalance(addr2, balance)

	app.State.Commit(true)

	tx1 := generateTx(getAmount(12), addr2, 1, 0, key1)
	tx2 := generateTx(getAmount(88), addr1, 1, 0, key2)
	tx3 := generateTx(getAmount(32), addr2, 2, 0, key1)

	feeTx1 := types.CalculateCost(1, tx1)
	feeTx2 := types.CalculateCost(1, tx2)
	feeTx3 := types.CalculateCost(1, tx3)

	spend1 := new(big.Int).Add(feeTx1, feeTx3)
	receive1 := new(big.Int).Add(balance, getAmount(88))

	spend2 := feeTx2
	receive2 := new(big.Int).Add(balance, getAmount(44))

	require.NoError(t, pool.Add(tx1))
	require.NoError(t, pool.Add(tx2))
	require.NoError(t, pool.Add(tx3))

	block := chain.ProposeBlock()
	require.NoError(t, chain.AddBlock(block))

	require.Equal(t, app.State.GetBalance(addr1), new(big.Int).Sub(receive1, spend1))
	require.Equal(t, app.State.GetBalance(addr2), new(big.Int).Sub(receive2, spend2))

	//new epoch
	for i := 0; i < 100; i++ {
		block = chain.ProposeBlock()
		require.NoError(t, chain.AddBlock(block))
	}

	// new epoch started
	tx1 = generateTx(getAmount(15), addr2, 1, 1, key1)
	tx2 = generateTx(getAmount(10), addr1, 2, 1, key2) // wont be mined, nonce from future

	spend1 = types.CalculateCost(1, tx1)
	receive1 = app.State.GetBalance(addr1)

	receive2 = new(big.Int).Add(app.State.GetBalance(addr2), getAmount(15))

	require.NoError(t, pool.Add(tx1))
	require.NoError(t, pool.Add(tx2))

	block = chain.ProposeBlock()
	require.NoError(t, chain.AddBlock(block))

	require.Equal(t, 1, len(block.Body.Transactions))

	require.Equal(t, app.State.GetBalance(addr1), new(big.Int).Sub(receive1, spend1))
	require.Equal(t, app.State.GetBalance(addr2), receive2)

}

func getAmount(amount int64) *big.Int {
	return new(big.Int).Mul(common.DnaBase, big.NewInt(amount))
}

func getAppState(database db.DB, key *ecdsa.PrivateKey) *appstate.AppState {

	stateDb, _ := state.NewLatest(database)

	// need at least 1 network size
	id := stateDb.GetOrNewIdentityObject(crypto.PubkeyToAddress(key.PublicKey))
	id.SetState(state.Verified)

	stateDb.Commit(false)

	res := appstate.NewAppState(stateDb)
	res.ValidatorsCache.Load()

	return res
}

func generateTx(amount *big.Int, to common.Address, nonce uint32, epoch uint16, key *ecdsa.PrivateKey) *types.Transaction {

	tx := types.Transaction{
		AccountNonce: nonce,
		Type:         types.RegularTx,
		To:           &to,
		Amount:       amount,
		Epoch:        epoch,
	}

	signedTx, _ := types.SignTx(&tx, key)

	return signedTx
}
