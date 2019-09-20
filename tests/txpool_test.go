package tests

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestTxPool_BuildBlockTransactions(t *testing.T) {

	key1, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()
	balance := new(big.Int).Mul(common.DnaBase, big.NewInt(100))

	alloc := make(map[common.Address]config.GenesisAllocation)
	alloc[crypto.PubkeyToAddress(key1.PublicKey)] = config.GenesisAllocation{
		Balance: balance,
	}
	alloc[crypto.PubkeyToAddress(key2.PublicKey)] = config.GenesisAllocation{
		Balance: balance,
	}

	_, app, pool := newBlockchain(true, alloc, -1, -1)

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
	app.Commit(nil)

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

func TestTxPool_TxLimits(t *testing.T) {
	key1, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()
	balance := new(big.Int).Mul(common.DnaBase, big.NewInt(100))

	alloc := make(map[common.Address]config.GenesisAllocation)
	alloc[crypto.PubkeyToAddress(key1.PublicKey)] = config.GenesisAllocation{
		Balance: balance,
	}
	alloc[crypto.PubkeyToAddress(key2.PublicKey)] = config.GenesisAllocation{
		Balance: balance,
	}

	_, _, pool := newBlockchain(true, alloc, 3, 2)

	tx := getTx(1, 0, key1)
	err := pool.Add(tx)
	require.Nil(t, err)

	err = pool.Add(getTx(2, 1, key1))
	require.Nil(t, err)

	err = pool.Add(getTx(3, 2, key1))
	require.NotNil(t, err)

	err = pool.Add(getTx(4, 3, key2))
	require.Nil(t, err)

	// total limit
	err = pool.Add(getTx(5, 4, key2))
	require.NotNil(t, err)

	// remove unknown tx
	pool.Remove(getTx(1, 2, key2))
	err = pool.Add(getTx(6, 5, key2))
	require.NotNil(t, err)

	pool.Remove(tx)
	err = pool.Add(getTx(7, 5, key2))
	require.Nil(t, err)
}

func TestTxPool_InvalidEpoch(t *testing.T) {
	key, _ := crypto.GenerateKey()

	balance := new(big.Int).Mul(common.DnaBase, big.NewInt(100))

	alloc := make(map[common.Address]config.GenesisAllocation)
	alloc[crypto.PubkeyToAddress(key.PublicKey)] = config.GenesisAllocation{
		Balance: balance,
	}

	chain, app, pool := newBlockchain(true, alloc, -1, -1)
	app.State.AddBalance(crypto.PubkeyToAddress(key.PublicKey), balance)

	app.State.IncEpoch()

	app.Commit(nil)

	// need to emulate new block
	chain.Head.ProposedHeader.Height++

	app.Commit(nil)
	err := pool.Add(getTx(1, 1, key))
	require.NoError(t, err)

	err = pool.Add(getTx(1, 0, key))
	require.Error(t, err)
}

func TestTxPool_ResetTo(t *testing.T) {
	require := require.New(t)
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)
	key2, _ := crypto.GenerateKey()
	addr2 := crypto.PubkeyToAddress(key2.PublicKey)

	alloc := make(map[common.Address]config.GenesisAllocation)
	alloc[addr] = config.GenesisAllocation{
		Balance: getAmount(100),
	}
	alloc[addr2] = config.GenesisAllocation{
		Balance: getAmount(20),
	}

	_, app, pool := newBlockchain(true, alloc, -1, -1)

	tx1 := getTypedTxWithAmount(1, 1, key, types.SendTx, getAmount(50))
	err := pool.Add(tx1)
	require.NoError(err)

	tx2 := getTypedTxWithAmount(2, 1, key, types.SendTx, getAmount(60))
	err = pool.Add(tx2)
	require.NoError(err)

	// will be removed because of balance
	tx3 := getTypedTxWithAmount(3, 1, key, types.SendTx, getAmount(70))
	err = pool.Add(tx3)
	require.NoError(err)

	// will be removed because of nonce hole
	tx4 := getTypedTxWithAmount(4, 1, key, types.SendTx, getAmount(20))
	err = pool.Add(tx4)
	require.NoError(err)

	tx5 := getTypedTxWithAmount(1, 1, key2, types.SendTx, getAmount(15))
	err = pool.Add(tx5)
	require.NoError(err)

	// will be removed because of past epoch
	tx6 := getTypedTxWithAmount(2, 0, key2, types.SendTx, getAmount(15))
	err = pool.Add(tx6)
	require.NoError(err)

	// will be removed because mined
	tx7 := getTypedTxWithAmount(1, 0, key2, types.SendTx, getAmount(15))
	err = pool.Add(tx7)
	require.NoError(err)

	app.State.SubBalance(addr, getAmount(35))
	app.State.IncEpoch()

	app.Commit(nil)

	pool.ResetTo(&types.Block{
		Header: &types.Header{
			ProposedHeader: &types.ProposedHeader{
				Height: 2,
			},
		},
		Body: &types.Body{
			Transactions: []*types.Transaction{tx7},
		},
	})

	txs := pool.GetPendingTransaction()

	require.Equal(3, len(txs))
	require.Contains(txs, tx1)
	require.Contains(txs, tx2)
	require.Contains(txs, tx5)

	require.Equal(uint32(2), app.NonceCache.GetNonce(addr, 1))
	require.Equal(uint32(1), app.NonceCache.GetNonce(addr2, 1))
}

