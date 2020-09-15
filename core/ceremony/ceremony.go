package ceremony

import (
	"bytes"
	"context"
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/flip"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/sha3"
	"github.com/idena-network/idena-go/crypto/vrf"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/protocol"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/stats/collector"
	statsTypes "github.com/idena-network/idena-go/stats/types"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	dbm "github.com/tendermint/tm-db"
	"math/rand"
	"sync"
	"time"
)

const (
	LotterySeedLag                      = 100
	MaxFlipKeysPackageBroadcastDelaySec = 120
	MaxShortAnswersBroadcastDelaySec    = 60
)

type ValidationCeremony struct {
	bus                      eventbus.Bus
	db                       dbm.DB
	appState                 *appstate.AppState
	flipper                  *flip.Flipper
	secStore                 *secstore.SecStore
	log                      log.Logger
	flips                    [][]byte
	flipsPerAuthor           map[int][][]byte
	shortFlipsPerCandidate   [][]int
	longFlipsPerCandidate    [][]int
	shortFlipsToSolve        [][]byte
	longFlipsToSolve         [][]byte
	publicKeySent            bool
	privateKeysSent          bool
	shortAnswersSent         bool
	evidenceSent             bool
	shortSessionStarted      bool
	candidates               []*candidate
	nonCandidates            []common.Address
	mutex                    sync.Mutex
	epochDb                  *database.EpochDb
	qualification            *qualification
	mempool                  *mempool.TxPool
	keysPool                 *mempool.KeysPool
	chain                    *blockchain.Blockchain
	syncer                   protocol.Syncer
	blockHandlers            map[state.ValidationPeriod]blockHandler
	validationStats          *statsTypes.ValidationStats
	flipWordsInfo            *flipWordsInfo
	epoch                    uint16
	config                   *config.Config
	applyEpochMutex          sync.Mutex
	flipAuthorMap            map[string]common.Address
	flipAuthorMapLock        sync.Mutex
	epochApplyingCache       map[uint64]epochApplyingCache
	validationStartCtxCancel context.CancelFunc
	validationStartMutex     sync.Mutex
	candidatesPerAuthor      map[int][]int
	authorsPerCandidate      map[int][]int
	newTxQueue               chan *types.Transaction
	lottery                  *lottery
}

type flipWordsInfo struct {
	pairs []int
	proof []byte
	rnd   uint64
	pool  *sync.Map
}

type epochApplyingCache struct {
	epochApplyingResult map[common.Address]cacheValue
	validationFailed    bool
	validationResults   *types.ValidationResults
}

type cacheValue struct {
	state                    state.IdentityState
	prevState                state.IdentityState
	shortQualifiedFlipsCount uint32
	shortFlipPoint           float32
	birthday                 uint16
	missed                   bool
}

type lottery struct {
	finished bool
	wg       sync.WaitGroup
}

type blockHandler func(block *types.Block)

func NewValidationCeremony(appState *appstate.AppState, bus eventbus.Bus, flipper *flip.Flipper, secStore *secstore.SecStore, db dbm.DB, mempool *mempool.TxPool,
	chain *blockchain.Blockchain, syncer protocol.Syncer, keysPool *mempool.KeysPool, config *config.Config) *ValidationCeremony {

	vc := &ValidationCeremony{
		flipper:            flipper,
		appState:           appState,
		bus:                bus,
		secStore:           secStore,
		log:                log.New(),
		db:                 db,
		mempool:            mempool,
		keysPool:           keysPool,
		epochApplyingCache: make(map[uint64]epochApplyingCache),
		chain:              chain,
		syncer:             syncer,
		config:             config,
		newTxQueue:         make(chan *types.Transaction, 10000),
		flipWordsInfo:      &flipWordsInfo{pool: &sync.Map{}},
		lottery:            &lottery{},
	}

	vc.blockHandlers = map[state.ValidationPeriod]blockHandler{
		state.NonePeriod:             func(block *types.Block) {},
		state.FlipLotteryPeriod:      vc.handleFlipLotteryPeriod,
		state.ShortSessionPeriod:     vc.handleShortSessionPeriod,
		state.LongSessionPeriod:      vc.handleLongSessionPeriod,
		state.AfterLongSessionPeriod: vc.handleAfterLongSessionPeriod,
	}
	return vc
}

func (vc *ValidationCeremony) Initialize(currentBlock *types.Block) {
	vc.epochDb = database.NewEpochDb(vc.db, vc.appState.State.Epoch())
	vc.epoch = vc.appState.State.Epoch()
	vc.qualification = NewQualification(vc.epochDb)

	_ = vc.bus.Subscribe(events.AddBlockEventID,
		func(e eventbus.Event) {
			newBlockEvent := e.(*events.NewBlockEvent)
			vc.addBlock(newBlockEvent.Block)
		})

	_ = vc.bus.Subscribe(events.FastSyncCompleted, func(event eventbus.Event) {
		vc.completeEpoch()
		vc.restoreState()
	})

	_ = vc.bus.Subscribe(events.DeleteFlipEventID,
		func(e eventbus.Event) {
			event := e.(*events.DeleteFlipEvent)
			vc.dropFlip(event.FlipCid)
		})

	_ = vc.bus.Subscribe(events.NewTxEventID, func(e eventbus.Event) {
		newTxEvent := e.(*events.NewTxEvent)
		if !newTxEvent.Deferred {
			vc.addNewTx(newTxEvent.Tx)
		}
	})

	go vc.newTxLoop()
	vc.restoreState()
	vc.addBlock(currentBlock)
}

func (vc *ValidationCeremony) addBlock(block *types.Block) {
	vc.handleBlock(block)
	vc.qualification.persist()

	// completeEpoch if finished
	if block.Header.Flags().HasFlag(types.ValidationFinished) {
		vc.completeEpoch()
		vc.startValidationShortSessionTimer()
		vc.generateFlipKeyWordPairs(vc.appState.State.FlipWordsSeed().Bytes())
	}
}

func (vc *ValidationCeremony) ShortSessionBeginTime() time.Time {
	return vc.appState.EvidenceMap.GetShortSessionBeginningTime()
}

func (vc *ValidationCeremony) isCandidate() bool {
	identity := vc.appState.State.GetIdentity(vc.secStore.GetAddress())
	return state.IsCeremonyCandidate(identity)
}

func (vc *ValidationCeremony) shouldBroadcastFlipKey(appState *appstate.AppState) bool {
	identity := appState.State.GetIdentity(vc.secStore.GetAddress())
	return len(identity.Flips) > 0
}

func (vc *ValidationCeremony) GetShortFlipsToSolve() [][]byte {
	return vc.shortFlipsToSolve
}

