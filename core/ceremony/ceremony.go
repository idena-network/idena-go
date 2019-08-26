package ceremony

import (
	"bytes"
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/flip"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/protocol"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/secstore"
	"github.com/ipfs/go-cid"
	"github.com/shopspring/decimal"
	dbm "github.com/tendermint/tm-cmn/db"
	"sync"
	"time"
)

const (
	LotterySeedLag = 100
)

const (
	MinTotalScore = 0.75
	MinShortScore = 0.5
	MinLongScore  = 0.75

	WordDictionarySize = 3300
	WordPairsPerFlip   = 3
)

type ValidationCeremony struct {
	bus                    eventbus.Bus
	db                     dbm.DB
	appState               *appstate.AppState
	flipper                *flip.Flipper
	secStore               *secstore.SecStore
	log                    log.Logger
	flips                  [][]byte
	flipsPerAuthor         map[int][][]byte
	shortFlipsPerCandidate [][]int
	longFlipsPerCandidate  [][]int
	shortFlipsToSolve      [][]byte
	longFlipsToSolve       [][]byte
	keySent                bool
	shortAnswersSent       bool
	evidenceSent           bool
	candidates             []*candidate
	nonCandidates          []common.Address
	mutex                  sync.Mutex
	epochDb                *database.EpochDb
	qualification          *qualification
	mempool                *mempool.TxPool
	keysPool               *mempool.KeysPool
	chain                  *blockchain.Blockchain
	syncer                 protocol.Syncer
	blockHandlers          map[state.ValidationPeriod]blockHandler
	epochApplyingResult    map[common.Address]cacheValue
	validationStats        *Stats
	flipKeyWordPairs       []int
	epoch                  uint16
	config                 *config.Config
	applyEpochMutex        sync.Mutex
}

type cacheValue struct {
	state                    state.IdentityState
	shortQualifiedFlipsCount uint32
	shortFlipPoint           float32
	birthday                 uint16
}

type blockHandler func(block *types.Block)

