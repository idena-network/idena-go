package ceremony

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"github.com/shopspring/decimal"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/eventbus"
	"idena-go/core/appstate"
	"idena-go/core/flip"
	"idena-go/core/mempool"
	"idena-go/core/state"
	"idena-go/crypto"
	"idena-go/database"
	"idena-go/events"
	"idena-go/log"
	"idena-go/protocol"
	"idena-go/rlp"
	"idena-go/secstore"
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
)

type ValidationCeremony struct {
	bus                    eventbus.Bus
	db                     dbm.DB
	appState               *appstate.AppState
	flipper                *flip.Flipper
	pm                     *protocol.ProtocolManager
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
	mutex                  sync.Mutex
	epochDb                *database.EpochDb
	qualification          *qualification
	mempool                *mempool.TxPool
	chain                  *blockchain.Blockchain
	syncer                 protocol.Syncer
	blockHandlers          map[state.ValidationPeriod]blockHandler
	epochApplyingResult    map[common.Address]cacheValue
}

type cacheValue struct {
	state                    state.IdentityState
	shortQualifiedFlipsCount uint32
	shortFlipPoint           float32
}

type blockHandler func(block *types.Block)

func NewValidationCeremony(appState *appstate.AppState, bus eventbus.Bus, flipper *flip.Flipper, pm *protocol.ProtocolManager, secStore *secstore.SecStore, db dbm.DB, mempool *mempool.TxPool,
	chain *blockchain.Blockchain, syncer protocol.Syncer) *ValidationCeremony {

	vc := &ValidationCeremony{
		flipper:             flipper,
		appState:            appState,
		bus:                 bus,
		pm:                  pm,
		secStore:            secStore,
		log:                 log.New(),
		db:                  db,
		mempool:             mempool,
		epochApplyingResult: make(map[common.Address]cacheValue),
		chain:               chain,
		syncer:              syncer,
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
	vc.qualification = NewQualification(vc.epochDb)

	_ = vc.bus.Subscribe(events.AddBlockEventID,
		func(e eventbus.Event) {
			newBlockEvent := e.(*events.NewBlockEvent)
			vc.addBlock(newBlockEvent.Block)
		})

	vc.restoreState()
	vc.handleBlock(currentBlock)
}

func (vc *ValidationCeremony) addBlock(block *types.Block) {
	vc.handleBlock(block)
	vc.qualification.persistAnswers()

	// completeEpoch if finished
	if block.Header.Flags().HasFlag(types.ValidationFinished) {
		vc.completeEpoch()
	}
}

func (vc *ValidationCeremony) ShortSessionBeginTime() *time.Time {
	return vc.appState.EvidenceMap.GetShortSessionBeginningTime()
}

func (vc *ValidationCeremony) IsCandidate() bool {
	identity := vc.appState.State.GetIdentity(vc.secStore.GetAddress())
	return state.IsCeremonyCandidate(identity)
}

func (vc *ValidationCeremony) GetShortFlipsToSolve() [][]byte {
	return vc.shortFlipsToSolve
}

func (vc *ValidationCeremony) GetLongFlipsToSolve() [][]byte {
	return vc.longFlipsToSolve
}

func (vc *ValidationCeremony) SubmitShortAnswers(answers *types.Answers) (common.Hash, error) {
	vc.epochDb.WriteOwnShortAnswers(answers)
	hash := rlp.Hash(answers.Bytes())
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

func (vc *ValidationCeremony) restoreState() {
	timestamp := vc.epochDb.ReadShortSessionTime()
	if timestamp != nil {
		vc.appState.EvidenceMap.SetShortSessionTime(timestamp)
	}
	vc.qualification.restoreAnswers()
	vc.calculateCeremonyCandidates()
}

func (vc *ValidationCeremony) completeEpoch() {
	edb := vc.epochDb
	go func() {
		vc.dropFlips(edb)
		edb.Clear()
	}()

	vc.epochDb = database.NewEpochDb(vc.db, vc.appState.State.Epoch())
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
		vc.broadcastFlipKey()
		vc.logInfoWithInteraction("Flip lottery started")
	}
}

func (vc *ValidationCeremony) handleShortSessionPeriod(block *types.Block) {
	timestamp := vc.epochDb.ReadShortSessionTime()
	if timestamp != nil {
		vc.appState.EvidenceMap.SetShortSessionTime(timestamp)
	} else if block.Header.Flags().HasFlag(types.ShortSessionStarted) {
		t := time.Now().UTC()
		vc.epochDb.WriteShortSessionTime(t)
		vc.appState.EvidenceMap.SetShortSessionTime(&t)
		if vc.shouldInteractWithNetwork() {
			vc.logInfoWithInteraction("Short session started", "at", t.String())
		}
	}

	vc.processCeremonyTxs(block)
}

func (vc *ValidationCeremony) handleLongSessionPeriod(block *types.Block) {
	if block.Header.Flags().HasFlag(types.LongSessionStarted) {
		vc.logInfoWithInteraction("Long session started")
	}
	vc.broadcastShortAnswersTx()
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

	vc.candidates, vc.flips, vc.flipsPerAuthor = vc.getCandidatesAndFlips()

	shortFlipsPerCandidate := SortFlips(vc.flipsPerAuthor, vc.candidates, vc.flips, int(vc.ShortSessionFlipsCount()), seed, false, nil)

	chosenFlips := make(map[int]bool)
	for _, a := range shortFlipsPerCandidate {
		for _, f := range a {
			chosenFlips[f] = true
		}
	}

	longFlipsPerCandidate := SortFlips(vc.flipsPerAuthor, vc.candidates, vc.flips, int(vc.LongSessionFlipsCount()), seed, true, chosenFlips)

	vc.shortFlipsPerCandidate = shortFlipsPerCandidate
	vc.longFlipsPerCandidate = longFlipsPerCandidate

	vc.shortFlipsToSolve = getFlipsToSolve(vc.secStore.GetPubKey(), vc.candidates, vc.shortFlipsPerCandidate, vc.flips)
	vc.longFlipsToSolve = getFlipsToSolve(vc.secStore.GetPubKey(), vc.candidates, vc.longFlipsPerCandidate, vc.flips)

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
	if vc.keySent || !vc.shouldInteractWithNetwork() || !vc.IsCandidate() {
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

	vc.pm.BroadcastFlipKey(signedMsg)

	vc.keySent = true
}

func (vc *ValidationCeremony) getCandidatesAndFlips() ([]*candidate, [][]byte, map[int][][]byte) {
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
				PubKey:     data.PubKey,
				Generation: data.Generation,
				Code:       data.Code,
			})
		}

		return false
	})

	return m, flips, flipsPerAuthor
}