func (vc *ValidationCeremony) GetLongFlipsToSolve() [][]byte {
	return vc.longFlipsToSolve
}

func (vc *ValidationCeremony) SubmitShortAnswers(answers *types.Answers) (common.Hash, error) {
	vc.mutex.Lock()
	prevAnswers := vc.epochDb.ReadOwnShortAnswersBits()
	salt := getShortAnswersSalt(vc.epoch, vc.secStore)
	var hash [32]byte
	if len(prevAnswers) == 0 {
		vc.epochDb.WriteOwnShortAnswers(answers)
		hash = crypto.Hash(append(answers.Bytes(), salt[:]...))
	} else {
		vc.log.Warn("Repeated short answers submitting")
		hash = crypto.Hash(append(prevAnswers, salt[:]...))
	}
	vc.mutex.Unlock()

	h, err := vc.sendTx(types.SubmitAnswersHashTx, hash[:])
	if err != nil {
		vc.log.Error("cannot send short answers hash tx", "err", err)
	}

	return h, err
}

func (vc *ValidationCeremony) SubmitLongAnswers(answers *types.Answers) (common.Hash, error) {

	key := vc.flipper.GetFlipPublicEncryptionKey()
	salt := getShortAnswersSalt(vc.epoch, vc.secStore)

	hash, err := vc.sendTx(types.SubmitLongAnswersTx, attachments.CreateLongAnswerAttachment(answers.Bytes(), vc.flipWordsInfo.proof, salt, key))
	if err == nil {
		vc.broadcastEvidenceMap()
	} else {
		vc.log.Error("cannot send long answers tx", "err", err)
	}
	return hash, err
}

func (vc *ValidationCeremony) ShortSessionFlipsCount() uint {
	return common.ShortSessionFlipsCount()
}

func (vc *ValidationCeremony) LongSessionFlipsCount() uint {
	if len(vc.candidates) == 0 {
		return 1
	}
	count := uint(len(vc.flips) * common.LongSessionTesters / len(vc.candidates))
	if count == 0 {
		count = 1
	}
	return count
}

func (vc *ValidationCeremony) restoreState() {
	vc.generateFlipKeyWordPairs(vc.appState.State.FlipWordsSeed().Bytes())
	vc.appState.EvidenceMap.SetShortSessionTime(vc.appState.State.NextValidationTime(), vc.config.Validation.GetShortSessionDuration())
	vc.qualification.restore()
	vc.calculateCeremonyCandidates()
	vc.calculatePrivateFlipKeysIndexes()
	vc.startValidationShortSessionTimer()
	vc.lottery.finished = true
}