func NewValidationCeremony(appState *appstate.AppState, bus eventbus.Bus, flipper *flip.Flipper, secStore *secstore.SecStore, db dbm.DB, mempool *mempool.TxPool,
	chain *blockchain.Blockchain, syncer protocol.Syncer, keysPool *mempool.KeysPool, config *config.Config) *ValidationCeremony {

	vc := &ValidationCeremony{
		flipper:             flipper,
		appState:            appState,
		bus:                 bus,
		secStore:            secStore,
		log:                 log.New(),
		db:                  db,
		mempool:             mempool,
		keysPool:            keysPool,
		epochApplyingResult: make(map[common.Address]cacheValue),
		chain:               chain,
		syncer:              syncer,
		config:              config,
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

	vc.restoreState()
	vc.addBlock(currentBlock)
}

func (vc *ValidationCeremony) addBlock(block *types.Block) {
	vc.handleBlock(block)
	vc.qualification.persistAnswers()

	// completeEpoch if finished
	if block.Header.Flags().HasFlag(types.ValidationFinished) {
		vc.completeEpoch()
		vc.generateFlipKeyWordPairs(vc.appState.State.FlipWordsSeed().Bytes())
	}
}

func (vc *ValidationCeremony) ShortSessionBeginTime() *time.Time {
	return vc.appState.EvidenceMap.GetShortSessionBeginningTime()
}

func (vc *ValidationCeremony) IsCandidate() bool {
	identity := vc.appState.State.GetIdentity(vc.secStore.GetAddress())
	return state.IsCeremonyCandidate(identity)
}

func (vc *ValidationCeremony) shouldBroadcastFlipKey() bool {
	identity := vc.appState.State.GetIdentity(vc.secStore.GetAddress())
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
	var hash [32]byte
	if len(prevAnswers) == 0 {
		vc.epochDb.WriteOwnShortAnswers(answers)
		hash = rlp.Hash(answers.Bytes())
	} else {
		vc.log.Warn("Repeated short answers submitting")
		hash = rlp.Hash(prevAnswers)
	}
	vc.mutex.Unlock()
	return vc.sendTx(types.SubmitAnswersHashTx, hash[:])
}

func (vc *ValidationCeremony) SubmitLongAnswers(answers *types.Answers) (common.Hash, error) {
	return vc.sendTx(types.SubmitLongAnswersTx, answers.Bytes())
}

func (vc *ValidationCeremony) ShortSessionFlipsCount() uint {
	return common.ShortSessionFlipsCount()
}

func (vc *ValidationCeremony) LongSessionFlipsCount() uint {
	networkSize := vc.appState.ValidatorsCache.NetworkSize()
	return common.LongSessionFlipsCount(networkSize)
}

func (vc *ValidationCeremony) GetValidationStats() *Stats {
	return vc.validationStats
}

func (vc *ValidationCeremony) restoreState() {
	timestamp := vc.epochDb.ReadShortSessionTime()
	if timestamp != nil {
		vc.appState.EvidenceMap.SetShortSessionTime(timestamp, vc.config.Validation.GetShortSessionDuration())
	}
	vc.qualification.restoreAnswers()
	vc.calculateCeremonyCandidates()
	vc.restoreFlipKeyWordPairs()
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
	vc.flipper.Reset()
	vc.appState.EvidenceMap.Clear()

	vc.candidates = nil
	vc.flips = nil
	vc.flipsPerAuthor = nil
	vc.shortFlipsPerCandidate = nil
	vc.longFlipsPerCandidate = nil
	vc.shortFlipsToSolve = nil
	vc.longFlipsToSolve = nil
	vc.keySent = false
	vc.shortAnswersSent = false
	vc.evidenceSent = false
	vc.epochApplyingResult = make(map[common.Address]cacheValue)
	vc.validationStats = nil
	vc.flipKeyWordPairs = nil
}
func (vc *ValidationCeremony) handleBlock(block *types.Block) {
	vc.blockHandlers[vc.appState.State.ValidationPeriod()](block)
}

func (vc *ValidationCeremony) handleFlipLotteryPeriod(block *types.Block) {
	if block.Header.Flags().HasFlag(types.FlipLotteryStarted) {

		seedHeight := uint64(2)
		if block.Height()+seedHeight > LotterySeedLag {
			seedHeight = block.Height() + seedHeight - LotterySeedLag
		}
		seedBlock := vc.chain.GetBlockHeaderByHeight(seedHeight)

		vc.epochDb.WriteLotterySeed(seedBlock.Seed().Bytes())
		vc.calculateCeremonyCandidates()
		vc.logInfoWithInteraction("Flip lottery started")
	}
}

func (vc *ValidationCeremony) handleShortSessionPeriod(block *types.Block) {
	timestamp := vc.epochDb.ReadShortSessionTime()
	if timestamp != nil {
		vc.appState.EvidenceMap.SetShortSessionTime(timestamp, vc.config.Validation.GetShortSessionDuration())
	} else if block.Header.Flags().HasFlag(types.ShortSessionStarted) {
		t := time.Now().UTC()
		vc.epochDb.WriteShortSessionTime(t)
		vc.appState.EvidenceMap.SetShortSessionTime(&t, vc.config.Validation.GetShortSessionDuration())
		if vc.shouldInteractWithNetwork() {
			vc.logInfoWithInteraction("Short session started", "at", t.String())
		}
	}
	vc.broadcastFlipKey()
	vc.processCeremonyTxs(block)
}

func (vc *ValidationCeremony) handleLongSessionPeriod(block *types.Block) {
	if block.Header.Flags().HasFlag(types.LongSessionStarted) {
		vc.logInfoWithInteraction("Long session started")
	}
	vc.broadcastShortAnswersTx()
	vc.broadcastFlipKey()
	vc.processCeremonyTxs(block)
	vc.broadcastEvidenceMap(block)
}

func (vc *ValidationCeremony) handleAfterLongSessionPeriod(block *types.Block) {
	vc.processCeremonyTxs(block)
}

func (vc *ValidationCeremony) calculateCeremonyCandidates() {
	if vc.candidates != nil {
		return
	}

	seed := vc.epochDb.ReadLotterySeed()
	if seed == nil {
		return
	}

	vc.candidates, vc.nonCandidates, vc.flips, vc.flipsPerAuthor = vc.getCandidatesAndFlips()

	shortFlipsPerCandidate := SortFlips(vc.flipsPerAuthor, vc.candidates, vc.flips, int(vc.ShortSessionFlipsCount()+common.ShortSessionExtraFlipsCount()), seed, false, nil)

	chosenFlips := make(map[int]bool)
	for _, a := range shortFlipsPerCandidate {
		for _, f := range a {
			chosenFlips[f] = true
		}
	}

	longFlipsPerCandidate := SortFlips(vc.flipsPerAuthor, vc.candidates, vc.flips, int(vc.LongSessionFlipsCount()), common.ReverseBytes(seed), true, chosenFlips)

	vc.shortFlipsPerCandidate = shortFlipsPerCandidate
	vc.longFlipsPerCandidate = longFlipsPerCandidate

	vc.shortFlipsToSolve = getFlipsToSolve(vc.secStore.GetAddress(), vc.candidates, vc.shortFlipsPerCandidate, vc.flips)
	vc.longFlipsToSolve = getFlipsToSolve(vc.secStore.GetAddress(), vc.candidates, vc.longFlipsPerCandidate, vc.flips)

	if vc.shouldInteractWithNetwork() {
		go vc.flipper.Load(vc.shortFlipsToSolve)
		go vc.flipper.Load(vc.longFlipsToSolve)
	}

	vc.logInfoWithInteraction("Ceremony candidates", "cnt", len(vc.candidates))

	if len(vc.candidates) < 100 {
		var addrs []string
		for _, c := range vc.getParticipantsAddrs() {
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
		conf.GetAfterLongSessionDuration() +
		time.Minute*5 // added extra minutes to prevent time lags
	headTime := common.TimestampToTime(vc.chain.Head.Time())

	// if head's timestamp is close to now() we should interact with network
	return time.Now().UTC().Sub(headTime) < ceremonyDuration
}

func (vc *ValidationCeremony) broadcastFlipKey() {
	if vc.keySent || !vc.shouldInteractWithNetwork() || !vc.shouldBroadcastFlipKey() {
		return
	}

	epoch := vc.appState.State.Epoch()
	key := vc.flipper.GetFlipEncryptionKey()

	msg := types.FlipKey{
		Key:   crypto.FromECDSA(key.ExportECDSA()),
		Epoch: epoch,
	}

	signedMsg, err := vc.secStore.SignFlipKey(&msg)

	if err != nil {
		vc.log.Error("cannot sign flip key", "epoch", epoch, "err", err)
		return
	}

	vc.keysPool.Add(signedMsg)
	vc.keySent = true
}

func (vc *ValidationCeremony) getCandidatesAndFlips() ([]*candidate, []common.Address, [][]byte, map[int][][]byte) {
	nonCandidates := make([]common.Address, 0)
	m := make([]*candidate, 0)
	flips := make([][]byte, 0)
	flipsPerAuthor := make(map[int][][]byte)

	addFlips := func(candidateFlips [][]byte) {
		for _, f := range candidateFlips {
			flips = append(flips, f)
			flipsPerAuthor[len(m)] = append(flipsPerAuthor[len(m)], f)
		}
	}

	vc.appState.State.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])

		var data state.Identity
		if err := rlp.DecodeBytes(value, &data); err != nil {
			return false
		}
		if state.IsCeremonyCandidate(data) {
			addFlips(data.Flips)
			m = append(m, &candidate{
				Address:    addr,
				Generation: data.Generation,
				Code:       data.Code,
			})
		} else {
			nonCandidates = append(nonCandidates, addr)
		}

		return false
	})

	return m, nonCandidates, flips, flipsPerAuthor
}

