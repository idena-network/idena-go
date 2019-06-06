package mempool

import (
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/eventbus"
	"idena-go/core/appstate"
	"idena-go/tests"
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

func getPool() *TxPool {
	bus := eventbus.New()
	appState := appstate.NewAppState(db.NewMemDB(), bus)
	return NewTxPool(appState, bus, -1, -1)
}