func (vc *ValidationCeremony) startValidationShortSessionTimer() {
	if vc.validationStartCtxCancel != nil {
		return
	}
	t := time.Now().UTC()
	validationTime := vc.appState.State.NextValidationTime()
	if t.Before(validationTime) {
		ctx, cancel := context.WithCancel(context.Background())
		vc.validationStartCtxCancel = cancel
		go func() {
			ticker := time.NewTicker(time.Second * 1)
			defer ticker.Stop()
			vc.log.Info("Short session timer has been created", "time", validationTime)
			for {
				select {
				case <-ticker.C:
					if time.Now().UTC().After(validationTime) {
						if appState, err := vc.appState.Readonly(vc.chain.Head.Height()); err == nil {
							vc.startShortSession(appState)
							vc.log.Info("Timer triggered")
						} else {
							vc.log.Error("Can not start short session with timer", "err", err)
						}
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}
}

func (vc *ValidationCeremony) completeEpoch() {
	if vc.epoch != vc.appState.State.Epoch() {
		edb := vc.epochDb
		go func() {
			vc.dropFlips(edb)
			edb.Clear()
		}()
	}
	vc.epochDb = database.NewEpochDb(vc.db, vc.appState.State.Epoch())
	vc.epoch = vc.appState.State.Epoch()

	vc.qualification = NewQualification(vc.epochDb)
	vc.flipper.Clear()
	vc.keysPool.Clear()
	vc.appState.EvidenceMap.Clear()
	vc.appState.EvidenceMap.SetShortSessionTime(vc.appState.State.NextValidationTime(), vc.config.Validation.GetShortSessionDuration())
	if vc.validationStartCtxCancel != nil {
		vc.validationStartCtxCancel()
	}
	vc.candidates = nil
	vc.flips = nil
	vc.flipsPerAuthor = nil
	vc.shortFlipsPerCandidate = nil
	vc.longFlipsPerCandidate = nil
	vc.shortFlipsToSolve = nil
	vc.longFlipsToSolve = nil
	vc.publicKeySent = false
	vc.privateKeysSent = false
	vc.shortAnswersSent = false
	vc.evidenceSent = false
	vc.shortSessionStarted = false
	vc.validationStats = nil
	vc.flipAuthorMap = nil
	vc.validationStartCtxCancel = nil
	vc.epochApplyingCache = make(map[uint64]epochApplyingCache)
	vc.candidatesPerAuthor = nil
	vc.authorsPerCandidate = nil
	vc.flipWordsInfo = &flipWordsInfo{pool: &sync.Map{}}
	vc.lottery = &lottery{}
}

func (vc *ValidationCeremony) handleBlock(block *types.Block) {
	vc.blockHandlers[vc.appState.State.ValidationPeriod()](block)
}

func (vc *ValidationCeremony) handleFlipLotteryPeriod(block *types.Block) {
	if block.Header.Flags().HasFlag(types.FlipLotteryStarted) {
		vc.logInfoWithInteraction("Flip lottery started")

		seedHeight := uint64(0)
		if block.Height() > LotterySeedLag {
			seedHeight = block.Height() - LotterySeedLag
		}

		seedHeight = math.Max(vc.chain.Genesis().Height()+1, seedHeight)
		seedBlock := vc.chain.GetBlockHeaderByHeight(seedHeight)

		vc.epochDb.WriteLotterySeed(seedBlock.Seed().Bytes())
		vc.lottery.wg.Add(1)
		go vc.asyncFlipLotteryCalculations()
	}

	if vc.lottery.finished {
		vc.tryToBroadcastFlipKeysPackage()
	}
}

func (vc *ValidationCeremony) tryToBroadcastFlipKeysPackage() {
	// attempt to broadcast own flip key package since MaxFlipKeysPackageBroadcastDelaySec seconds after flip lottery has started
	shift := vc.config.Validation.GetFlipLotteryDuration() - MaxFlipKeysPackageBroadcastDelaySec*time.Second
	if shift < 0 || vc.appState.State.NextValidationTime().Sub(time.Now().UTC()) < shift {
		vc.broadcastPrivateFlipKeysPackage(vc.appState)
	}
}

func (vc *ValidationCeremony) asyncFlipLotteryCalculations() {
	vc.logInfoWithInteraction("Flip lottery calculations started")
	vc.calculateCeremonyCandidates()
	vc.calculatePrivateFlipKeysIndexes()
	vc.logInfoWithInteraction("Flip lottery calculations finished")
	vc.lottery.finished = true
	vc.lottery.wg.Done()

	go vc.delayedFlipPackageBroadcast()
	vc.tryToBroadcastFlipKeysPackage()
}

func (vc *ValidationCeremony) handleShortSessionPeriod(block *types.Block) {
	vc.lottery.wg.Wait()

	if block.Header.Flags().HasFlag(types.ShortSessionStarted) {
		vc.startShortSession(vc.appState)
	}
	vc.broadcastPrivateFlipKeysPackage(vc.appState)
	vc.broadcastPublicFipKey(vc.appState)
	vc.processCeremonyTxs(block)
}

func (vc *ValidationCeremony) startShortSession(appState *appstate.AppState) {
	vc.validationStartMutex.Lock()
	defer vc.validationStartMutex.Unlock()

	if vc.shortSessionStarted {
		return
	}
	if vc.appState.State.ValidationPeriod() < state.FlipLotteryPeriod {
		return
	}

	vc.logInfoWithInteraction("Short session started", "at", vc.appState.State.NextValidationTime().String())
	vc.broadcastPublicFipKey(appState)
	vc.shortSessionStarted = true
}

func (vc *ValidationCeremony) handleLongSessionPeriod(block *types.Block) {
	if block.Header.Flags().HasFlag(types.LongSessionStarted) {
		vc.logInfoWithInteraction("Long session started")
		go vc.delayedShortAnswersTxBroadcast()
	}

	vc.processCeremonyTxs(block)
	vc.broadcastPublicFipKey(vc.appState)

	// attempt to broadcast short answers since MaxShortAnswersBroadcastDelaySec seconds after long session has started
	shortAnswersBroadcastTime := vc.appState.State.NextValidationTime().Add(vc.config.Validation.GetShortSessionDuration()).Add(MaxShortAnswersBroadcastDelaySec * time.Second)

	if shortAnswersBroadcastTime.Before(time.Now().UTC()) {
		vc.broadcastShortAnswersTx()
	}
	vc.broadcastEvidenceMap()
}

func (vc *ValidationCeremony) handleAfterLongSessionPeriod(block *types.Block) {
	if block.Header.Flags().HasFlag(types.AfterLongSessionStarted) {
		vc.logInfoWithInteraction("After long session started")
	}
	vc.processCeremonyTxs(block)
	vc.log.Info("After long blocks without ceremonial txs", "cnt", vc.appState.State.BlocksCntWithoutCeremonialTxs())
}

func (vc *ValidationCeremony) calculateCeremonyCandidates() {
	if vc.candidates != nil {
		return
	}

	seed := vc.epochDb.ReadLotterySeed()
	if seed == nil {
		return
	}

	vc.flipAuthorMapLock.Lock()
	vc.candidates, vc.nonCandidates, vc.flips, vc.flipsPerAuthor, vc.flipAuthorMap = vc.getCandidatesAndFlips()
	vc.flipAuthorMapLock.Unlock()

	shortFlipsCount := int(vc.ShortSessionFlipsCount() + common.ShortSessionExtraFlipsCount())
	vc.authorsPerCandidate, vc.candidatesPerAuthor = GetAuthorsDistribution(vc.candidates, seed, shortFlipsCount)
	vc.shortFlipsPerCandidate, vc.longFlipsPerCandidate = GetFlipsDistribution(len(vc.candidates), vc.authorsPerCandidate, vc.flipsPerAuthor, vc.flips, seed, shortFlipsCount)

	vc.shortFlipsToSolve = getFlipsToSolve(vc.secStore.GetAddress(), vc.candidates, vc.shortFlipsPerCandidate, vc.flips)
	vc.longFlipsToSolve = getFlipsToSolve(vc.secStore.GetAddress(), vc.candidates, vc.longFlipsPerCandidate, vc.flips)

	if vc.shouldInteractWithNetwork() {
		go vc.flipper.Load(vc.shortFlipsToSolve)
		go vc.flipper.Load(vc.longFlipsToSolve)
	}

	vc.logInfoWithInteraction("Ceremony candidates", "cnt", len(vc.candidates))

	if len(vc.candidates) < 100 {
		var addrs []string
		for _, c := range vc.getCandidatesAddresses() {
			addrs = append(addrs, c.Hex())
		}
		vc.logInfoWithInteraction("Ceremony candidates", "addresses", addrs)
	}

	vc.logInfoWithInteraction("Should solve flips in short session", "cnt", len(vc.shortFlipsToSolve))
	vc.logInfoWithInteraction("Should solve flips in long session", "cnt", len(vc.longFlipsToSolve))
}

func (vc *ValidationCeremony) shouldInteractWithNetwork() bool {

	if !vc.syncer.IsSyncing() {
		return true
	}

	conf := vc.chain.Config().Validation
	ceremonyDuration := conf.GetFlipLotteryDuration() +
		conf.GetShortSessionDuration() +
		conf.GetLongSessionDuration(vc.appState.ValidatorsCache.NetworkSize()) +
		time.Minute*15 // added extra minutes to prevent time lags
	headTime := time.Unix(vc.chain.Head.Time(), 0)

	// if head's timestamp is close to now() we should interact with network
	return time.Now().UTC().Sub(headTime) < ceremonyDuration
}

func (vc *ValidationCeremony) broadcastPublicFipKey(appState *appstate.AppState) {
	if vc.publicKeySent || !vc.shouldInteractWithNetwork() || !vc.shouldBroadcastFlipKey(appState) {
		return
	}

	epoch := vc.appState.State.Epoch()
	key := vc.flipper.GetFlipPublicEncryptionKey()

	msg := types.PublicFlipKey{
		Key:   crypto.FromECDSA(key.ExportECDSA()),
		Epoch: epoch,
	}

	signedMsg, err := vc.secStore.SignFlipKey(&msg)

	if err != nil {
		vc.log.Error("cannot sign public flip key", "epoch", epoch, "err", err)
		return
	}

	vc.keysPool.AddPublicFlipKey(signedMsg, true)
	vc.publicKeySent = true
}
func (vc *ValidationCeremony) delayedFlipPackageBroadcast() {
	if vc.shouldInteractWithNetwork() {
		time.Sleep(time.Duration(rand.Intn(MaxFlipKeysPackageBroadcastDelaySec)) * time.Second)
		vc.broadcastPrivateFlipKeysPackage(vc.appState)
	}
}

func (vc *ValidationCeremony) broadcastPrivateFlipKeysPackage(appState *appstate.AppState) {
	if vc.privateKeysSent || !vc.shouldInteractWithNetwork() || !vc.shouldBroadcastFlipKey(appState) {
		return
	}

	epoch := vc.appState.State.Epoch()
	myIndex := vc.getOwnCandidateIndex()

	candidateIndexes, ok := vc.candidatesPerAuthor[myIndex]
	if !ok {
		vc.log.Error("user does not have candidates", "epoch", epoch, "addr", vc.secStore.GetAddress().String())
		return
	}
	publicFlipKey, privateFlipKey := vc.flipper.GetFlipPublicEncryptionKey(), vc.flipper.GetFlipPrivateEncryptionKey()

	var pubKeys [][]byte
	for _, item := range candidateIndexes {
		candidate := vc.candidates[item]
		pubKeys = append(pubKeys, candidate.PubKey)
	}

	msg := types.PrivateFlipKeysPackage{
		Data:  mempool.EncryptPrivateKeysPackage(publicFlipKey, privateFlipKey, pubKeys),
		Epoch: epoch,
	}

	signedMsg, err := vc.secStore.SignFlipKeysPackage(&msg)

	if err != nil {
		vc.log.Error("cannot sign private flip keys package", "epoch", epoch, "err", err)
		return
	}

	if err := vc.keysPool.AddPrivateKeysPackage(signedMsg, true); err != nil {
		vc.log.Error("failed to add key package", "epoch", epoch, "err", err)
	} else {
		vc.log.Info("private flip keys package has been broadcast")
		vc.privateKeysSent = true
	}
}

func (vc *ValidationCeremony) getCandidatesAndFlips() ([]*candidate, []common.Address, [][]byte, map[int][][]byte, map[string]common.Address) {
	nonCandidates := make([]common.Address, 0)
	m := make([]*candidate, 0)
	flips := make([][]byte, 0)
	flipsPerAuthor := make(map[int][][]byte)
	flipAuthorMap := make(map[string]common.Address)

	addFlips := func(author common.Address, identityFlips []state.IdentityFlip) {
		for _, f := range identityFlips {
			authorIndex := len(m)
			flips = append(flips, f.Cid)
			flipsPerAuthor[authorIndex] = append(flipsPerAuthor[authorIndex], f.Cid)
			flipAuthorMap[string(f.Cid)] = author
		}
	}

	vc.appState.State.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])

		var data state.Identity
		if err := data.FromBytes(value); err != nil {
			return false
		}
		if state.IsCeremonyCandidate(data) {
			addFlips(addr, data.Flips)
			m = append(m, &candidate{
				Address:    addr,
				PubKey:     data.PubKey,
				Generation: data.Generation,
				Code:       data.Code,
				IsAuthor:   len(data.Flips) > 0,
			})
		} else {
			nonCandidates = append(nonCandidates, addr)
		}

		return false
	})

	return m, nonCandidates, flips, flipsPerAuthor, flipAuthorMap
}

