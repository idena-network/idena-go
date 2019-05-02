package ceremony

import (
	"bytes"
	"github.com/asaskevich/EventBus"
	"github.com/deckarep/golang-set"
	"github.com/shopspring/decimal"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/constants"
	"idena-go/core/appstate"
	"idena-go/core/flip"
	"idena-go/core/mempool"
	"idena-go/core/state"
	"idena-go/crypto"
	"idena-go/log"
	"idena-go/protocol"
	"idena-go/rlp"
	"idena-go/secstore"
	"sync"
	"time"
)

const (
	FlipsPerAddress = 10
)

const (
	MinTotalScore = 0.75
	MinShortScore = 0.5
	MinLongScore  = 0.75
)

type ValidationCeremony struct {
	bus                    EventBus.Bus
	db                     dbm.DB
	blocksChan             chan *types.Block
	appState               *appstate.AppState
	flipper                *flip.Flipper
	pm                     *protocol.ProtocolManager
	secStore               *secstore.SecStore
	log                    log.Logger
	shortFlipsPerCandidate [][]int
	longFlipsPerCandidate  [][]int
	shortFlipCidsToSolve   [][]byte
	longFlipCidsToSolve    [][]byte
	keySent                bool
	shortAnswersSent       bool
	evidenceSent           bool
	candidates             []*candidate
	mutex                  sync.Mutex
	epochDb                *EpochDb
	qualification          *qualification
	mempool                *mempool.TxPool

	blockHandlers map[state.ValidationPeriod]blockHandler
}

type blockHandler func(block *types.Block)

func NewValidationCeremony(appState *appstate.AppState, bus EventBus.Bus, flipper *flip.Flipper, pm *protocol.ProtocolManager, secStore *secstore.SecStore, db dbm.DB, mempool *mempool.TxPool) *ValidationCeremony {

	vc := &ValidationCeremony{
		flipper:    flipper,
		appState:   appState,
		bus:        bus,
		blocksChan: make(chan *types.Block, 100),
		pm:         pm,
		secStore:   secStore,
		log:        log.New(),
		db:         db,
		mempool:    mempool,
	}

	vc.blockHandlers = map[state.ValidationPeriod]blockHandler{
		state.NonePeriod:             func(block *types.Block) {},
		state.FlipLotteryPeriod:      vc.handleFlipLotterPeriod,
		state.ShortSessionPeriod:     vc.handleShortSessionPeriod,
		state.LongSessionPeriod:      vc.handleLongSessionPeriod,
		state.AfterLongSessionPeriod: vc.handleAfterLongSessionPeriod,
	}
	return vc
}

func (vc *ValidationCeremony) Start(currentBlock *types.Block) {
	vc.epochDb = NewEpochDb(vc.db, vc.appState.State.Epoch())
	vc.qualification = NewQualification(vc.epochDb)

	_ = vc.bus.Subscribe(constants.AddBlockEvent,
		func(block *types.Block) {
			vc.blocksChan <- block
		})

	vc.restoreState()
	vc.handleBlock(currentBlock)

	go vc.watchingLoop()
}

func (vc *ValidationCeremony) GetShortFlipsToSolve() [][]byte {
	return vc.shortFlipCidsToSolve
}

func (vc *ValidationCeremony) GetLongFlipsToSolve() [][]byte {
	return vc.longFlipCidsToSolve
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
	return FlipsPerAddress
}

func (vc *ValidationCeremony) LongSessionFlipsCount() uint {
	return FlipsPerAddress
}

func (vc *ValidationCeremony) restoreState() {
	timestamp := vc.epochDb.ReadShortSessionTime()
	if timestamp != nil {
		vc.appState.EvidenceMap.SetShortSessionTime(timestamp)
	}
	vc.qualification.restoreAnswers()
}

func (vc *ValidationCeremony) watchingLoop() {
	for {
		select {
		case block := <-vc.blocksChan:
			vc.handleBlock(block)
			vc.qualification.persistAnswers()

			// reset if finished
			if block.Header.Flags().HasFlag(types.ValidationFinished) {
				vc.reset()
			}
		}
	}
}

