package mempool

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-db"
	"math/big"
	"testing"
)

func TestTxPool_addDeferredTx(t *testing.T) {
	bus := eventbus.New()
	appState := appstate.NewAppState(db.NewMemDB(), bus)

	key, _ := crypto.GenerateKey()
	secStore := secstore.NewSecStore()
	secStore.AddKey(crypto.FromECDSA(key))
	pool := NewTxPool(appState, bus, &config.Mempool{TxPoolQueueSlots: -1, TxPoolAddrQueueLimit: -1}, big.NewInt(0))
	r := require.New(t)

	key, _ = crypto.GenerateKey()

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
	r.Len(pool.executableTxs, 1)
}

func getPool() *TxPool {
	bus := eventbus.New()
	appState := appstate.NewAppState(db.NewMemDB(), bus)

	key, _ := crypto.GenerateKey()
	secStore := secstore.NewSecStore()
	secStore.AddKey(crypto.FromECDSA(key))

	return NewTxPool(appState, bus, &config.Mempool{
		TxPoolExecutableSlots: -1,
		TxPoolQueueSlots:      -1,
	}, big.NewInt(0))
}
