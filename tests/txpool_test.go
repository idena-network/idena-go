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

	_, _, pool := blockchain.NewTestBlockchainWithTxLimits(true, alloc, 3, 2)

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