func TestTxPool_BuildBlockTransactionsWithPriorityTypes(t *testing.T) {
	balance := new(big.Int).Mul(common.DnaBase, big.NewInt(100))
	alloc := make(map[common.Address]config.GenesisAllocation)

	var keys []*ecdsa.PrivateKey

	var addresses []common.Address

	for i := 0; i < 8; i++ {
		key, _ := crypto.GenerateKey()
		keys = append(keys, key)
		address := crypto.PubkeyToAddress(key.PublicKey)
		addresses = append(addresses, address)

		alloc[crypto.PubkeyToAddress(key.PublicKey)] = config.GenesisAllocation{
			Balance: balance,
			State:   uint8(state.Verified),
		}
	}

	_, app, pool := newBlockchain(true, alloc, -1, -1)

	// Current epoch = 1
	app.State.IncEpoch()
	app.Commit(nil)

	addressIndex := 0
	// Prev epoch
	app.State.SetEpoch(addresses[addressIndex], 0)
	pool.Add(getTypedTx(1, 0, keys[addressIndex], types.SendTx))

	// Current epoch and wrong start nonce
	addressIndex++
	app.State.SetEpoch(addresses[addressIndex], 1)
	app.State.SetNonce(addresses[addressIndex], 1)
	pool.Add(getTypedTx(3, 1, keys[addressIndex], types.SendTx))
	pool.Add(getTypedTx(4, 1, keys[addressIndex], types.SendTx))
	pool.Add(getTypedTx(5, 1, keys[addressIndex], types.SendTx))

	// Priority tx after size limit
	addressIndex++
	for i := 0; i < 10596; i++ {
		pool.Add(getTypedTx(uint32(i+1), 1, keys[addressIndex], types.SendTx))
	}
	pool.Add(getTypedTx(10597, 1, keys[addressIndex], types.EvidenceTx))

	addressIndex++
	app.State.SetEpoch(addresses[addressIndex], 1)
	app.State.SetNonce(addresses[addressIndex], 3)
	pool.Add(getTypedTx(4, 1, keys[addressIndex], types.SendTx))
	pool.Add(getTypedTx(5, 1, keys[addressIndex], types.SendTx))

	addressIndex++
	app.State.SetEpoch(addresses[addressIndex], 1)
	app.State.SetNonce(addresses[addressIndex], 4)
	pool.Add(getTypedTx(5, 1, keys[addressIndex], types.SendTx))
	pool.Add(getTypedTx(7, 1, keys[addressIndex], types.SendTx))

	addressIndex++
	app.State.SetEpoch(addresses[addressIndex], 1)
	app.State.SetNonce(addresses[addressIndex], 2)
	pool.Add(getTypedTx(3, 1, keys[addressIndex], types.EvidenceTx))
	pool.Add(getTypedTx(4, 1, keys[addressIndex], types.SendTx))
	pool.Add(getTypedTx(5, 1, keys[addressIndex], types.SubmitLongAnswersTx))
	pool.Add(getTypedTx(6, 1, keys[addressIndex], types.SendTx))
	pool.Add(getTypedTx(8, 1, keys[addressIndex], types.SendTx))

	addressIndex++
	app.State.SetEpoch(addresses[addressIndex], 1)
	app.State.SetNonce(addresses[addressIndex], 1)
	pool.Add(getTypedTx(2, 1, keys[addressIndex], types.SendTx))
	pool.Add(getTypedTx(3, 1, keys[addressIndex], types.SendTx))
	pool.Add(getTypedTx(4, 1, keys[addressIndex], types.SubmitLongAnswersTx))

	addressIndex++
	app.State.SetEpoch(addresses[addressIndex], 1)
	pool.Add(getTypedTx(1, 1, keys[addressIndex], types.SendTx))
	pool.Add(getTypedTx(6, 1, keys[addressIndex], types.SubmitLongAnswersTx))

	// when
	result := pool.BuildBlockTransactions()

	// then
	require.Equal(t, 10596, len(result))
	sender, _ := types.Sender(result[0])
	require.Equal(t, addresses[5], sender)
	require.Equal(t, uint32(3), result[0].AccountNonce)

	sender, _ = types.Sender(result[1])
	require.Equal(t, addresses[6], sender)
	require.Equal(t, uint32(2), result[1].AccountNonce)
	sender, _ = types.Sender(result[2])
	require.Equal(t, addresses[6], sender)
	require.Equal(t, uint32(3), result[2].AccountNonce)
	sender, _ = types.Sender(result[3])
	require.Equal(t, addresses[6], sender)
	require.Equal(t, uint32(4), result[3].AccountNonce)

	sender, _ = types.Sender(result[4])
	require.Equal(t, addresses[5], sender)
	require.Equal(t, uint32(4), result[4].AccountNonce)
	sender, _ = types.Sender(result[5])
	require.Equal(t, addresses[5], sender)
	require.Equal(t, uint32(5), result[5].AccountNonce)

	require.Equal(t, uint32(1), result[6].AccountNonce)
	require.Equal(t, uint32(1), result[7].AccountNonce)
	require.Equal(t, uint32(2), result[8].AccountNonce)
	require.Equal(t, uint32(3), result[9].AccountNonce)
	require.Equal(t, uint32(4), result[10].AccountNonce)
	require.Equal(t, uint32(4), result[11].AccountNonce)
	require.Equal(t, uint32(5), result[12].AccountNonce)
	require.Equal(t, uint32(5), result[13].AccountNonce)
	require.Equal(t, uint32(5), result[14].AccountNonce)
	require.Equal(t, uint32(6), result[15].AccountNonce)
	require.Equal(t, uint32(6), result[16].AccountNonce)

	for i := 17; i < len(result); i++ {
		// Start with nonce=6
		require.Equal(t, uint32(i-10), result[i].AccountNonce)
	}
}

