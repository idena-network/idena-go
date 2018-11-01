package mempool

import (
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/validators"
	"idena-go/crypto"
	"sync"
)

type TxPool struct {
	penging    map[common.Hash]*types.Transaction
	validators *validators.ValidatorsSet

	txSubscription chan *types.Transaction
	mutex          *sync.Mutex
}

func NewTxPool(validators *validators.ValidatorsSet) *TxPool {
	return &TxPool{
		validators: validators,
		penging:    make(map[common.Hash]*types.Transaction),
		mutex:      &sync.Mutex{},
	}
}

func (txpool *TxPool) Add(transaction *types.Transaction) {

	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()

	if txpool.validators.Contains(transaction.PubKey) {
		return
	}
	if _, ok := txpool.penging[transaction.Hash()]; ok {
		return
	}

	hash := transaction.Hash()
	if !crypto.VerifySignature(transaction.PubKey, hash[:], transaction.Signature[:len(transaction.Signature)-1]) {
		return
	}

	txpool.penging[hash] = transaction

	txpool.txSubscription <- transaction
}

func (txpool *TxPool) Subscribe(transactions chan *types.Transaction) {
	txpool.txSubscription = transactions
}
func (txpool *TxPool) GetPendingTransaction() []*types.Transaction {
	var list []*types.Transaction

	for _, tx := range txpool.penging {
		list = append(list, tx)
	}
	return list
}
func (txpool *TxPool) Remove(transaction *types.Transaction) {
	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()
	delete(txpool.penging, transaction.Hash())
}
