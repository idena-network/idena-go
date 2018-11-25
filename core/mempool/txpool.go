package mempool

import (
	"idena-go/blockchain/types"
	"idena-go/blockchain/validation"
	"idena-go/common"
	cstate "idena-go/consensus/state"
	"idena-go/core/state"
	"idena-go/log"
	"sync"
)

type TxPool struct {
	penging        map[common.Hash]*types.Transaction
	currentState   *state.StateDB
	txSubscription chan *types.Transaction
	mutex          *sync.Mutex
	consensusState *cstate.ConsensusState
	log            log.Logger
}

func NewTxPool(consensusState *cstate.ConsensusState, state *state.StateDB) *TxPool {
	return &TxPool{
		penging:        make(map[common.Hash]*types.Transaction),
		mutex:          &sync.Mutex{},
		currentState:   state,
		consensusState: consensusState,
		log:            log.New(),
	}
}

func (txpool *TxPool) Add(tx *types.Transaction) {

	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()

	hash := tx.Hash()

	if _, ok := txpool.penging[hash]; ok {
		return
	}

	if err := validation.ValidateTx(txpool.consensusState, txpool.currentState, tx); err != nil {
		log.Warn("Tx is not valid", "hash", tx.Hash().Hex(), "err", err)
		return
	}

	txpool.penging[hash] = tx

	txpool.txSubscription <- tx
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
