package mempool

import (
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/log"
	"github.com/pkg/errors"
	"math/big"
	"sort"
	"sync"
)

const (
	BlockBodySize  = 1024 * 1024
	MaxDeferredTxs = 100
)

var (
	DuplicateTxError = errors.New("tx with same hash already exists")
	MempoolFullError = errors.New("mempool is full")
	priorityTypes    = map[types.TxType]bool{
		types.SubmitAnswersHashTx:  true,
		types.SubmitShortAnswersTx: true,
		types.SubmitLongAnswersTx:  true,
		types.EvidenceTx:           true,
	}
)

type TransactionPool interface {
	Add(tx *types.Transaction) error
	GetPendingTransaction() []*types.Transaction
}

type TxPool struct {
	knownDeferredTxs mapset.Set
	deferredTxs      []*types.Transaction
	all              *txMap
	executableTxs    map[common.Address]*sortedTxs
	pendingTxs       map[common.Address]*txMap
	cfg              *config.Mempool
	txSubscription   chan *types.Transaction
	mutex            *sync.Mutex
	appState         *appstate.AppState
	log              log.Logger
	head             *types.Header
	bus              eventbus.Bus
	isSyncing        bool //indicates about blockchain's syncing
	coinbase         common.Address
	minFeePerByte    *big.Int
}

func NewTxPool(appState *appstate.AppState, bus eventbus.Bus, cfg *config.Mempool, minFeePerByte *big.Int) *TxPool {
	pool := &TxPool{
		all:              newTxMap(-1),
		executableTxs:    make(map[common.Address]*sortedTxs),
		pendingTxs:       make(map[common.Address]*txMap),
		knownDeferredTxs: mapset.NewSet(),
		cfg:              cfg,
		mutex:            &sync.Mutex{},
		appState:         appState,
		log:              log.New(),
		bus:              bus,
		minFeePerByte:    minFeePerByte,
	}

	_ = pool.bus.Subscribe(events.AddBlockEventID,
		func(e eventbus.Event) {
			newBlockEvent := e.(*events.NewBlockEvent)
			pool.head = newBlockEvent.Block.Header
		})
	_ = pool.bus.Subscribe(events.FastSyncCompleted, func(event eventbus.Event) {
		pool.appState.NonceCache.Lock()
		pool.appState.NonceCache.ReloadFallback(pool.appState.State)
		pool.appState.NonceCache.UnLock()
	})
	return pool
}

func (pool *TxPool) Initialize(head *types.Header, coinbase common.Address) {
	pool.head = head
	pool.coinbase = coinbase
}

func (pool *TxPool) addDeferredTx(tx *types.Transaction) {
	if pool.knownDeferredTxs.Contains(tx.Hash()) {
		return
	}
	pool.deferredTxs = append(pool.deferredTxs, tx)
	if len(pool.deferredTxs) > MaxDeferredTxs {
		pool.deferredTxs[0] = nil
		pool.deferredTxs = pool.deferredTxs[1:]
	}
	pool.knownDeferredTxs.Add(tx.Hash())
}

// validate tx as inbound transaction
func (pool *TxPool) Validate(tx *types.Transaction) error {
	if _, ok := pool.all.Get(tx.Hash()); ok {
		return DuplicateTxError
	}
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	if err := pool.checkLimits(tx); err != nil {
		return err
	}
	appState, err := pool.appState.Readonly(pool.head.Height())

	if err != nil {
		return errors.WithMessage(err, "tx can't be validated")
	}
	return pool.validate(tx, appState, validation.InboundTx)
}

func (pool *TxPool) checkLimits(tx *types.Transaction) error {
	if priorityTypes[tx.Type] {
		return pool.checkPriorityTxLimits(tx)
	}
	return pool.checkRegularTxLimits(tx)
}

func (pool *TxPool) checkPriorityTxLimits(tx *types.Transaction) error {
	sender, _ := types.Sender(tx)
	if executable, ok := pool.executableTxs[sender]; ok {
		for _, existingTx := range executable.txs {
			if existingTx.Type == tx.Type {
				return errors.Errorf("multiple ceremony transaction [type=%v] in executable queue", tx.Type)
			}
		}
	}
	if txs, ok := pool.pendingTxs[sender]; ok {
		for _, existingTx := range txs.txs {
			if existingTx.Type == tx.Type {
				return errors.Errorf("multiple ceremony transaction [type=%v] in pending queue", tx.Type)
			}
		}
	}
	return nil
}