func (vc *ValidationCeremony) getParticipantsAddrs() []common.Address {
	var candidates []*candidate
	if vc.candidates != nil {
		candidates = vc.candidates
	} else {
		candidates, _, _, _ = vc.getCandidatesAndFlips()
	}
	var result []common.Address
	for _, p := range candidates {
		result = append(result, p.Address)
	}
	return result
}

func getFlipsToSolve(self common.Address, participants []*candidate, flipsPerCandidate [][]int, flipCids [][]byte) [][]byte {
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
			attachment := attachments.ParseShortAnswerAttachment(tx)
			if attachment == nil {
				log.Error("short answer attachment is invalid", "tx", tx.Hash())
				continue
			}
			vc.qualification.addAnswers(true, sender, attachment.Answers)
		case types.SubmitLongAnswersTx:
			vc.qualification.addAnswers(false, sender, tx.Payload)
		case types.EvidenceTx:
			if !vc.epochDb.HasEvidenceMap(sender) {
				vc.epochDb.WriteEvidenceMap(sender, tx.Payload)
			}
		}
	}
}

func (vc *ValidationCeremony) broadcastShortAnswersTx() {
	if vc.shortAnswersSent || !vc.shouldInteractWithNetwork() || !vc.IsCandidate() {
		return
	}
	answers := vc.epochDb.ReadOwnShortAnswersBits()
	if answers == nil {
		vc.log.Error("short session answers are missing")
		return
	}

	key := vc.flipper.GetFlipEncryptionKey()
	_, proof := vc.epochDb.ReadFlipKeyWordPairs()

	var pairs []uint8
	myFlips := vc.appState.State.GetIdentity(vc.secStore.GetAddress()).Flips
	for _, key := range myFlips {
		localSavedPair := vc.epochDb.ReadFlipPair(key)
		if localSavedPair != nil {
			pairs = append(pairs, *localSavedPair)
			continue
		}
		flip, err := vc.flipper.GetRawFlip(key)
		if err != nil {
			cid, _ := cid.Parse(key)
			vc.log.Error(fmt.Sprintf("flip is missing, cid: %v", cid.String()))
			pairs = append(pairs, 0)
		} else {
			pairs = append(pairs, flip.Pair)
		}
	}

	ipfsAnswer := &attachments.ShortAnswerAttachment{
		Answers: answers,
		Proof:   proof,
		Key:     crypto.FromECDSA(key.ExportECDSA()),
		Pairs:   pairs,
	}

	payload, _ := rlp.EncodeToBytes(ipfsAnswer)

	vc.sendTx(types.SubmitShortAnswersTx, payload)
	vc.shortAnswersSent = true
}

