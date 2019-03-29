package tests

import (
	"crypto/ecdsa"
	"github.com/stretchr/testify/require"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/config"
	"idena-go/crypto"
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

	_, app, pool := blockchain.NewTestBlockchain(true, alloc)

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
	app.Commit()

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
	key, _ := crypto.GenerateKey()

	balance := new(big.Int).Mul(common.DnaBase, big.NewInt(100))

	alloc := make(map[common.Address]config.GenesisAllocation)
	alloc[crypto.PubkeyToAddress(key.PublicKey)] = config.GenesisAllocation{
		Balance: balance,
	}

	chain, app, pool := blockchain.NewTestBlockchain(true, alloc)
	app.State.AddBalance(crypto.PubkeyToAddress(key.PublicKey), balance)

	app.State.IncEpoch()

	app.State.Commit(true)

	// need to emulate new block
	chain.Head.ProposedHeader.Height++

	err := pool.Add(getTx(1, 1, key))
	require.NoError(t, err)

	err = pool.Add(getTx(1, 0, key))
	require.Error(t, err)
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