func (pool *TxPool) checkRegularTxLimits(tx *types.Transaction) error {
	var totalLimit = 0
	if pool.cfg.TxPoolExecutableSlots < 0 || pool.cfg.TxPoolQueueSlots < 0 {
		totalLimit = -1
	} else {
		totalLimit = pool.cfg.TxPoolExecutableSlots*pool.cfg.TxPoolAddrExecutableLimit +
			pool.cfg.TxPoolQueueSlots*pool.cfg.TxPoolAddrQueueLimit
	}
	if totalLimit > 0 && len(pool.all.txs) >= totalLimit {
		return errors.New("tx queue max size reached")
	}
	sender, _ := types.Sender(tx)

	if byAddr, ok := pool.executableTxs[sender]; ok {
		if byAddr.Full() {
			if pending, ok := pool.pendingTxs[sender]; ok {
				if pending.Full() {
					return MempoolFullError
				}
			}
			if pool.cfg.TxPoolQueueSlots > 0 && len(pool.pendingTxs) >= pool.cfg.TxPoolQueueSlots {
				return MempoolFullError
			}
		}
	}

	return nil
}

func (pool *TxPool) validate(tx *types.Transaction, appState *appstate.AppState, txType validation.TxType) error {
	return validation.ValidateTx(appState, tx, pool.minFeePerByte, txType)
}

func (pool *TxPool) AddTxs(txs []*types.Transaction) {
	appState, err := pool.appState.Readonly(pool.head.Height())

	if err != nil {
		pool.log.Warn("txpool: failed to create readonly appState", "err", err)
		return
	}
	for _, tx := range txs {

		sender, _ := types.Sender(tx)

		if pool.isSyncing && sender != pool.coinbase {
			pool.addDeferredTx(tx)
			continue
		}

		pool.add(tx, appState)
	}
}

func (pool *TxPool) Add(tx *types.Transaction) error {

	sender, _ := types.Sender(tx)

	if pool.isSyncing && sender != pool.coinbase {
		pool.addDeferredTx(tx)
		return nil
	}

	appState, err := pool.appState.Readonly(pool.head.Height())

	if err != nil {
		return errors.WithMessage(err, "tx can't be validated")
	}
	return pool.add(tx, appState)
}

func (pool *TxPool) add(tx *types.Transaction, appState *appstate.AppState) error {
	if _, ok := pool.all.Get(tx.Hash()); ok {
		return DuplicateTxError
	}

	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	if err := pool.checkLimits(tx); err != nil {
		log.Warn("Tx limits", "hash", tx.Hash().Hex(), "err", err)
		return err
	}

	sender, _ := types.Sender(tx)

	if err := pool.validate(tx, appState, validation.InboundTx); err != nil {
		if sender == pool.coinbase {
			log.Warn("Tx is not valid", "hash", tx.Hash().Hex(), "err", err)
		}
		return err
	}

	return pool.put(tx)
}

func (pool *TxPool) putToPending(tx *types.Transaction) error {
	sender, _ := types.Sender(tx)
	set, ok := pool.pendingTxs[sender]
	if !ok {
		if pool.cfg.TxPoolQueueSlots > 0 && len(pool.pendingTxs) >= pool.cfg.TxPoolQueueSlots {
			return MempoolFullError
		}
		set = newTxMap(pool.cfg.TxPoolAddrQueueLimit)
	}
	err := set.Add(tx)
	if err == nil {
		pool.pendingTxs[sender] = set
	}
	return err
}

func (pool *TxPool) put(tx *types.Transaction) error {

	sender, _ := types.Sender(tx)

	executable, ok := pool.executableTxs[sender]
	if !ok {
		executable = newSortedTxs(pool.cfg.TxPoolAddrExecutableLimit)
	}

	isExecutable := true

	if executable.Empty() {
		globalEpoch := pool.appState.State.Epoch()
		nonce := pool.appState.State.GetNonce(sender)
		accountEpoch := pool.appState.State.GetEpoch(sender)
		if accountEpoch < globalEpoch {
			nonce = 0
		}
		if tx.Epoch != globalEpoch || tx.AccountNonce != nonce+1 {
			isExecutable = false
		}
	}
	var err error
	if isExecutable {
		err = executable.Add(tx)
		if err != nil {
			err = pool.putToPending(tx)
		} else {
			pool.executableTxs[sender] = executable
		}
	} else {
		err = pool.putToPending(tx)
	}

	if err != nil {
		return err
	}

	pool.all.Add(tx)

	pool.appState.NonceCache.SetNonce(sender, tx.Epoch, tx.AccountNonce)

	pool.bus.Publish(&events.NewTxEvent{
		Tx:  tx,
		Own: sender == pool.coinbase,
	})
	return nil
}

func (pool *TxPool) GetPendingTransaction() []*types.Transaction {
	return pool.all.List()
}

