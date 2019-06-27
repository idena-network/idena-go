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

	priorityTypes = map[types.TxType]bool{
		types.SubmitAnswersHashTx:  true,
		types.SubmitShortAnswersTx: true,
		types.SubmitLongAnswersTx:  true,
		types.EvidenceTx:           true,
	}
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

	appState := txpool.appState.Readonly(txpool.head.Height())

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

	txpool.appState.NonceCache.SetNonce(sender, tx.Epoch, tx.AccountNonce)

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
	ctx := txpool.createBuildingContext()
	ctx.addPriorityTxsToBlock()
	ctx.addTxsToBlock()
	return ctx.blockTxs
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

func (txpool *TxPool) createBuildingContext() *buildingContext {
	curNoncesPerSender := make(map[common.Address]uint32)
	var txs []*types.Transaction
	txpool.mutex.Lock()
	globalEpoch := txpool.appState.State.Epoch()
	withPriorityTx := false
	for _, tx := range txpool.pending {
		if tx.Epoch != globalEpoch {
			continue
		}
		txs = append(txs, tx)
		withPriorityTx = withPriorityTx || priorityTypes[tx.Type]
	}
	for sender := range txpool.pendingPerAddr {
		if txpool.appState.State.GetEpoch(sender) < globalEpoch {
			curNoncesPerSender[sender] = 0
		} else {
			curNoncesPerSender[sender] = txpool.appState.State.GetNonce(sender)
		}
	}
	txpool.mutex.Unlock()

	sort.SliceStable(txs, func(i, j int) bool {
		return txs[i].AccountNonce < txs[j].AccountNonce
	})

	var priorityTxs []*types.Transaction
	var sortedTxsPerSender map[common.Address][]*types.Transaction
	if withPriorityTx {
		sortedTxsPerSender = make(map[common.Address][]*types.Transaction)
		for _, tx := range txs {
			if priorityTypes[tx.Type] {
				priorityTxs = append(priorityTxs, tx)
			}
			sender, _ := types.Sender(tx)
			sortedTxsPerSender[sender] = append(sortedTxsPerSender[sender], tx)
		}
	}

	return newBuildingContext(txs, priorityTxs, sortedTxsPerSender, curNoncesPerSender)
}
