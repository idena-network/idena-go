package mempool

import "github.com/idena-network/idena-go/blockchain/types"

const batchSize = 10

type AsyncTxPool struct {
	txPool *TxPool
	queue  chan *types.Transaction
}

func NewAsyncTxPool(txPool *TxPool) *AsyncTxPool {
	pool := &AsyncTxPool{
		txPool: txPool,
		queue:  make(chan *types.Transaction, 10000),
	}
	go pool.loop()
	return pool
}

func (pool *AsyncTxPool) Add(tx *types.Transaction) error {
	select {
	case pool.queue <- tx:
	default:
	}
	return nil
}

func (pool *AsyncTxPool) GetPendingTransaction() []*types.Transaction {
	return pool.txPool.GetPendingTransaction()
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
		pool.txPool.AddTxs(batch)
	}
}
