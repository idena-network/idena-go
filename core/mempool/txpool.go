package mempool

import (
	"errors"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/log"
	"sort"
	"sync"
)

const (
	BlockBodySize  = 1024 * 1024
	MaxDeferredTxs = 100
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
	knownDeferredTxs mapset.Set
	deferredTxs      []*types.Transaction
	pending          map[common.Hash]*types.Transaction
	pendingPerAddr   map[common.Address]map[common.Hash]*types.Transaction
	totalTxLimit     int
	addrTxLimit      int
	txSubscription   chan *types.Transaction
	mutex            *sync.Mutex
	appState         *appstate.AppState
	log              log.Logger
	head             *types.Header
	bus              eventbus.Bus
	isSyncing        bool //indicates about blockchain's syncing
}

func NewTxPool(appState *appstate.AppState, bus eventbus.Bus, totalTxLimit int, addrTxLimit int) *TxPool {
	return &TxPool{
		pending:          make(map[common.Hash]*types.Transaction),
		pendingPerAddr:   make(map[common.Address]map[common.Hash]*types.Transaction),
		knownDeferredTxs: mapset.NewSet(),
		totalTxLimit:     totalTxLimit,
		addrTxLimit:      addrTxLimit,
		mutex:            &sync.Mutex{},
		appState:         appState,
		log:              log.New(),
		bus:              bus,
	}
}

func (txpool *TxPool) Initialize(head *types.Header) {
	txpool.head = head
}

func (txpool *TxPool) addDeferredTx(tx *types.Transaction) {
	if txpool.knownDeferredTxs.Contains(tx.Hash()) {
		return
	}
	txpool.deferredTxs = append(txpool.deferredTxs, tx)
	if len(txpool.deferredTxs) > MaxDeferredTxs {
		txpool.deferredTxs = txpool.deferredTxs[1:]
	}
	txpool.knownDeferredTxs.Add(tx.Hash())
}

func (txpool *TxPool) Validate(tx *types.Transaction) error {

	if err := txpool.checkTotalTxLimit(); err != nil {
		return err
	}

	hash := tx.Hash()

	if _, ok := txpool.pending[hash]; ok {
		return DuplicateTxError
	}

	sender, _ := types.Sender(tx)

	if err := txpool.checkAddrTxLimit(sender); err != nil {
		return err
	}

	if err := txpool.checkAddrCeremonyTx(tx); err != nil {
		return err
	}

	appState := txpool.appState.Readonly(txpool.head.Height())

	return validation.ValidateTx(appState, tx, true)
}

func (txpool *TxPool) Add(tx *types.Transaction) error {

	txpool.mutex.Lock()
	defer txpool.mutex.Unlock()

	if txpool.isSyncing {
		txpool.addDeferredTx(tx)
		return nil
	}

	if err := txpool.Validate(tx); err != nil {
		if err != DuplicateTxError {
			log.Warn("Tx is not valid", "hash", tx.Hash().Hex(), "err", err)
		}
		return err
	}

	hash := tx.Hash()
	sender, _ := types.Sender(tx)

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

	appState := txpool.appState.Readonly(txpool.head.Height())

	for _, tx := range pending {
		if tx.Epoch < globalEpoch {
			txpool.Remove(tx)
			continue
		}
		if tx.Epoch > globalEpoch {
			continue
		}
		if err := validation.ValidateTx(appState, tx, false); err != nil {
			txpool.Remove(tx)
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

func (txpool *TxPool) checkAddrCeremonyTx(tx *types.Transaction) error {
	if !priorityTypes[tx.Type] {
		return nil
	}
	sender, _ := types.Sender(tx)
	for _, existingTx := range txpool.pendingPerAddr[sender] {
		if existingTx.Type == tx.Type {
			return errors.New("multiple ceremony transaction")
		}
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

func (txpool *TxPool) StartSync() {
	txpool.isSyncing = true
}

func (txpool *TxPool) StopSync(block *types.Block) {
	txpool.isSyncing = false
	txpool.ResetTo(block)
	for _, tx := range txpool.deferredTxs {
		txpool.Add(tx)
	}
	txpool.deferredTxs = make([]*types.Transaction, 0)
	txpool.knownDeferredTxs.Clear()
}