func (vc *ValidationCeremony) getCandidatesAddresses() []common.Address {
	var result []common.Address
	for _, p := range vc.candidates {
		result = append(result, p.Address)
	}
	return result
}

func getFlipsToSolve(self common.Address, participants []*candidate, flipsPerCandidate [][]int, flipCids [][]byte) [][]byte {
	if len(flipCids) == 0 || len(participants) == 0 {
		return nil
	}
	var result [][]byte
	for i := 0; i < len(participants); i++ {
		if participants[i].Address == self {
			myFlips := flipsPerCandidate[i]
			allFlips := flipCids

			for j := 0; j < len(myFlips); j++ {
				result = append(result, allFlips[myFlips[j]%len(allFlips)])
			}
			break
		}
	}

	return result
}

func (vc *ValidationCeremony) processCeremonyTxs(block *types.Block) {
	for _, tx := range block.Body.Transactions {
		sender, _ := types.Sender(tx)

		switch tx.Type {
		case types.SubmitAnswersHashTx:
			if !vc.epochDb.HasAnswerHash(sender) {
				vc.epochDb.WriteAnswerHash(sender, common.BytesToHash(tx.Payload), time.Now().UTC())
			}
		case types.SubmitShortAnswersTx:
			vc.qualification.addAnswers(true, sender, tx.Payload)
		case types.SubmitLongAnswersTx:
			vc.qualification.addAnswers(false, sender, tx.Payload)
		case types.EvidenceTx:
			if !vc.epochDb.HasEvidenceMap(sender) {
				vc.epochDb.WriteEvidenceMap(sender, tx.Payload)
			}
		}
	}
}

func (vc *ValidationCeremony) delayedShortAnswersTxBroadcast() {
	if vc.shouldInteractWithNetwork() {
		time.Sleep(time.Duration(rand.Intn(MaxShortAnswersBroadcastDelaySec)) * time.Second)
		vc.broadcastShortAnswersTx()
	}
}

func (vc *ValidationCeremony) broadcastShortAnswersTx() {
	if vc.shortAnswersSent || !vc.shouldInteractWithNetwork() || !vc.isCandidate() {
		return
	}
	answers := vc.epochDb.ReadOwnShortAnswersBits()
	if answers == nil {
		vc.log.Error("short session answers are missing")
		return
	}

	h, err := vrf.HashFromProof(vc.flipWordsInfo.proof)
	if err != nil {
		vc.log.Error("Cannot get hash from proof during short answers broadcasting")
		return
	}

	if _, err := vc.sendTx(types.SubmitShortAnswersTx, attachments.CreateShortAnswerAttachment(answers, getWordsRnd(h))); err == nil {
		vc.shortAnswersSent = true
	} else {
		vc.log.Error("cannot send short answers tx", "err", err)
	}
}

func (vc *ValidationCeremony) broadcastEvidenceMap() {
	if vc.evidenceSent || !vc.shouldInteractWithNetwork() || !vc.isCandidate() || !vc.appState.EvidenceMap.IsCompleted() || !vc.shortAnswersSent {
		return
	}

	if existTx := vc.epochDb.ReadOwnTx(types.SubmitLongAnswersTx); existTx == nil {
		return
	}

	shortSessionStart, shortSessionEnd := vc.appState.EvidenceMap.GetShortSessionBeginningTime(), vc.appState.EvidenceMap.GetShortSessionEndingTime()

	additional := vc.epochDb.GetConfirmedRespondents(shortSessionStart, shortSessionEnd)

	candidates := vc.getCandidatesAddresses()

	bitmap := vc.appState.EvidenceMap.CalculateBitmap(candidates, additional, vc.appState.State.GetRequiredFlips)

	if len(candidates) < 100 {
		var inMemory string
		var final string
		for i, c := range candidates {
			if vc.appState.EvidenceMap.ContainsAnswer(c) && (vc.appState.EvidenceMap.ContainsKey(c) || vc.appState.State.GetRequiredFlips(c) <= 0) {
				inMemory += "1"
			} else {
				inMemory += "0"
			}
			if bitmap.Contains(uint32(i)) {
				final += "1"
			} else {
				final += "0"
			}
		}
		vc.logInfoWithInteraction("In memory evidence map", "map", inMemory)
		vc.logInfoWithInteraction("Final evidence map", "map", final)
	}

	buf := new(bytes.Buffer)

	bitmap.WriteTo(buf)

	if _, err := vc.sendTx(types.EvidenceTx, buf.Bytes()); err == nil {
		vc.evidenceSent = true
	} else {
		vc.log.Error("cannot send evidence tx", "err", err)
	}
}