func (vc *ValidationCeremony) broadcastEvidenceMap(block *types.Block) {
	if vc.evidenceSent || !vc.shouldInteractWithNetwork() || !vc.IsCandidate() {
		return
	}

	shortSessionStart, shortSessionEnd := vc.appState.EvidenceMap.GetShortSessionBeginningTime(), vc.appState.EvidenceMap.GetShortSessionEndingTime()

	additional := vc.epochDb.GetConfirmedRespondents(*shortSessionStart, *shortSessionEnd)

	candidates := vc.getParticipantsAddrs()

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

	vc.sendTx(types.EvidenceTx, buf.Bytes())
	vc.evidenceSent = true
}

func (vc *ValidationCeremony) sendTx(txType uint16, payload []byte) (common.Hash, error) {
	vc.mutex.Lock()
	defer vc.mutex.Unlock()

	signedTx := &types.Transaction{}

	if existTx := vc.epochDb.ReadOwnTx(txType); existTx != nil {
		rlp.DecodeBytes(existTx, signedTx)
	} else {
		addr := vc.secStore.GetAddress()
		tx := blockchain.BuildTx(vc.appState, addr, nil, txType, decimal.Zero, 0, 0, payload)
		var err error
		signedTx, err = vc.secStore.SignTx(tx)
		if err != nil {
			vc.log.Error(err.Error())
			return common.Hash{}, err
		}
		txBytes, _ := rlp.EncodeToBytes(signedTx)
		vc.epochDb.WriteOwnTx(txType, txBytes)
	}

	err := vc.mempool.Add(signedTx)

	if err != nil {
		vc.log.Error(err.Error())
		vc.epochDb.RemoveOwnTx(txType)
	}
	vc.logInfoWithInteraction("Broadcast ceremony tx", "type", txType, "hash", signedTx.Hash().Hex())

	return signedTx.Hash(), err
}

