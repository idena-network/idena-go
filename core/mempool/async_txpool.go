package mempool

import (
	"errors"
	"fmt"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
)

const batchSize = 1000

type AsyncTxPool struct {
	txPool *TxPool
	queue  chan *types.Transaction
}

func (pool *AsyncTxPool) IsSyncing() bool {
	return pool.txPool.IsSyncing()
}

func NewAsyncTxPool(txPool *TxPool) *AsyncTxPool {
	pool := &AsyncTxPool{
		txPool: txPool,
		queue:  make(chan *types.Transaction, 10000),
	}
	go pool.loop()
	return pool
}

func (pool *AsyncTxPool) AddInternalTx(tx *types.Transaction) error {
	panic("not implemented")
}

func (pool *AsyncTxPool) AddExternalTxs(txType validation.TxType, txs ...*types.Transaction) error {
	skipped := 0
	for _, tx := range txs {
		select {
		case pool.queue <- tx:
		default:
			skipped++
		}
	}
	if skipped > 0 {
		return errors.New(fmt.Sprintf("%v txs skipped", skipped))
	}
	return nil
}

func (pool *AsyncTxPool) GetPendingTransaction(noFilter bool, id common.ShardId, count bool) []*types.Transaction {
	return pool.txPool.GetPendingTransaction(noFilter, id, count)
}

func (pool *AsyncTxPool) loop() {
	for {

		batch := make([]*types.Transaction, 1)
		batch[0] = <-pool.queue

	batchLoop:
		for i := 0; i < batchSize-1; i++ {
			select {
			case tx := <-pool.queue:
				batch = append(batch, tx)
			default:
				break batchLoop
			}
		}
		pool.txPool.AddExternalTxs(validation.InboundTx, batch...)
	}
}