func (vc *ValidationCeremony) sendTx(txType uint16, payload []byte) (common.Hash, error) {
	vc.mutex.Lock()
	defer vc.mutex.Unlock()

	signedTx := &types.Transaction{}

	if existTx := vc.epochDb.ReadOwnTx(txType); existTx != nil {
		if err := signedTx.FromBytes(existTx); err != nil {
			return common.Hash{}, err
		}
	} else {
		addr := vc.secStore.GetAddress()
		tx := blockchain.BuildTx(vc.appState, addr, nil, txType, decimal.Zero, decimal.Zero, decimal.Zero, 0, 0, payload)
		var err error
		signedTx, err = vc.secStore.SignTx(tx)
		if err != nil {
			return common.Hash{}, err
		}
		txBytes, err := signedTx.ToBytes()
		if err != nil {
			return common.Hash{}, err
		}
		vc.epochDb.WriteOwnTx(txType, txBytes)
	}

	err := vc.mempool.Add(signedTx)

	if err != nil {
		if !vc.epochDb.HasSuccessfulOwnTx(signedTx.Hash()) {
			vc.epochDb.RemoveOwnTx(txType)
		}
	} else {
		vc.epochDb.WriteSuccessfulOwnTx(signedTx.Hash())
	}
	vc.logInfoWithInteraction("Broadcast ceremony tx", "type", txType, "hash", signedTx.Hash().Hex())

	return signedTx.Hash(), err
}

func applyOnState(appState *appstate.AppState, statsCollector collector.StatsCollector, addr common.Address, value cacheValue) (identitiesCount int) {
	collector.BeginFailedValidationBalanceUpdate(statsCollector, addr, appState)
	appState.State.SetState(addr, value.state)
	collector.CompleteBalanceUpdate(statsCollector, appState)
	if !value.missed {
		appState.State.AddNewScore(addr, common.EncodeScore(value.shortFlipPoint, value.shortQualifiedFlipsCount))
	}
	appState.State.SetBirthday(addr, value.birthday)
	if value.state == state.Verified && value.prevState == state.Newbie {
		addToBalance := math.ToInt(decimal.NewFromBigInt(appState.State.GetStakeBalance(addr), 0).Mul(decimal.NewFromFloat(common.StakeToBalanceCoef)))
		collector.BeginVerifiedStakeTransferBalanceUpdate(statsCollector, addr, appState)
		appState.State.AddBalance(addr, addToBalance)
		appState.State.SubStake(addr, addToBalance)
		collector.CompleteBalanceUpdate(statsCollector, appState)
	}

	if value.state.NewbieOrBetter() {
		identitiesCount++
	} else if value.state == state.Killed {
		// Stake of killed identity is burnt
		collector.AddKilledBurntCoins(statsCollector, addr, appState.State.GetStakeBalance(addr))
	}
	return identitiesCount
}

