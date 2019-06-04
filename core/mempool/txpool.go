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

var (
	DuplicateTxError = errors.New("tx with same hash already exists")
)

type TxPool struct {
	pending        map[common.Hash]*types.Transaction
	pendingPerAddr map[common.Address]map[common.Hash]*types.Transaction
	totalTxLimit   int
	addrTxLimit    int
	txSubscription chan *types.Transaction
	mutex          *sync.Mutex
	appState       *appstate.AppState
	log            log.Logger
	head           *types.Header
	bus            eventbus.Bus
}

func NewTxPool(appState *appstate.AppState, bus eventbus.Bus, totalTxLimit int, addrTxLimit int) *TxPool {
	return &TxPool{
		pending:        make(map[common.Hash]*types.Transaction),
		pendingPerAddr: make(map[common.Address]map[common.Hash]*types.Transaction),
		totalTxLimit:   totalTxLimit,
		addrTxLimit:    addrTxLimit,
		mutex:          &sync.Mutex{},
		appState:       appState,
		log:            log.New(),
		bus:            bus,
	}
}

func (txpool *TxPool) Initialize(head *types.Header) {
	txpool.head = head
}

func (txpool *TxPool) Add(tx *types.Transaction) error {

	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()

	if err := txpool.checkTotalTxLimit(); err != nil {
		return err
	}

	sender, _ := types.Sender(tx)

	if err := txpool.checkAddrTxLimit(sender); err != nil {
		return err
	}

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
	senderPending := txpool.pendingPerAddr[sender]
	if senderPending == nil {
		senderPending = make(map[common.Hash]*types.Transaction)
		txpool.pendingPerAddr[sender] = senderPending
	}
	senderPending[hash] = tx

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

func (txpool *TxPool) GetTx(hash common.Hash) *types.Transaction {
	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()

	tx, ok := txpool.pending[hash]
	if ok {
		return tx
	}
	return nil
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

	sender, _ := types.Sender(transaction)
	senderPending := txpool.pendingPerAddr[sender]
	delete(senderPending, transaction.Hash())
	if len(senderPending) == 0 {
		delete(txpool.pendingPerAddr, sender)
	}
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

func (txpool *TxPool) checkTotalTxLimit() error {
	if txpool.totalTxLimit > 0 && len(txpool.pending) >= txpool.totalTxLimit {
		return errors.New("tx queue max size reached")
	}
	return nil
}

func (txpool *TxPool) checkAddrTxLimit(sender common.Address) error {
	if txpool.addrTxLimit > 0 && len(txpool.pendingPerAddr[sender]) >= txpool.addrTxLimit {
		return errors.New("address tx queue max size reached")
	}
	return nil
}
