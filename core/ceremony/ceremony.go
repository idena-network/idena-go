package ceremony

import (
	"github.com/asaskevich/EventBus"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/constants"
	"idena-go/core/appstate"
	"idena-go/core/flip"
	"idena-go/core/state"
	"idena-go/crypto"
	"idena-go/log"
	"idena-go/protocol"
	"idena-go/rlp"
	"idena-go/secstore"
)

const (
	FlipsPerAddress = 10
)

type ValidationCeremony struct {
	bus                EventBus.Bus
	blocksChan         chan *types.Block
	appState           *appstate.AppState
	flipper            *flip.Flipper
	pm                 *protocol.ProtocolManager
	secStore           *secstore.SecStore
	log                log.Logger
	flipsPerCandidate  [][]int
	candidatesPerFlips [][]int
	keySent            bool
}

func NewValidationCeremony(appState *appstate.AppState, bus EventBus.Bus, flipper *flip.Flipper, pm *protocol.ProtocolManager, secStore *secstore.SecStore) *ValidationCeremony {
	return &ValidationCeremony{
		flipper:    flipper,
		appState:   appState,
		bus:        bus,
		blocksChan: make(chan *types.Block, 100),
		pm:         pm,
		secStore:   secStore,
		log:        log.New(),
	}
}

func (vc *ValidationCeremony) Start(currentBlock *types.Block) {
	_ = vc.bus.Subscribe(constants.AddBlockEvent, func(block *types.Block) { vc.blocksChan <- block })

	vc.processBlock(currentBlock)

	go vc.watchingLoop()
}

func (vc *ValidationCeremony) watchingLoop() {
	for {
		select {
		case block := <-vc.blocksChan:
			vc.processBlock(block)
		}
	}
}

func (vc *ValidationCeremony) processBlock(block *types.Block) {
	vc.calculateFlipCandidates(block)
	vc.sendFlipKey()
}

func (vc *ValidationCeremony) calculateFlipCandidates(block *types.Block) {
	if vc.candidatesPerFlips != nil && vc.flipsPerCandidate != nil {
		return
	}

	if !vc.appState.State.HasGlobalFlag(state.FlipSubmissionStarted) {
		return
	}

	participants := vc.getParticipants()

	flipsPerCandidate, candidatesPerFlip := SortFlips(len(participants), FlipsPerAddress, block.Header.Seed().Bytes())

	vc.flipsPerCandidate = flipsPerCandidate
	vc.candidatesPerFlips = candidatesPerFlip
}

func (vc *ValidationCeremony) sendFlipKey() {
	if vc.keySent || !vc.appState.State.HasGlobalFlag(state.FlipSubmissionStarted) {
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