func (vc *ValidationCeremony) ApplyNewEpoch(height uint64, appState *appstate.AppState, statsCollector collector.StatsCollector) (identitiesCount int, authors *types.ValidationResults, failed bool) {

	vc.applyEpochMutex.Lock()
	defer vc.applyEpochMutex.Unlock()
	defer collector.SetValidation(statsCollector, vc.validationStats)

	if applyingCache, ok := vc.epochApplyingCache[height]; ok {
		if applyingCache.validationFailed {
			return vc.appState.ValidatorsCache.NetworkSize(), applyingCache.validationResults, true
		}

		if len(applyingCache.epochApplyingResult) > 0 {
			for addr, value := range applyingCache.epochApplyingResult {
				identitiesCount += applyOnState(appState, statsCollector, addr, value)
			}
			return identitiesCount, applyingCache.validationResults, false
		}
	}

	vc.validationStats = statsTypes.NewValidationStats()
	stats := vc.validationStats
	stats.FlipCids = vc.flips
	approvedCandidates := vc.appState.EvidenceMap.CalculateApprovedCandidates(vc.getCandidatesAddresses(), vc.epochDb.ReadEvidenceMaps())
	approvedCandidatesSet := mapset.NewSet()
	for _, item := range approvedCandidates {
		approvedCandidatesSet.Add(item)
	}

	totalFlipsCount := len(vc.flips)

	flipQualification, reportersToReward := vc.qualification.qualifyFlips(uint(totalFlipsCount), vc.candidates, vc.longFlipsPerCandidate)

	flipQualificationMap := make(map[int]FlipQualification)
	for i, item := range flipQualification {
		flipQualificationMap[i] = item
		stats.FlipsPerIdx[i] = &statsTypes.FlipStats{
			Status: byte(item.status),
			Answer: item.answer,
			Grade:  item.grade,
		}
	}
	var flipsByAuthor map[common.Address][]int
	validationResults := new(types.ValidationResults)
	validationResults.GoodInviters = make(map[common.Address]*types.InviterValidationResult)
	validationResults.BadAuthors,
		validationResults.GoodAuthors,
		validationResults.AuthorResults,
		flipsByAuthor,
		reportersToReward = vc.analyzeAuthors(flipQualification, reportersToReward)

	vc.logInfoWithInteraction("Approved candidates", "cnt", len(approvedCandidates))

	notApprovedFlips := vc.getNotApprovedFlips(approvedCandidatesSet)

	god := appState.State.GodAddress()

	intermediateIdentitiesCount := 0
	epochApplyingValues := make(map[common.Address]cacheValue)

	for idx, candidate := range vc.candidates {
		addr := candidate.Address
		var totalFlips uint32
		var shortScore, longScore, totalScore float32
		shortFlipsToSolve := vc.shortFlipsPerCandidate[idx]
		shortFlipPoint, shortQualifiedFlipsCount, shortFlipAnswers, noQualShort, noAnswersShort := vc.qualification.qualifyCandidate(addr, flipQualificationMap, shortFlipsToSolve, true, notApprovedFlips)
		addFlipAnswersToStats(shortFlipAnswers, true, stats)

		longFlipsToSolve := vc.longFlipsPerCandidate[idx]
		longFlipPoint, longQualifiedFlipsCount, longFlipAnswers, noQualLong, noAnswersLong := vc.qualification.qualifyCandidate(addr, flipQualificationMap, longFlipsToSolve, false, notApprovedFlips)
		addFlipAnswersToStats(longFlipAnswers, false, stats)

		totalFlipPoints := appState.State.GetShortFlipPoints(addr)
		totalQualifiedFlipsCount := appState.State.GetQualifiedFlipsCount(addr)
		approved := approvedCandidatesSet.Contains(addr)
		missed := !approved || noAnswersShort || noAnswersLong

		if shortQualifiedFlipsCount > 0 {
			shortScore = shortFlipPoint / float32(shortQualifiedFlipsCount)
		}
		if !approved {
			shortQualifiedFlipsCount, shortFlipPoint, shortScore = 0, 0, 0
		}
		if longQualifiedFlipsCount > 0 {
			longScore = longFlipPoint / float32(longQualifiedFlipsCount)
		}

		totalScore, totalFlips = calculateNewTotalScore(appState.State.GetScores(addr), shortFlipPoint, shortQualifiedFlipsCount, totalFlipPoints, totalQualifiedFlipsCount)

		identity := appState.State.GetIdentity(addr)
		newIdentityState := determineNewIdentityState(identity, shortScore, longScore, totalScore,
			totalFlips, missed, noQualShort, noQualLong)
		identityBirthday := determineIdentityBirthday(vc.epoch, identity, newIdentityState)

		incSuccessfulInvites(validationResults, god, identity, identityBirthday, newIdentityState, vc.epoch)
		setValidationResultToGoodAuthor(addr, newIdentityState, missed, validationResults)
		setValidationResultToGoodInviter(validationResults, addr, newIdentityState, identity.Invites)
		reportersToReward.setValidationResult(addr, newIdentityState, missed, flipsByAuthor)

		value := cacheValue{
			state:                    newIdentityState,
			prevState:                identity.State,
			shortQualifiedFlipsCount: shortQualifiedFlipsCount,
			shortFlipPoint:           shortFlipPoint,
			birthday:                 identityBirthday,
			missed:                   missed,
		}

		epochApplyingValues[addr] = value

		stats.IdentitiesPerAddr[addr] = &statsTypes.IdentityStats{
			ShortPoint:        shortFlipPoint,
			ShortFlips:        shortQualifiedFlipsCount,
			LongPoint:         longFlipPoint,
			LongFlips:         longQualifiedFlipsCount,
			Approved:          approved,
			Missed:            missed,
			ShortFlipsToSolve: shortFlipsToSolve,
			LongFlipsToSolve:  longFlipsToSolve,
		}

		if value.state.NewbieOrBetter() {
			intermediateIdentitiesCount++
		}
	}
	validationResults.ReportersToRewardByFlip = reportersToReward.getReportersByFlipMap()

	if intermediateIdentitiesCount == 0 {
		vc.log.Warn("validation failed, nobody is validated, identities remains the same")
		stats.Failed = true
		vc.epochApplyingCache[height] = epochApplyingCache{
			epochApplyingResult: epochApplyingValues,
			validationResults:   validationResults,
			validationFailed:    true,
		}
		return vc.appState.ValidatorsCache.NetworkSize(), validationResults, true
	}

	for addr, value := range epochApplyingValues {
		identitiesCount += applyOnState(appState, statsCollector, addr, value)
	}

	for _, addr := range vc.nonCandidates {
		identity := appState.State.GetIdentity(addr)
		newIdentityState := determineNewIdentityState(identity, 0, 0, 0, 0, true, false, false)
		identityBirthday := determineIdentityBirthday(vc.epoch, identity, newIdentityState)

		if identity.State == state.Invite && identity.Inviter != nil && identity.Inviter.Address != god {
			if goodInviter, ok := validationResults.GoodInviters[identity.Inviter.Address]; ok {
				goodInviter.SavedInvites += 1
			}
		}

		value := cacheValue{
			state:                    newIdentityState,
			shortQualifiedFlipsCount: 0,
			shortFlipPoint:           0,
			birthday:                 identityBirthday,
		}
		epochApplyingValues[addr] = value
		identitiesCount += applyOnState(appState, statsCollector, addr, value)
	}

	vc.epochApplyingCache[height] = epochApplyingCache{
		epochApplyingResult: epochApplyingValues,
		validationResults:   validationResults,
		validationFailed:    false,
	}

	return identitiesCount, validationResults, false
}

func calculateNewTotalScore(scores []byte, shortPoints float32, shortFlipsCount uint32, totalShortPoints float32, totalShortFlipsCount uint32) (totalScore float32, totalFlips uint32) {
	newScores := make([]byte, len(scores))
	copy(newScores, scores)
	newScores = append(newScores, common.EncodeScore(shortPoints, shortFlipsCount))

	if len(newScores) > common.LastScoresCount {
		newScores = newScores[len(newScores)-common.LastScoresCount:]
	}

	resPoints, resFlips := common.CalculateIdentityScores(newScores, totalShortPoints, totalShortFlipsCount)

	return resPoints / float32(resFlips), resFlips
}

func setValidationResultToGoodAuthor(address common.Address, newState state.IdentityState, missed bool, validationResults *types.ValidationResults) {
	goodAuthors := validationResults.GoodAuthors
	if vr, ok := goodAuthors[address]; ok {
		vr.Missed = missed
		vr.NewIdentityState = uint8(newState)
	}
}

func setValidationResultToGoodInviter(validationResults *types.ValidationResults, address common.Address, newState state.IdentityState, invites uint8) {
	goodInviter, ok := getOrPutGoodInviter(validationResults, address)
	if !ok {
		return
	}
	goodInviter.PayInvitationReward = newState.NewbieOrBetter()
	goodInviter.SavedInvites = invites
	goodInviter.NewIdentityState = uint8(newState)
}

func incSuccessfulInvites(validationResults *types.ValidationResults, god common.Address, invitee state.Identity,
	birthday uint16, newState state.IdentityState, currentEpoch uint16) {
	if invitee.Inviter == nil || newState != state.Newbie && newState != state.Verified {
		return
	}
	newAge := currentEpoch - birthday + 1
	if newAge > 3 {
		return
	}
	goodInviter, ok := getOrPutGoodInviter(validationResults, invitee.Inviter.Address)
	if !ok {
		return
	}
	goodInviter.SuccessfulInvites = append(goodInviter.SuccessfulInvites, &types.SuccessfulInvite{
		Age:    newAge,
		TxHash: invitee.Inviter.TxHash,
	})
	if invitee.Inviter.Address == god {
		goodInviter.PayInvitationReward = true
	}
}