func (pool *TxPool) GetPendingByAddress(address common.Address) []*types.Transaction {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	var list []*types.Transaction

	if executable, ok := pool.executableTxs[address]; ok {
		for _, tx := range executable.txs {
			list = append(list, tx)
		}
	}

	if pending, ok := pool.pendingTxs[address]; ok {
		for _, tx := range pending.txs {
			list = append(list, tx)
		}
	}

	return list
}

func (pool *TxPool) GetTx(hash common.Hash) *types.Transaction {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	tx, ok := pool.all.Get(hash)
	if ok {
		return tx
	}
	return nil
}

func (pool *TxPool) BuildBlockTransactions() []*types.Transaction {
	ctx := pool.createBuildingContext()
	ctx.addPriorityTxsToBlock()
	ctx.addTxsToBlock()
	return ctx.blockTxs
}

func (pool *TxPool) Remove(transaction *types.Transaction) {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()
	pool.all.Remove(transaction.Hash())

	sender, _ := types.Sender(transaction)

	if executable, ok := pool.executableTxs[sender]; ok {
		executable.Remove(transaction)
		if executable.Empty() {
			delete(pool.executableTxs, sender)
		}
	}

	if pending, ok := pool.pendingTxs[sender]; ok {
		pending.Remove(transaction.Hash())
		if pending.Empty() {
			delete(pool.pendingTxs, sender)
		}
	}
}

func (pool *TxPool) movePendingTxsToExecutable() {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()
	for sender, pending := range pool.pendingTxs {
		executable, ok := pool.executableTxs[sender]
		if !ok {
			executable = newSortedTxs(pool.cfg.TxPoolAddrExecutableLimit)
		}

		for _, tx := range pending.Sorted() {
			if executable.Empty() {
				epoch := pool.appState.State.Epoch()
				nonce := pool.appState.State.GetNonce(sender)
				accountEpoch := pool.appState.State.GetEpoch(sender)
				if accountEpoch < epoch {
					nonce = 0
				}
				if epoch != tx.Epoch || tx.AccountNonce != nonce+1 {
					break
				}
			}
			if executable.Add(tx) == nil {
				pool.executableTxs[sender] = executable
				pending.Remove(tx.Hash())
			} else {
				break
			}
		}
		if pending.Empty() {
			delete(pool.pendingTxs, sender)
		}
	}
}

func (pool *TxPool) ResetTo(block *types.Block) {

	pool.head = block.Header

	for _, tx := range block.Body.Transactions {
		pool.Remove(tx)
	}

	pool.movePendingTxsToExecutable()

	globalEpoch := pool.appState.State.Epoch()

	pool.appState.NonceCache.Lock()

	pool.appState.NonceCache.Clear()

	if err := pool.appState.NonceCache.ReloadFallback(pool.appState.State); err != nil {
		pool.log.Warn("failed to reload nonce cache", "err", err)
	}

	pending := pool.GetPendingTransaction()

	appState, _ := pool.appState.Readonly(pool.head.Height())

	type txError struct {
		nonce uint32
		err   error
	}

	minErrorNonce := make(map[common.Address]txError)

	removingTxs := make(map[common.Hash]*types.Transaction)

	for _, tx := range pending {
		if tx.Epoch != globalEpoch {
			continue
		}

		if err := validation.ValidateTx(appState, tx, pool.minFeePerByte, validation.MempoolTx); err != nil {
			if errors.Cause(err) == validation.InvalidNonce {
				removingTxs[tx.Hash()] = tx
				continue
			}
			sender, _ := types.Sender(tx)
			if n, ok := minErrorNonce[sender]; ok {
				if tx.AccountNonce < n.nonce {
					minErrorNonce[sender] = txError{tx.AccountNonce, err}
				}
			} else {
				minErrorNonce[sender] = txError{tx.AccountNonce, err}
			}
			continue
		}
	}

	for _, tx := range pending {
		if tx.Epoch < globalEpoch {
			removingTxs[tx.Hash()] = tx
			continue
		}
		if tx.Epoch > globalEpoch {
			continue
		}

		sender, _ := types.Sender(tx)

		if txError, ok := minErrorNonce[sender]; ok && tx.AccountNonce >= txError.nonce {
			removingTxs[tx.Hash()] = tx
			if tx.AccountNonce == txError.nonce {
				pool.log.Info("Tx is invalid", "tx", tx.Hash().Hex(), "err", txError.err)
			} else {
				pool.log.Info("Tx removed by nonce", "tx", tx.Hash().Hex())
			}
			continue
		}

		pool.appState.NonceCache.UnsafeSetNonce(sender, tx.Epoch, tx.AccountNonce)
	}

	pool.appState.NonceCache.UnLock()
	for _, tx := range removingTxs {
		pool.Remove(tx)
	}
}

