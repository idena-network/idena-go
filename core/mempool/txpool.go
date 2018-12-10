package mempool

import (
	"idena-go/blockchain/types"
	"idena-go/blockchain/validation"
	"idena-go/common"
	"idena-go/core/appstate"
	"idena-go/core/state"
	"idena-go/log"
	"sort"
	"sync"
)

type TxPool struct {
	pending        map[common.Hash]*types.Transaction
	txSubscription chan *types.Transaction
	mutex          *sync.Mutex
	appState       *appstate.AppState
	log            log.Logger
}

func NewTxPool(appState *appstate.AppState) *TxPool {
	return &TxPool{
		pending:  make(map[common.Hash]*types.Transaction),
		mutex:    &sync.Mutex{},
		appState: appState,
		log:      log.New(),
	}
}

func (txpool *TxPool) Add(tx *types.Transaction) {

	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()

	hash := tx.Hash()
	sender, _ := types.Sender(tx)

	if _, ok := txpool.pending[hash]; ok {
		return
	}

	if err := validation.ValidateTx(txpool.appState, tx); err != nil {
		log.Warn("Tx is not valid", "hash", tx.Hash().Hex(), "err", err)
		return
	}

	txpool.pending[hash] = tx

	txpool.appState.NonceCache.SetNonce(sender, tx.AccountNonce+1)
	select {
	case txpool.txSubscription <- tx:
	default:
	}

}

func (txpool *TxPool) Subscribe(transactions chan *types.Transaction) {
	txpool.txSubscription = transactions
}
func (txpool *TxPool) GetPendingTransaction() []*types.Transaction {
	var list []*types.Transaction

	for _, tx := range txpool.pending {
		list = append(list, tx)
	}
	return list
}

func (txpool *TxPool) BuildBlockTransactions() []*types.Transaction {
	var list []*types.Transaction
	var result []*types.Transaction

	for _, tx := range txpool.pending {
		list = append(list, tx)
	}

	sort.SliceStable(list, func(i, j int) bool {
		return list[i].AccountNonce < list[j].AccountNonce
	})

	currentNonces := make(map[common.Address]uint64)

	for _, tx := range list {
		sender, _ := types.Sender(tx)
		if _, ok := currentNonces[sender]; !ok {
			currentNonces[sender] = txpool.appState.State.GetNonce(sender)
		}
		if currentNonces[sender]+1 == tx.AccountNonce {
			result = append(result, tx)
			currentNonces[sender] = tx.AccountNonce
		}
	}

	return result
}

func (txpool *TxPool) Remove(transaction *types.Transaction) {
	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()
	delete(txpool.pending, transaction.Hash())
}

func (txpool *TxPool) ResetTo(block *types.Block) {
	for _, tx := range block.Body.Transactions {
		txpool.Remove(tx)
	}

	txpool.appState.NonceCache = state.NewNonceCache(txpool.appState.State)

	for _, tx := range txpool.pending {
		sender, _ := types.Sender(tx)
		currentCache := txpool.appState.NonceCache.GetNonce(sender)
		if tx.AccountNonce >= currentCache {
			txpool.appState.NonceCache.SetNonce(sender, tx.AccountNonce+1)
		}
	}
}
