package mempool

import (
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/validators"
	"idena-go/crypto"
)

type TxPool struct {
	penging    map[common.Hash]*types.Transaction
	validators *validators.ValidatorsSet

	txSubscription chan *types.Transaction
}

func NewTxPool(validators *validators.ValidatorsSet) *TxPool {
	return &TxPool{
		validators: validators,
	}
}

func (txpool *TxPool) Add(transaction *types.Transaction) {
	if txpool.validators.Contains(transaction.PubKey) {
		return
	}
	if _, ok := txpool.penging[transaction.Hash()]; ok {
		return
	}

	hash := transaction.Hash()
	if !crypto.VerifySignature(transaction.PubKey, hash[:], transaction.Signature) {
		return
	}

	txpool.penging[hash] = transaction

	txpool.txSubscription <- transaction
}

func (txpool *TxPool) Subscribe(transactions chan *types.Transaction) {
	txpool.txSubscription = transactions
}
func (txpool *TxPool) GetPendingTransaction() []*types.Transaction {
	var list [] *types.Transaction
	for _, tx := range txpool.penging {
		list = append(list, tx)
	}
	return list
}