func (pool *TxPool) createBuildingContext() *buildingContext {
	curNoncesPerSender := make(map[common.Address]uint32)
	var txs []*types.Transaction
	pool.mutex.Lock()
	globalEpoch := pool.appState.State.Epoch()
	withPriorityTx := false
	for sender, executable := range pool.executableTxs {
		for _, tx := range executable.txs {
			if tx.Epoch != globalEpoch {
				continue
			}
			txs = append(txs, tx)
			withPriorityTx = withPriorityTx || priorityTypes[tx.Type]
		}
		if pool.appState.State.GetEpoch(sender) < globalEpoch {
			curNoncesPerSender[sender] = 0
		} else {
			curNoncesPerSender[sender] = pool.appState.State.GetNonce(sender)
		}
	}
	pool.mutex.Unlock()

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

	return newBuildingContext(pool.appState, txs, priorityTxs, sortedTxsPerSender, curNoncesPerSender)
}

func (pool *TxPool) StartSync() {
	pool.isSyncing = true
}

func (pool *TxPool) StopSync(block *types.Block) {
	pool.isSyncing = false
	pool.ResetTo(block)
	for _, tx := range pool.deferredTxs {
		pool.Add(tx)
	}
	pool.deferredTxs = make([]*types.Transaction, 0)
	pool.knownDeferredTxs.Clear()
}

var (
	setIsFullErr     = errors.New("txs for address is full")
	sortedTxNonceErr = errors.New("nonce is not sequential")
)

type txMap struct {
	mutex  sync.RWMutex
	txs    map[common.Hash]*types.Transaction
	maxTxs int
}

func (m *txMap) Get(hash common.Hash) (*types.Transaction, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	tx, ok := m.txs[hash]
	return tx, ok
}

func (m *txMap) Add(tx *types.Transaction) error {

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := priorityTypes[tx.Type]; !ok && m.Full() {
		return setIsFullErr
	}
	m.txs[tx.Hash()] = tx
	return nil
}

func (m *txMap) Full() bool {
	return m.maxTxs > 0 && len(m.txs) >= m.maxTxs
}

func (m *txMap) Remove(hash common.Hash) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.txs, hash)
}

func (m *txMap) Empty() bool {
	return len(m.txs) == 0
}

func (m *txMap) Sorted() []*types.Transaction {
	result := make([]*types.Transaction, 0)
	m.mutex.RLock()
	for _, tx := range m.txs {
		result = append(result, tx)
	}
	m.mutex.RUnlock()
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Epoch < result[j].Epoch {
			return true
		}
		if result[i].Epoch > result[j].Epoch {
			return false
		}
		return result[i].AccountNonce < result[j].AccountNonce
	})
	return result
}

func (m *txMap) List() []*types.Transaction {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	result := make([]*types.Transaction, 0, len(m.txs))

	for _, tx := range m.txs {
		result = append(result, tx)
	}
	return result
}

func newTxMap(maxTxs int) *txMap {
	return &txMap{
		txs:    make(map[common.Hash]*types.Transaction),
		maxTxs: maxTxs,
	}
}

type sortedTxs struct {
	txs    []*types.Transaction
	maxTxs int
}

func newSortedTxs(maxTxs int) *sortedTxs {
	return &sortedTxs{
		txs:    make([]*types.Transaction, 0),
		maxTxs: maxTxs,
	}
}

func (s *sortedTxs) Add(tx *types.Transaction) error {

	if _, ok := priorityTypes[tx.Type]; !ok && s.Full() {
		return setIsFullErr
	}
	if len(s.txs) > 0 && (tx.AccountNonce != s.txs[len(s.txs)-1].AccountNonce+1 || tx.Epoch != s.txs[len(s.txs)-1].Epoch) {
		return sortedTxNonceErr
	}
	s.txs = append(s.txs, tx)
	return nil
}

func (s *sortedTxs) Full() bool {
	return s.maxTxs > 0 && len(s.txs) >= s.maxTxs
}

func (s *sortedTxs) Remove(transaction *types.Transaction) {
	i := sort.Search(len(s.txs), func(i int) bool {
		return s.txs[i].AccountNonce >= transaction.AccountNonce
	})

	if i < len(s.txs) && s.txs[i].Hash() == transaction.Hash() {
		copy(s.txs[i:], s.txs[i+1:])
		s.txs[len(s.txs)-1] = nil
		s.txs = s.txs[:len(s.txs)-1]
	}
}

func (s *sortedTxs) Empty() bool {
	return len(s.txs) == 0
}