func getOrPutGoodInviter(validationResults *types.ValidationResults, address common.Address) (*types.InviterValidationResult, bool) {
	if _, isBadAuthor := validationResults.BadAuthors[address]; isBadAuthor {
		return nil, false
	}
	inviter, ok := validationResults.GoodInviters[address]
	if !ok {
		inviter = &types.InviterValidationResult{}
		validationResults.GoodInviters[address] = inviter
	}
	return inviter, true
}

func (vc *ValidationCeremony) analyzeAuthors(qualifications []FlipQualification, reportersToReward *reportersToReward) (badAuthors map[common.Address]types.BadAuthorReason, goodAuthors map[common.Address]*types.ValidationResult, authorResults map[common.Address]*types.AuthorResults, madeFlips map[common.Address][]int, filteredReportersToReward *reportersToReward) {

	badAuthors = make(map[common.Address]types.BadAuthorReason)
	goodAuthors = make(map[common.Address]*types.ValidationResult)
	authorResults = make(map[common.Address]*types.AuthorResults)
	madeFlips = make(map[common.Address][]int)

	badAuthorsWithoutReport := make(map[common.Address]struct{})
	nonQualifiedFlips := make(map[common.Address]int)

	for flipIdx, item := range qualifications {
		cid := vc.flips[flipIdx]
		author := vc.flipAuthorMap[string(cid)]
		madeFlips[author] = append(madeFlips[author], flipIdx)
		if _, ok := authorResults[author]; !ok {
			authorResults[author] = new(types.AuthorResults)
		}
		if item.grade == types.GradeReported || item.status == QualifiedByNone {
			if item.status == QualifiedByNone {
				badAuthorsWithoutReport[author] = struct{}{}
			}
			if item.grade == types.GradeReported {
				badAuthors[author] = types.WrongWordsBadAuthor
				if item.status != Qualified && item.status != WeaklyQualified {
					reportersToReward.deleteFlip(flipIdx)
				}
			} else if _, ok := badAuthors[author]; !ok {
				badAuthors[author] = types.QualifiedByNoneBadAuthor
			}
			authorResults[author].HasOneReportedFlip = true
		}
		if item.status == NotQualified {
			nonQualifiedFlips[author] += 1
			authorResults[author].HasOneNotQualifiedFlip = true
		}

		if item.status == Qualified || item.status == WeaklyQualified {
			vr, ok := goodAuthors[author]
			if !ok {
				vr = new(types.ValidationResult)
				goodAuthors[author] = vr
			}
			vr.FlipsToReward = append(vr.FlipsToReward, &types.FlipToReward{
				Cid:   cid,
				Grade: item.grade,
			})
		}
	}

	for author, nonQual := range nonQualifiedFlips {
		if len(madeFlips[author]) == nonQual {
			if _, ok := badAuthors[author]; !ok {
				badAuthors[author] = types.NoQualifiedFlipsBadAuthor
			}
			authorResults[author].AllFlipsNotQualified = true
			badAuthorsWithoutReport[author] = struct{}{}
		}
	}
	for author := range badAuthors {
		delete(goodAuthors, author)
		reportersToReward.deleteReporter(author)
		if _, ok := badAuthorsWithoutReport[author]; ok {
			for _, flipIdx := range madeFlips[author] {
				reportersToReward.deleteFlip(flipIdx)
			}
		}
	}
	return badAuthors, goodAuthors, authorResults, madeFlips, reportersToReward
}

func addFlipAnswersToStats(answers map[int]statsTypes.FlipAnswerStats, isShort bool, stats *statsTypes.ValidationStats) {
	for flipIdx, answer := range answers {
		flipStats, _ := stats.FlipsPerIdx[flipIdx]
		if isShort {
			flipStats.ShortAnswers = append(flipStats.ShortAnswers, answer)
		} else {
			flipStats.LongAnswers = append(flipStats.LongAnswers, answer)
		}
	}
}

func (vc *ValidationCeremony) getNotApprovedFlips(approvedCandidates mapset.Set) mapset.Set {
	result := mapset.NewSet()
	for i, c := range vc.candidates {
		addr := c.Address
		if !approvedCandidates.Contains(addr) && vc.appState.State.GetRequiredFlips(addr) > 0 {
			for _, f := range vc.flipsPerAuthor[i] {
				flipIdx := flipPos(vc.flips, f)
				result.Add(flipIdx)
			}
		}
	}
	return result
}

func flipPos(flips [][]byte, flip []byte) int {
	for i, curFlip := range flips {
		if bytes.Compare(curFlip, flip) == 0 {
			return i
		}
	}
	return -1
}

func (vc *ValidationCeremony) dropFlips(db *database.EpochDb) {
	db.IterateOverFlipCids(func(cid []byte) {
		vc.flipper.UnpinFlip(cid)
	})
}

func (vc *ValidationCeremony) dropFlip(cid []byte) {
	vc.epochDb.DeleteFlipCid(cid)
	vc.flipper.UnpinFlip(cid)
}

func (vc *ValidationCeremony) logInfoWithInteraction(msg string, ctx ...interface{}) {
	if vc.shouldInteractWithNetwork() {
		log.Info(msg, ctx...)
	}
}

func determineIdentityBirthday(currentEpoch uint16, identity state.Identity, newState state.IdentityState) uint16 {
	switch identity.State {
	case state.Candidate:
		if newState == state.Newbie {
			return currentEpoch
		}
		return 0
	case state.Newbie,
		state.Verified,
		state.Human,
		state.Suspended,
		state.Zombie:
		return identity.Birthday
	}
	return 0
}