func (vc *ValidationCeremony) ApplyNewEpoch(appState *appstate.AppState) (identitiesCount int) {

	vc.applyEpochMutex.Lock()
	defer vc.applyEpochMutex.Unlock()

	applyOnState := func(addr common.Address, value cacheValue) {
		appState.State.SetState(addr, value.state)
		appState.State.AddQualifiedFlipsCount(addr, value.shortQualifiedFlipsCount)
		appState.State.AddShortFlipPoints(addr, value.shortFlipPoint)
		appState.State.SetBirthday(addr, value.birthday)
		if value.state == state.Verified || value.state == state.Newbie {
			identitiesCount++
		}
	}

	if len(vc.epochApplyingResult) > 0 {
		for addr, value := range vc.epochApplyingResult {
			applyOnState(addr, value)
		}
		return identitiesCount
	}

	stats := NewStats()
	stats.FlipCids = vc.flips
	approvedCandidates := vc.appState.EvidenceMap.CalculateApprovedCandidates(vc.getParticipantsAddrs(), vc.epochDb.ReadEvidenceMaps())
	approvedCandidatesSet := mapset.NewSet()
	for _, item := range approvedCandidates {
		approvedCandidatesSet.Add(item)
	}

	totalFlipsCount := len(vc.flips)

	flipQualification := vc.qualification.qualifyFlips(uint(totalFlipsCount), vc.candidates, vc.longFlipsPerCandidate)

	flipQualificationMap := make(map[int]FlipQualification)
	for i, item := range flipQualification {
		flipQualificationMap[i] = item
		stats.FlipsPerIdx[i] = &FlipStats{
			Status: item.status,
			Answer: item.answer,
		}
	}

	vc.logInfoWithInteraction("Approved candidates", "cnt", len(approvedCandidates))

	notApprovedFlips := vc.getNotApprovedFlips(approvedCandidatesSet)

	for idx, candidate := range vc.candidates {
		addr := candidate.Address
		var shortScore, longScore, totalScore float32
		shortFlipsToSolve := vc.shortFlipsPerCandidate[idx]
		shortFlipPoint, shortQualifiedFlipsCount, shortFlipAnswers := vc.qualification.qualifyCandidate(addr, flipQualificationMap, shortFlipsToSolve, true, notApprovedFlips)
		addFlipAnswersToStats(shortFlipAnswers, true, stats)

		longFlipsToSolve := vc.longFlipsPerCandidate[idx]
		longFlipPoint, longQualifiedFlipsCount, longFlipAnswers := vc.qualification.qualifyCandidate(addr, flipQualificationMap, longFlipsToSolve, false, notApprovedFlips)
		addFlipAnswersToStats(longFlipAnswers, false, stats)

		totalFlipPoints := appState.State.GetShortFlipPoints(addr)
		totalQualifiedFlipsCount := appState.State.GetQualifiedFlipsCount(addr)
		approved := approvedCandidatesSet.Contains(addr)
		missed := !approved

		if shortQualifiedFlipsCount > 0 {
			shortScore = shortFlipPoint / float32(shortQualifiedFlipsCount)
		} else {
			missed = true
		}
		if longQualifiedFlipsCount > 0 {
			longScore = longFlipPoint / float32(longQualifiedFlipsCount)
		} else {
			missed = true
		}
		newTotalQualifiedFlipsCount := shortQualifiedFlipsCount + totalQualifiedFlipsCount
		if newTotalQualifiedFlipsCount > 0 {
			totalScore = (shortFlipPoint + totalFlipPoints) / float32(newTotalQualifiedFlipsCount)
		}

		identity := appState.State.GetIdentity(addr)
		newIdentityState := determineNewIdentityState(identity, shortScore, longScore, totalScore,
			newTotalQualifiedFlipsCount, missed)
		identityBirthday := determineIdentityBirthday(vc.epoch, identity, newIdentityState)

		value := cacheValue{
			state:                    newIdentityState,
			shortQualifiedFlipsCount: shortQualifiedFlipsCount,
			shortFlipPoint:           shortFlipPoint,
			birthday:                 identityBirthday,
		}

		vc.epochApplyingResult[addr] = value

		stats.IdentitiesPerAddr[addr] = &IdentityStats{
			ShortPoint:        shortFlipPoint,
			ShortFlips:        shortQualifiedFlipsCount,
			LongPoint:         longFlipPoint,
			LongFlips:         longQualifiedFlipsCount,
			Approved:          approved,
			Missed:            missed,
			ShortFlipsToSolve: shortFlipsToSolve,
			LongFlipsToSolve:  longFlipsToSolve,
		}

		applyOnState(addr, value)
	}

	for _, addr := range vc.nonCandidates {
		identity := appState.State.GetIdentity(addr)
		newIdentityState := determineNewIdentityState(identity, 0, 0, 0,
			0, true)
		identityBirthday := determineIdentityBirthday(vc.epoch, identity, newIdentityState)

		value := cacheValue{
			state:                    newIdentityState,
			shortQualifiedFlipsCount: 0,
			shortFlipPoint:           0,
			birthday:                 identityBirthday,
		}
		vc.epochApplyingResult[addr] = value
		applyOnState(addr, value)
	}

	vc.validationStats = stats
	return identitiesCount
}

