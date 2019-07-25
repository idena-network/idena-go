package mempool

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/tests"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-cmn/db"
	"math/big"
	"testing"
)

func TestTxPool_checkTotalTxLimit(t *testing.T) {
	pool := getPool()

	r := require.New(t)

	tr := &types.Transaction{
		AccountNonce: 1,
	}
	pool.pending[tr.Hash()] = tr
	r.Nil(pool.checkTotalTxLimit())

	pool.totalTxLimit = 1
	r.Error(pool.checkTotalTxLimit())

	pool.totalTxLimit = 2
	r.Nil(pool.checkTotalTxLimit())

	tr = &types.Transaction{
		AccountNonce: 2,
	}
	pool.pending[tr.Hash()] = tr
	r.Error(pool.checkTotalTxLimit())
}

func TestTxPool_checkAddrTxLimit(t *testing.T) {
	pool := getPool()

	r := require.New(t)

	address := tests.GetRandAddr()
	r.Nil(pool.checkAddrTxLimit(address))

	pool.addrTxLimit = 1
	r.Nil(pool.checkAddrTxLimit(address))

	pool.pendingPerAddr[address] = make(map[common.Hash]*types.Transaction)
	tr := &types.Transaction{}
	pool.pendingPerAddr[address][tr.Hash()] = tr
	r.Error(pool.checkAddrTxLimit(address))
}

func TestTxPool_addDeferredTx(t *testing.T) {
	bus := eventbus.New()
	appState := appstate.NewAppState(db.NewMemDB(), bus)
	pool := NewTxPool(appState, bus, -1, -1)
	r := require.New(t)

	key, _ := crypto.GenerateKey()
	address := crypto.PubkeyToAddress(key.PublicKey)

	balance := new(big.Int).Mul(common.DnaBase, big.NewInt(100))

	appState.State.SetBalance(address, balance)
	appState.Commit(nil)
	appState.Initialize(0)
	pool.StartSync()

	tx := &types.Transaction{
		AccountNonce: 1,
		To:           &address,
		Epoch:        0,
		Type:         types.SendTx,
		Amount:       new(big.Int).Mul(common.DnaBase, big.NewInt(1)),
	}

	tx, err := types.SignTx(tx, key)
	r.NoError(err)

	err = pool.Add(tx)
	r.NoError(err)
	r.Len(pool.deferredTxs, 1)
	r.True(pool.knownDeferredTxs.Contains(tx.Hash()))

	pool.StopSync(&types.Block{
		Header: &types.Header{
			EmptyBlockHeader: &types.EmptyBlockHeader{
				Height: 0,
			},
		},
		Body: &types.Body{},
	})

	r.Len(pool.deferredTxs, 0)
	r.True(pool.knownDeferredTxs.Cardinality() == 0)
	r.Len(pool.pending, 1)
}

func getPool() *TxPool {
	bus := eventbus.New()
	appState := appstate.NewAppState(db.NewMemDB(), bus)
	return NewTxPool(appState, bus, -1, -1)
}
