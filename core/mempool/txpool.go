package mempool

import (
	"errors"
	"idena-go/blockchain/types"
	"idena-go/blockchain/validation"
	"idena-go/common"
	"idena-go/common/eventbus"
	"idena-go/core/appstate"
	"idena-go/core/state"
	"idena-go/events"
	"idena-go/log"
	"sort"
	"sync"
)

const (
	BlockBodySize = 1024 * 1024
)

var(
	DuplicateTxError = errors.New("tx with same hash already exists")
)

type TxPool struct {
	pending        map[common.Hash]*types.Transaction
	txSubscription chan *types.Transaction
	mutex          *sync.Mutex
	appState       *appstate.AppState
	log            log.Logger
	head           *types.Header
	bus            eventbus.Bus
}

func NewTxPool(appState *appstate.AppState, bus eventbus.Bus) *TxPool {
	return &TxPool{
		pending:  make(map[common.Hash]*types.Transaction),
		mutex:    &sync.Mutex{},
		appState: appState,
		log:      log.New(),
		bus:      bus,
	}
}

func (txpool *TxPool) Initialize(head *types.Header) {
	txpool.head = head
}

func (txpool *TxPool) Add(tx *types.Transaction) error {

	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()

	hash := tx.Hash()

	if _, ok := txpool.pending[hash]; ok {
		return DuplicateTxError
	}

	appState := txpool.appState.ForCheck(txpool.head.Height())

	if err := validation.ValidateTx(appState, tx, true); err != nil {
		log.Warn("Tx is not valid", "hash", tx.Hash().Hex(), "err", err)
		return err
	}

	txpool.pending[hash] = tx

	sender, _ := types.Sender(tx)

	appState.NonceCache.SetNonce(sender, tx.Epoch, tx.AccountNonce)

	txpool.bus.Publish(&events.NewTxEvent{
		Tx: tx,
	})

	return nil
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

	txpool.head = block.Header

	for _, tx := range block.Body.Transactions {
		txpool.Remove(tx)
	}

	txpool.appState.NonceCache = state.NewNonceCache(txpool.appState.State)
	globalEpoch := txpool.appState.State.Epoch()

	pending := txpool.GetPendingTransaction()

	for _, tx := range pending {
		if tx.Epoch < globalEpoch {
			txpool.Remove(tx)
		}
		if tx.Epoch > globalEpoch {
			continue
		}
		sender, _ := types.Sender(tx)
		txpool.appState.NonceCache.SetNonce(sender, tx.Epoch, tx.AccountNonce)
	}
}