func (vc *ValidationCeremony) reset() {
	vc.epochDb.Clear()
	vc.epochDb = NewEpochDb(vc.db, vc.appState.State.Epoch())
	vc.qualification = NewQualification(vc.epochDb)
	vc.flipper.Reset()
	vc.appState.EvidenceMap.Clear()

	vc.candidates = nil
	vc.shortFlipsPerCandidate = nil
	vc.longFlipsPerCandidate = nil
	vc.shortFlipCidsToSolve = nil
	vc.longFlipCidsToSolve = nil
	vc.keySent = false
	vc.shortAnswersSent = false
	vc.evidenceSent = false
}

func (vc *ValidationCeremony) handleBlock(block *types.Block) {
	vc.blockHandlers[vc.appState.State.ValidationPeriod()](block)
}

func (vc *ValidationCeremony) handleFlipLotterPeriod(block *types.Block) {
	vc.calculateFlipCandidates(block)
	vc.broadcastFlipKey()
}

func (vc *ValidationCeremony) handleShortSessionPeriod(block *types.Block) {
	timestamp := vc.epochDb.ReadShortSessionTime()
	if timestamp != nil {
		vc.appState.EvidenceMap.SetShortSessionTime(timestamp)
	} else if block.Header.Flags().HasFlag(types.ShortSessionStarted) {
		t := time.Now()
		vc.epochDb.WriteShortSessionTime(t)
		vc.appState.EvidenceMap.SetShortSessionTime(&t)
	}

	vc.processCeremonyTxs(block)
}

func (vc *ValidationCeremony) handleLongSessionPeriod(block *types.Block) {
	vc.broadcastShortAnswersTx()
	vc.processCeremonyTxs(block)
	vc.broadcastEvidenceMap(block)
}

func (vc *ValidationCeremony) handleAfterLongSessionPeriod(block *types.Block) {
	vc.processCeremonyTxs(block)
}

func (vc *ValidationCeremony) calculateFlipCandidates(block *types.Block) {
	if vc.candidates != nil {
		return
	}

	vc.candidates = vc.getCandidates()

	shortFlipsPerCandidate, _ := SortFlips(len(vc.candidates), len(vc.appState.State.FlipCids()), int(vc.ShortSessionFlipsCount()), block.Header.Seed().Bytes())

	longFlipsPerCandidate, _ := SortFlips(len(vc.candidates), len(vc.appState.State.FlipCids()), int(vc.LongSessionFlipsCount()), block.Header.Seed().Bytes())

	vc.shortFlipsPerCandidate = shortFlipsPerCandidate
	vc.longFlipsPerCandidate = longFlipsPerCandidate

	vc.shortFlipCidsToSolve = getFlipsToSolve(vc.secStore.GetPubKey(), vc.candidates, vc.shortFlipsPerCandidate, vc.appState.State.FlipCids())
	vc.longFlipCidsToSolve = getFlipsToSolve(vc.secStore.GetPubKey(), vc.candidates, vc.longFlipsPerCandidate, vc.appState.State.FlipCids())

	go vc.flipper.Pin(vc.shortFlipCidsToSolve)
	go vc.flipper.Pin(vc.longFlipCidsToSolve)
}

func (vc *ValidationCeremony) broadcastFlipKey() {
	if vc.keySent {
		return
	}

	epoch := vc.appState.State.Epoch()
	key := vc.flipper.GetFlipEncryptionKey(epoch)

	msg := types.FlipKey{
		Key: crypto.FromECDSA(key.ExportECDSA()),
	}

	signedMsg, err := vc.secStore.SignFlipKey(&msg)

	if err != nil {
		vc.log.Error("cannot sign flip key", "epoch", epoch, "err", err)
		return
	}

	vc.pm.BroadcastFlipKey(signedMsg)

	vc.keySent = true
}

func (vc *ValidationCeremony) getCandidates() []*candidate {
	m := make([]*candidate, 0)

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

		if state.IsCeremonyCandidate(data.State) {
			m = append(m, &candidate{
				PubKey: data.PubKey,
			})
		}

		return false
	})

	return m
}