func addFlipAnswersToStats(answers map[int]FlipAnswerStats, isShort bool, stats *Stats) {
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
	case state.Newbie:
	case state.Verified:
	case state.Suspended:
	case state.Zombie:
		return identity.Birthday
	}
	return 0
}

func determineNewIdentityState(identity state.Identity, shortScore, longScore, totalScore float32, totalQualifiedFlips uint32, missed bool) state.IdentityState {

	if !identity.HasDoneAllRequiredFlips() {
		switch identity.State {
		case state.Verified:
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
		if missed || shortScore < MinShortScore || longScore < MinLongScore {
			return state.Killed
		}
		return state.Newbie
	case state.Newbie:
		if missed {
			return state.Killed
		}
		if totalQualifiedFlips > 10 && totalScore >= MinTotalScore && shortScore >= MinShortScore && longScore >= MinLongScore {
			return state.Verified
		}
		if totalQualifiedFlips <= 10 && shortScore >= MinShortScore && longScore >= 0.75 {
			return state.Newbie
		}
		return state.Killed
	case state.Verified:
		if missed {
			return state.Suspended
		}
		if totalQualifiedFlips > 10 && totalScore >= MinTotalScore && shortScore >= MinShortScore && longScore >= MinLongScore {
			return state.Verified
		}
		return state.Killed
	case state.Suspended:
		if missed {
			return state.Zombie
		}
		if totalScore >= MinTotalScore && shortScore >= MinShortScore && longScore >= MinLongScore {
			return state.Verified
		}
		return state.Killed
	case state.Zombie:
		if missed {
			return state.Killed
		}
		if totalScore >= MinTotalScore && shortScore >= MinShortScore {
			return state.Verified
		}
		return state.Killed
	case state.Killed:
		return state.Killed
	}
	return state.Undefined
}

func (vc *ValidationCeremony) FlipKeyWordPairs() []int {
	return vc.flipKeyWordPairs
}

func (vc *ValidationCeremony) generateFlipKeyWordPairs(seed []byte) {
	identity := vc.appState.State.GetIdentity(vc.secStore.GetAddress())
	words, proof := vc.GeneratePairs(seed, WordDictionarySize, WordPairsPerFlip*int(identity.RequiredFlips))
	var wordsToPersist []uint32
	for _, word := range words {
		wordsToPersist = append(wordsToPersist, uint32(word))
	}
	vc.epochDb.WriteFlipKeyWordPairs(wordsToPersist, proof)
	vc.flipKeyWordPairs = words
}

func (vc *ValidationCeremony) restoreFlipKeyWordPairs() {
	persistedWords, _ := vc.epochDb.ReadFlipKeyWordPairs()
	if persistedWords == nil {
		vc.generateFlipKeyWordPairs(vc.appState.State.FlipWordsSeed().Bytes())
		return
	}
	var words []int
	for _, persistedWord := range persistedWords {
		words = append(words, int(persistedWord))
	}
	vc.flipKeyWordPairs = words
}