func getTx(nonce uint32, epoch uint16, key *ecdsa.PrivateKey) *types.Transaction {
	return getTypedTx(nonce, epoch, key, types.SendTx)
}

func getTypedTxWithAmount(nonce uint32, epoch uint16, key *ecdsa.PrivateKey, txType types.TxType, amount *big.Int) *types.Transaction {

	addr := crypto.PubkeyToAddress(key.PublicKey)
	to := &addr

	if txType == types.EvidenceTx || txType == types.SubmitShortAnswersTx ||
		txType == types.SubmitLongAnswersTx || txType == types.SubmitAnswersHashTx || txType == types.SubmitFlipTx {
		to = nil
	}

	tx := types.Transaction{
		AccountNonce: nonce,
		Type:         txType,
		To:           to,
		Amount:       amount,
		Epoch:        epoch,
	}

	signedTx, _ := types.SignTx(&tx, key)

	return signedTx
}

//func getAmount(amount int64) *big.Int {
//	return new(big.Int).Mul(common.DnaBase, big.NewInt(amount))
//}

func getTypedTx(nonce uint32, epoch uint16, key *ecdsa.PrivateKey, txType types.TxType) *types.Transaction {
	return getTypedTxWithAmount(nonce, epoch, key, txType, new(big.Int))
}

func newBlockchain(withIdentity bool, alloc map[common.Address]config.GenesisAllocation, totalTxLimit int, addrTxLimit int) (*blockchain.Blockchain, *appstate.AppState, *mempool.TxPool) {
	conf := blockchain.GetDefaultConsensusConfig(false)
	conf.MinFeePerByte = big.NewInt(0)
	return blockchain.NewTestBlockchainWithConfig(withIdentity, conf, &config.ValidationConfig{}, alloc, totalTxLimit, addrTxLimit)
}