func (vc *ValidationCeremony) getParticipantsAddrs() []common.Address {
	var candidates []*candidate
	if vc.candidates != nil {
		candidates = vc.candidates
	} else {
		candidates = vc.getCandidates()
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
			vc.epochDb.WriteAnswerHash(*tx.To, common.BytesToHash(tx.Payload), time.Now())
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
	if vc.shortAnswersSent {
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
	if vc.evidenceSent {
		return
	}
	additional := vc.epochDb.GetConfirmedRespondents(vc.appState.EvidenceMap.GetShortSessionBeginningTime(), vc.appState.EvidenceMap.GetShortSessionEndingTime())
	bitmap := vc.appState.EvidenceMap.CalculateBitmap(vc.getParticipantsAddrs(), additional)
	vc.sendTx(types.EvidenceTx, bitmap)
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

	return signedTx.Hash(), err
}

func (vc *ValidationCeremony) ApplyNewEpoch(appState *appstate.AppState) {

	clearIdentityState(appState)

	approvedCandidates := vc.appState.EvidenceMap.CalculateApprovedCandidates(vc.getParticipantsAddrs(), vc.epochDb.ReadEvidenceMaps())
	approvedCandidatesSet := mapset.NewSet()
	for _, item := range approvedCandidates {
		approvedCandidatesSet.Add(item)
	}

	totalFlipsCount := len(appState.State.FlipCids())
	flipQualification := vc.qualification.qualifyFlips(uint(totalFlipsCount), vc.LongSessionFlipsCount(), vc.candidates, vc.longFlipsPerCandidate)

	flipQualificationMap := make(map[int]FlipQualification)
	for i, item := range flipQualification {
		flipQualificationMap[i] = item
	}

	for idx, candidate := range vc.candidates {
		addr, _ := crypto.PubKeyBytesToAddress(candidate.PubKey)

		shortFlipPoint, shortQualifiedFlipsCount := vc.qualification.qualifyCandidate(addr, flipQualificationMap, vc.shortFlipsPerCandidate[idx], true)

		longFlipPoint, longQualifiedFlipsCount := vc.qualification.qualifyCandidate(addr, flipQualificationMap, vc.longFlipsPerCandidate[idx], false)

		totalFlipPoints := appState.State.GetShortFlipPoints(addr)
		totalQualifiedFlipsCount := appState.State.GetQualifiedFlipsCount(addr)

		shortScore := shortFlipPoint / float32(shortQualifiedFlipsCount)
		longScore := longFlipPoint / float32(longQualifiedFlipsCount)
		totalScore := (shortFlipPoint + totalFlipPoints) / float32(shortQualifiedFlipsCount+totalQualifiedFlipsCount)

		newIdentityState := determineNewIdentityState(appState.State.GetIdentityState(addr), shortScore, longScore, totalScore,
			shortQualifiedFlipsCount+totalQualifiedFlipsCount, !approvedCandidatesSet.Contains(addr))

		appState.State.SetState(addr, newIdentityState)
		appState.State.AddQualifiedFlipsCount(addr, shortQualifiedFlipsCount)
		appState.State.AddShortFlipPoints(addr, shortFlipPoint)

		if newIdentityState == state.Newbie || newIdentityState == state.Verified {
			appState.IdentityState.Add(addr)
		}
	}
}

func clearIdentityState(appState *appstate.AppState) {
	var addrs []common.Address
	appState.IdentityState.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])
		addrs = append(addrs, addr)
		return false
	})

	for _, addr := range addrs {
		appState.IdentityState.Remove(addr)
	}

}

func determineNewIdentityState(prevState state.IdentityState, shortScore, longScore, totalScore float32, totalQualifiedFlips uint32, missed bool) state.IdentityState {

	switch prevState {
	case state.Undefined:
	case state.Invite:
		return state.Undefined
	case state.Candidate:
		if shortScore < MinShortScore || longScore < MinLongScore {
			return state.Killed
		}
		return state.Newbie
	case state.Newbie:
		if totalQualifiedFlips >= 10 && totalScore >= MinTotalScore && shortScore >= MinShortScore && longScore >= MinLongScore {
			return state.Verified
		}
		if totalQualifiedFlips < 10 && shortScore >= MinShortScore && longScore >= 0.75 {
			return state.Newbie
		}
		return state.Killed
	case state.Verified:
		if totalQualifiedFlips >= 10 && totalScore >= MinTotalScore && shortScore >= MinShortScore && longScore >= MinLongScore {
			return state.Verified
		}
		if missed {
			return state.Suspended
		}
		return state.Killed
	case state.Suspended:
		if totalScore >= MinTotalScore && shortScore >= MinShortScore && longScore >= MinLongScore {
			return state.Verified
		}
		if missed {
			return state.Zombie
		}
		return state.Killed
	case state.Zombie:
		if totalScore >= MinTotalScore && shortScore >= MinShortScore {
			return state.Verified
		}
		return state.Killed
	case state.Killed:
		return state.Killed
	}
	return state.Undefined
}
