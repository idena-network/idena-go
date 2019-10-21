package mempool

import "github.com/idena-network/idena-go/blockchain/types"

type AsyncTxPool struct {
	txPool *TxPool
	queue  chan *types.Transaction
}

func NewAsyncTxPool(txPool *TxPool) *AsyncTxPool {
	pool := &AsyncTxPool{
		txPool: txPool,
		queue:  make(chan *types.Transaction, 2000),
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
		select {
		case tx := <-pool.queue:
			pool.txPool.Add(tx)
		}
	}
}
