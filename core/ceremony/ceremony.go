package ceremony

import (
	"bytes"
	"github.com/asaskevich/EventBus"
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

type ValidationCeremony struct {
	bus                EventBus.Bus
	db                 dbm.DB
	blocksChan         chan *types.Block
	appState           *appstate.AppState
	flipper            *flip.Flipper
	pm                 *protocol.ProtocolManager
	secStore           *secstore.SecStore
	log                log.Logger
	flipsPerCandidate  [][]int
	candidatesPerFlips [][]int
	flipsToSolve       [][]byte
	keySent            bool
	shortAnswersSent   bool
	evidenceSent       bool
	participants       []*participant
	mutex              *sync.Mutex
	epochDb            *EpochDb
	qualification      *qualification
	mempool            *mempool.TxPool

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

	_ = vc.bus.Subscribe(constants.AddBlockEvent, func(block *types.Block) { vc.blocksChan <- block })

	vc.restoreState()
	vc.handleBlock(currentBlock)

	go vc.watchingLoop()
}

func (vc *ValidationCeremony) GetFlipsToSolve() [][]byte {
	return vc.flipsToSolve
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
		}
	}
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

}

func (vc *ValidationCeremony) calculateFlipCandidates(block *types.Block) {
	vc.mutex.Lock()
	if vc.participants != nil {
		return
	}
	vc.participants = vc.getParticipants()
	vc.mutex.Unlock()

	flipsPerCandidate, candidatesPerFlip := SortFlips(len(vc.participants), FlipsPerAddress, block.Header.Seed().Bytes())

	vc.flipsPerCandidate = flipsPerCandidate
	vc.candidatesPerFlips = candidatesPerFlip

	vc.flipsToSolve = getFlipsToSolve(vc.secStore.GetPubKey(), vc.participants, vc.flipsPerCandidate, vc.appState.State.FlipCids())

	go vc.flipper.Pin(vc.flipsToSolve)
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

func (vc *ValidationCeremony) getParticipants() []*participant {
	m := make([]*participant, 0)

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

		if data.State == state.Verified {
			m = append(m, &participant{
				PubKey:    data.PubKey,
				Candidate: false,
			})
		}
		if data.State == state.Candidate {
			m = append(m, &participant{
				PubKey:    data.PubKey,
				Candidate: true,
			})
		}

		return false
	})

	return m
}

func (vc *ValidationCeremony) getParticipantsAddrs() []common.Address {
	participants := vc.getParticipants()
	var result []common.Address
	for _, p := range participants {
		addr, _ := crypto.PubKeyBytesToAddress(p.PubKey)
		result = append(result, addr)
	}
	return result
}

func getFlipsToSolve(pubKey []byte, participants []*participant, flipsPerCandidate [][]int, flipCids [][]byte) [][]byte {
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
