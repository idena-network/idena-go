package ceremony

import (
	"bytes"
	"crypto/rand"
	"github.com/asaskevich/EventBus"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/constants"
	"idena-go/core/appstate"
	"idena-go/core/flip"
	"idena-go/core/state"
	"idena-go/crypto"
	"idena-go/crypto/ecies"
	"idena-go/protocol"
	"idena-go/rlp"
)

const (
	FlipsPerAddress = 10
)

type ValidationCeremony struct {
	bus        EventBus.Bus
	blocksChan chan *types.Block
	appState   *appstate.AppState
	flipper    *flip.Flipper
	pm         *protocol.ProtocolManager
}

func NewValidationCeremony(appState *appstate.AppState, bus EventBus.Bus, flipper *flip.Flipper, pm *protocol.ProtocolManager) *ValidationCeremony {
	return &ValidationCeremony{
		flipper:    flipper,
		appState:   appState,
		bus:        bus,
		blocksChan: make(chan *types.Block, 100),
		pm:         pm,
	}
}

func (vc *ValidationCeremony) Start() {
	_ = vc.bus.Subscribe(constants.NewTxEvent, func(block *types.Block) { vc.blocksChan <- block })

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

	prevState := vc.appState.State.ForCheck(block.Height() - 1)
	flipSubmissionStarted := prevState.HasGlobalFlag(state.FlipSubmissionStarted)

	// check if flip submission started
	if !flipSubmissionStarted && vc.appState.State.HasGlobalFlag(state.FlipSubmissionStarted) {
		//vc.sendFlipKeys(block)
	}
}

func (vc *ValidationCeremony) sendFlipKeys(block *types.Block) {
	participants := vc.getParticipants()

	// TODO: need to get current-100 block height
	_, candidatesPerFlip := SortFlips(len(participants), FlipsPerAddress, block.Header.Seed().Bytes())

	allFlips := vc.appState.State.FlipCids()

	myKeys := vc.flipper.GetMyEncryptionKeys(vc.appState.State.Epoch())

	var packages []*types.KeyPackage

	for i := 0; i < len(allFlips); i++ {
		for j := 0; j < len(myKeys); j++ {
			// if this flip is our, we should send key
			if bytes.Compare(myKeys[j].Cid.Bytes(), allFlips[i]) == 0 {
				candidates := candidatesPerFlip[i]

				p := new(types.KeyPackage)

				for k := 0; k < len(candidates); k++ {
					ecdsaPubKey, _ := crypto.UnmarshalPubkey(participants[candidates[k]].PubKey)
					eciesPubKey := ecies.ImportECDSAPublic(ecdsaPubKey)
					encrypted, _ := ecies.Encrypt(rand.Reader, eciesPubKey, myKeys[j].Cid.Bytes(), nil, nil)
					p.Keys = append(p.Keys, encrypted)
				}

				packages = append(packages, p)
			}
		}
	}

	for _, item := range packages {
		vc.pm.BroadcastKeyPackage(item)
	}
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
