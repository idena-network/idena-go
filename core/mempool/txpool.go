package mempool

import (
	"errors"
	"idena-go/blockchain/types"
	"idena-go/blockchain/validation"
	"idena-go/common"
	"idena-go/core/appstate"
	"idena-go/core/state"
	"idena-go/log"
	"sort"
	"sync"
)

const (
	BlockBodySize = 1024 * 1024
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

func (txpool *TxPool) Add(tx *types.Transaction) error {

	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()

	hash := tx.Hash()
	sender, _ := types.Sender(tx)

	if _, ok := txpool.pending[hash]; ok {
		return errors.New("tx with same hash already exists")
	}

	if err := validation.ValidateTx(txpool.appState, tx); err != nil {
		log.Warn("Tx is not valid", "hash", tx.Hash().Hex(), "err", err)
		return err
	}

	txpool.pending[hash] = tx

	if txpool.appState.State.Epoch() == tx.Epoch {
		txpool.appState.NonceCache.SetNonce(sender, tx.AccountNonce)
	}

	select {
	case txpool.txSubscription <- tx:
	default:
	}
	return nil
}

func (txpool *TxPool) Subscribe(transactions chan *types.Transaction) {
	txpool.txSubscription = transactions
}
func (txpool *TxPool) GetPendingTransaction() []*types.Transaction {
	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()

	var list []*types.Transaction

	for _, tx := range txpool.pending {
		list = append(list, tx)
	}
	return list
}

func (txpool *TxPool) BuildBlockTransactions() []*types.Transaction {
	var list []*types.Transaction
	var result []*types.Transaction

	txpool.mutex.Lock()
	for _, tx := range txpool.pending {
		list = append(list, tx)
	}
	txpool.mutex.Unlock()

	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Epoch < list[j].Epoch {
			return true
		}
		if list[i].Epoch > list[j].Epoch {
			return false
		}
		return list[i].AccountNonce < list[j].AccountNonce
	})

	currentNonces := make(map[common.Address]uint32)
	globalEpoch := txpool.appState.State.Epoch()
	size := 0
	for _, tx := range list {
		if size > BlockBodySize {
			break
		}
		sender, _ := types.Sender(tx)
		if tx.Epoch < globalEpoch {
			continue
		}
		// do not process transactions from future epoch
		if tx.Epoch > globalEpoch {
			break
		}
		if _, ok := currentNonces[sender]; !ok {
			currentNonces[sender] = txpool.appState.State.GetNonce(sender)
			if txpool.appState.State.GetEpoch(sender) < globalEpoch {
				currentNonces[sender] = 0
			}
		}
		if currentNonces[sender]+1 == tx.AccountNonce {
			result = append(result, tx)
			size += tx.Size()
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
	globalEpoch := txpool.appState.State.Epoch()

	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()

	for _, tx := range txpool.pending {
		if tx.Epoch < globalEpoch {
			txpool.Remove(tx)
		}
		if tx.Epoch > globalEpoch {
			continue
		}
		sender, _ := types.Sender(tx)
		currentCache := txpool.appState.NonceCache.GetNonce(sender)
		if tx.AccountNonce >= currentCache {
			txpool.appState.NonceCache.SetNonce(sender, tx.AccountNonce)
		}
	}
}
