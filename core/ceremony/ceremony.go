package ceremony

import (
	"bytes"
	"github.com/asaskevich/EventBus"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/constants"
	"idena-go/core/appstate"
	"idena-go/core/flip"
	"idena-go/core/state"
	"idena-go/crypto"
	"idena-go/database"
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
	participants       []*Participant
	mutex              *sync.Mutex
	epochDb            *database.EpochDb
	qualification      *qualification
}

func NewValidationCeremony(appState *appstate.AppState, bus EventBus.Bus, flipper *flip.Flipper, pm *protocol.ProtocolManager, secStore *secstore.SecStore, db dbm.DB) *ValidationCeremony {
	return &ValidationCeremony{
		flipper:    flipper,
		appState:   appState,
		bus:        bus,
		blocksChan: make(chan *types.Block, 100),
		pm:         pm,
		secStore:   secStore,
		log:        log.New(),
		db:         db,
	}
}

func (vc *ValidationCeremony) Start(currentBlock *types.Block) {
	vc.epochDb = database.NewEpochDb(vc.db, vc.appState.State.Epoch())
	vc.qualification = NewQualification(vc.epochDb)

	_ = vc.bus.Subscribe(constants.AddBlockEvent, func(block *types.Block) { vc.blocksChan <- block })

	vc.restoreState()
	vc.processBlock(currentBlock)

	go vc.watchingLoop()
}

func (vc *ValidationCeremony) restoreState() {
	timestamp := vc.epochDb.ReadShortSessionTime()
	if timestamp != nil {
		vc.appState.EvidenceMap.SetShortSessionTime(timestamp)
	}
	vc.qualification.restoreAnswers()
}

func (vc *ValidationCeremony) GetFlipsToSolve() [][]byte {
	return vc.flipsToSolve
}

func (vc *ValidationCeremony) SaveOwnShortAnswers(answers []*types.FlipAnswer) (common.Hash, error) {
	err := vc.epochDb.WriteOwnShortAnswers(answers)
	if err != nil {
		return common.Hash{}, err
	}

	return common.Hash(rlp.Hash(answers)), nil
}

func (vc *ValidationCeremony) watchingLoop() {
	for {
		select {
		case block := <-vc.blocksChan:
			vc.processBlock(block)
			vc.qualification.persistAnswers()
		}
	}
}

func (vc *ValidationCeremony) processBlock(block *types.Block) {
	if vc.appState.State.ValidationPeriod() == state.NonePeriod {
		return
	}
	if block.Header.Flags().HasFlag(types.ShortSessionStarted) {
		t := time.Now()
		vc.epochDb.WriteShortSessionTime(t)
		vc.appState.EvidenceMap.SetShortSessionTime(&t)
	}

	vc.calculateFlipCandidates(block)
	vc.processCeremonyTxs(block)
	vc.broadcastFlipKey()
	vc.broadcastEvidenceMap(block)
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

type Participant struct {
	PubKey    []byte
	Candidate bool
}

func (vc *ValidationCeremony) getParticipants() []*Participant {
	m := make([]*Participant, 0)

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
			m = append(m, &Participant{
				PubKey:    data.PubKey,
				Candidate: false,
			})
		}
		if data.State == state.Candidate {
			m = append(m, &Participant{
				PubKey:    data.PubKey,
				Candidate: true,
			})
		}

		return false
	})

	return m
}

func getFlipsToSolve(pubKey []byte, participants []*Participant, flipsPerCandidate [][]int, flipCids [][]byte) [][]byte {
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
			vc.epochDb.WriteAnswerHash(*tx.To, common.BytesToHash(tx.Payload))
		}

		if tx.Type == types.SubmitShortAnswersTx || tx.Type == types.SubmitLongAnswersTx {
			sender, _ := types.Sender(tx)
			vc.qualification.addAnswers(tx.Type == types.SubmitShortAnswersTx, sender, tx.Payload)
		}
	}
}

func (vc *ValidationCeremony) broadcastEvidenceMap(block *types.Block) {
	if block.Header.Flags().HasFlag(types.LongSessionStarted) {

	}
}