func determineNewIdentityState(identity state.Identity, shortScore, longScore, totalScore float32, totalQualifiedFlips uint32, missed, noQualShort, nonQualLong bool) state.IdentityState {

	if !identity.HasDoneAllRequiredFlips() {
		switch identity.State {
		case state.Verified, state.Human:
			return state.Suspended
		default:
			return state.Killed
		}
	}

	prevState := identity.State

	switch prevState {
	case state.Undefined:
		return state.Undefined
	case state.Invite:
		return state.Killed
	case state.Candidate:
		if missed {
			return state.Killed
		}
		if noQualShort || nonQualLong && shortScore >= common.MinShortScore {
			return state.Candidate
		}
		if shortScore < common.MinShortScore || longScore < common.MinLongScore {
			return state.Killed
		}
		return state.Newbie
	case state.Newbie:
		if missed {
			return state.Killed
		}
		if noQualShort ||
			nonQualLong && totalQualifiedFlips >= common.MinFlipsForVerified && totalScore >= common.MinTotalScore && shortScore >= common.MinShortScore ||
			nonQualLong && totalQualifiedFlips < common.MinFlipsForVerified && shortScore >= common.MinShortScore {
			return state.Newbie
		}
		if totalQualifiedFlips >= common.MinFlipsForVerified && totalScore >= common.MinTotalScore && shortScore >= common.MinShortScore && longScore >= common.MinLongScore {
			return state.Verified
		}
		if totalQualifiedFlips < common.MinFlipsForVerified && shortScore >= common.MinShortScore && longScore >= common.MinLongScore {
			return state.Newbie
		}
		return state.Killed
	case state.Verified:
		if missed {
			return state.Suspended
		}
		if noQualShort || nonQualLong && totalScore >= common.MinTotalScore && shortScore >= common.MinShortScore {
			return state.Verified
		}
		if totalQualifiedFlips >= common.MinFlipsForHuman && totalScore >= common.MinHumanTotalScore && shortScore >= common.MinShortScore && longScore >= common.MinLongScore {
			return state.Human
		}
		if totalQualifiedFlips >= common.MinFlipsForVerified && totalScore >= common.MinTotalScore && shortScore >= common.MinShortScore && longScore >= common.MinLongScore {
			return state.Verified
		}
		return state.Killed
	case state.Suspended:
		if missed {
			return state.Zombie
		}
		if noQualShort || nonQualLong && totalScore >= common.MinTotalScore && shortScore >= common.MinShortScore {
			return state.Suspended
		}
		if totalQualifiedFlips >= common.MinFlipsForHuman && totalScore >= common.MinHumanTotalScore && shortScore >= common.MinShortScore && longScore >= common.MinLongScore {
			return state.Human
		}
		if totalScore >= common.MinTotalScore && shortScore >= common.MinShortScore && longScore >= common.MinLongScore {
			return state.Verified
		}
		return state.Killed
	case state.Zombie:
		if missed {
			return state.Killed
		}
		if noQualShort || nonQualLong && totalScore >= common.MinTotalScore && shortScore >= common.MinShortScore {
			return state.Zombie
		}
		if totalQualifiedFlips >= common.MinFlipsForHuman && totalScore >= common.MinHumanTotalScore && shortScore >= common.MinShortScore && longScore >= common.MinLongScore {
			return state.Human
		}
		if totalScore >= common.MinTotalScore && shortScore >= common.MinShortScore {
			return state.Verified
		}
		return state.Killed
	case state.Human:
		if missed {
			return state.Suspended
		}
		if noQualShort || nonQualLong && totalScore >= common.MinHumanTotalScore && shortScore >= common.MinShortScore {
			return state.Human
		}
		if nonQualLong {
			return state.Suspended
		}
		if totalScore >= common.MinHumanTotalScore && shortScore >= common.MinShortScore && longScore >= common.MinLongScore {
			return state.Human
		}
		if totalScore >= common.MinTotalScore && shortScore >= common.MinShortScore && longScore >= common.MinLongScore {
			return state.Verified
		}
		if totalScore >= common.MinTotalScore && longScore >= common.MinLongScore {
			return state.Suspended
		}
		return state.Killed
	case state.Killed:
		return state.Killed
	}
	return state.Undefined
}

func (vc *ValidationCeremony) FlipKeyWordPairs() []int {
	return vc.flipWordsInfo.pairs
}

func (vc *ValidationCeremony) generateFlipKeyWordPairs(seed []byte) {
	identity := vc.appState.State.GetIdentity(vc.secStore.GetAddress())
	vc.flipWordsInfo.pairs, vc.flipWordsInfo.proof = vc.GeneratePairs(seed, common.WordDictionarySize,
		identity.GetTotalWordPairsCount())
}

func (vc *ValidationCeremony) GetFlipWords(cid []byte) (word1, word2 int, err error) {
	vc.flipAuthorMapLock.Lock()
	author, ok := vc.flipAuthorMap[string(cid)]
	vc.flipAuthorMapLock.Unlock()

	if !ok {
		return 0, 0, errors.New("flip author not found")
	}

	identity := vc.appState.State.GetIdentity(author)
	pairId := 0
	for _, item := range identity.Flips {
		if bytes.Compare(cid, item.Cid) == 0 {
			pairId = int(item.Pair)
			break
		}
	}
	rnd := vc.qualification.GetWordsRnd(author)

	if rnd == 0 {
		if value, ok := vc.flipWordsInfo.pool.Load(author); !ok {
			return 0, 0, errors.New("words not ready")
		} else {
			rnd = value.(uint64)
		}
	}

	return GetWords(rnd, common.WordDictionarySize, identity.GetTotalWordPairsCount(), pairId)
}

func (vc *ValidationCeremony) getOwnCandidateIndex() int {
	myAddress := vc.secStore.GetAddress()
	for idx, candidate := range vc.candidates {
		if candidate.Address == myAddress {
			return idx
		}
	}
	return -1
}

func getShortAnswersSalt(epoch uint16, secStore *secstore.SecStore) []byte {
	seed := []byte(fmt.Sprintf("short-answers-salt-%v", epoch))
	hash := common.Hash(crypto.Hash(seed))
	sig := secStore.Sign(hash.Bytes())
	sha := sha3.Sum256(sig)
	return sha[:]
}

func (vc *ValidationCeremony) ShortSessionStarted() bool {
	return vc.shortSessionStarted
}

func (vc *ValidationCeremony) calculatePrivateFlipKeysIndexes() {
	if vc.candidates == nil || !vc.isCandidate() {
		return
	}
	myIndex := vc.getOwnCandidateIndex()
	authors := vc.authorsPerCandidate[myIndex]
	m := make(map[common.Address]int)
	for _, item := range authors {
		authorCandidates := vc.candidatesPerAuthor[item]
		for idx, c := range authorCandidates {
			if c == myIndex {
				m[vc.candidates[item].Address] = idx
			}
		}
	}
	vc.keysPool.InitializePrivateKeyIndexes(m)
}

func (vc *ValidationCeremony) addNewTx(tx *types.Transaction) {
	select {
	case vc.newTxQueue <- tx:
	default:
	}
}

func (vc *ValidationCeremony) newTxLoop() {
	for {
		tx := <-vc.newTxQueue
		if tx.Type == types.SubmitShortAnswersTx {
			sender, _ := types.Sender(tx)
			attachment := attachments.ParseShortAnswerAttachment(tx)
			if attachment != nil {
				vc.flipWordsInfo.pool.Store(sender, attachment.Rnd)
			}
		}
	}
}