func (vc *ValidationCeremony) getParticipantsAddrs() []common.Address {
	var candidates []*candidate
	if vc.candidates != nil {
		candidates = vc.candidates
	} else {
		candidates, _, _ = vc.getCandidatesAndFlips()
	}
	var result []common.Address
	for _, p := range candidates {
		addr, _ := crypto.PubKeyBytesToAddress(p.PubKey)
		result = append(result, addr)
	}
	return result
}

func getFlipsToSolve(pubKey []byte, participants []*candidate, flipsPerCandidate [][]int, flipCids [][]byte) [][]byte {
	var result [][]byte
	for i := 0; i < len(participants); i++ {
		if bytes.Compare(participants[i].PubKey, pubKey) == 0 {
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
		if tx.Type == types.SubmitAnswersHashTx {
			vc.epochDb.WriteAnswerHash(*tx.To, common.BytesToHash(tx.Payload), time.Now().UTC())
		}

		if tx.Type == types.SubmitShortAnswersTx || tx.Type == types.SubmitLongAnswersTx {
			sender, _ := types.Sender(tx)
			vc.qualification.addAnswers(tx.Type == types.SubmitShortAnswersTx, sender, tx.Payload)
		}

		if tx.Type == types.EvidenceTx {
			vc.epochDb.WriteEvidenceMap(*tx.To, tx.Payload)
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
	vc.sendTx(types.SubmitShortAnswersTx, answers)
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

	signedTx := &types.Transaction{}

	if existTx := vc.epochDb.ReadOwnTx(txType); existTx != nil {
		rlp.DecodeBytes(existTx, signedTx)
	} else {
		addr := vc.secStore.GetAddress()
		tx := blockchain.BuildTx(vc.appState, addr, addr, txType, decimal.Zero, 0, 0, payload)
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
	}
	vc.logInfoWithInteraction("Broadcast ceremony tx", "type", txType, "hash", signedTx.Hash().Hex())

	return signedTx.Hash(), err
}

func (vc *ValidationCeremony) ApplyNewEpoch(appState *appstate.AppState) (identitiesCount int) {

	applyOnState := func(addr common.Address, s state.IdentityState, shortQualifiedFlipsCount uint32, shortFlipPoint float32) {
		appState.State.SetState(addr, s)
		appState.State.AddQualifiedFlipsCount(addr, shortQualifiedFlipsCount)
		appState.State.AddShortFlipPoints(addr, shortFlipPoint)
		if s == state.Verified || s == state.Newbie {
			identitiesCount++
		}
	}

	if len(vc.epochApplyingResult) > 0 {
		for addr, value := range vc.epochApplyingResult {
			applyOnState(addr, value.state, value.shortQualifiedFlipsCount, value.shortFlipPoint)
		}
		return identitiesCount
	}

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
	}

	vc.logInfoWithInteraction("Approved candidates", "cnt", len(approvedCandidates))

	notApprovedFlips := vc.getNotApprovedFlips(approvedCandidatesSet)

	for idx, candidate := range vc.candidates {
		addr, _ := crypto.PubKeyBytesToAddress(candidate.PubKey)

		shortFlipPoint, shortQualifiedFlipsCount := vc.qualification.qualifyCandidate(addr, flipQualificationMap, vc.shortFlipsPerCandidate[idx], true, notApprovedFlips)

		longFlipPoint, longQualifiedFlipsCount := vc.qualification.qualifyCandidate(addr, flipQualificationMap, vc.longFlipsPerCandidate[idx], false, notApprovedFlips)

		totalFlipPoints := appState.State.GetShortFlipPoints(addr)
		totalQualifiedFlipsCount := appState.State.GetQualifiedFlipsCount(addr)
		missed := !approvedCandidatesSet.Contains(addr)

		var shortScore, longScore, totalScore float32

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

		newIdentityState := determineNewIdentityState(appState.State.GetIdentityState(addr), shortScore, longScore, totalScore,
			newTotalQualifiedFlipsCount, missed)

		value := cacheValue{
			state:                    newIdentityState,
			shortQualifiedFlipsCount: shortQualifiedFlipsCount,
			shortFlipPoint:           shortFlipPoint,
		}

		vc.epochApplyingResult[addr] = value

		applyOnState(addr, newIdentityState, shortQualifiedFlipsCount, shortFlipPoint)
	}
	return identitiesCount
}

func (vc *ValidationCeremony) getNotApprovedFlips(approvedCandidates mapset.Set) mapset.Set {
	result := mapset.NewSet()
	for i, c := range vc.candidates {
		addr, _ := crypto.PubKeyBytesToAddress(c.PubKey)
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

func determineNewIdentityState(prevState state.IdentityState, shortScore, longScore, totalScore float32, totalQualifiedFlips uint32, missed bool) state.IdentityState {

	switch prevState {
	case state.Undefined:
	case state.Invite:
		return state.Undefined
	case state.Candidate:
		if missed || shortScore < MinShortScore || longScore < MinLongScore {
			return state.Killed
		}
		return state.Newbie
	case state.Newbie:
		if missed {
			return state.Killed
		}
		if totalQualifiedFlips >= 10 && totalScore >= MinTotalScore && shortScore >= MinShortScore && longScore >= MinLongScore {
			return state.Verified
		}
		if totalQualifiedFlips < 10 && shortScore >= MinShortScore && longScore >= 0.75 {
			return state.Newbie
		}
		return state.Killed
	case state.Verified:
		if missed {
			return state.Suspended
		}
		if totalQualifiedFlips >= 10 && totalScore >= MinTotalScore && shortScore >= MinShortScore && longScore >= MinLongScore {
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
