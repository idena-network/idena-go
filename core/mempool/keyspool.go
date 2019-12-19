package mempool

import (
	"errors"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/log"
	"sync"
)

type KeysPool struct {
	appState  *appstate.AppState
	flipKeys  map[common.Address]*types.PublicFlipKey
	knownKeys mapset.Set
	mutex     sync.Mutex
	bus       eventbus.Bus
	head      *types.Header
	log       log.Logger
}

func NewKeysPool(appState *appstate.AppState, bus eventbus.Bus) *KeysPool {
	return &KeysPool{
		appState:  appState,
		bus:       bus,
		knownKeys: mapset.NewSet(),
		log:       log.New(),
		flipKeys:  make(map[common.Address]*types.PublicFlipKey),
	}
}

func (p *KeysPool) Initialize(head *types.Header) {
	p.head = head

	_ = p.bus.Subscribe(events.AddBlockEventID,
		func(e eventbus.Event) {
			newBlockEvent := e.(*events.NewBlockEvent)
			p.head = newBlockEvent.Block.Header
		})
}

func (p *KeysPool) AddPublicFlipKey(key *types.PublicFlipKey, own bool) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	hash := key.Hash()

	if p.knownKeys.Contains(hash) {
		return errors.New("flipKey with same hash already exists")
	}

	sender, _ := types.SenderFlipKey(key)

	if _, ok := p.flipKeys[sender]; ok {
		return errors.New("sender has already published his key")
	}

	appState := p.appState.Readonly(p.head.Height())

	if err := validateFlipKey(appState, key); err != nil {
		log.Warn("PublicFlipKey is not valid", "hash", hash.Hex(), "err", err)
		return err
	}

	p.knownKeys.Add(hash)
	p.flipKeys[sender] = key

	p.appState.EvidenceMap.NewFlipsKey(sender)

	p.bus.Publish(&events.NewFlipKeyEvent{
		Key: key,
		Own: own,
	})

	return nil
}

func (p *KeysPool) AddPrivateKeysPackage(keysPackage *types.PrivateFlipKeysPackage, own bool) {

}

func (p *KeysPool) GetFlipKeys() []*types.PublicFlipKey {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	var list []*types.PublicFlipKey

	for _, tx := range p.flipKeys {
		list = append(list, tx)
	}
	return list
}

func (p *KeysPool) GetPublicFlipKey(address common.Address) *ecies.PrivateKey {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	key, ok := p.flipKeys[address]
	if !ok {
		return nil
	}

	ecdsaKey, _ := crypto.ToECDSA(key.Key)
	return ecies.ImportECDSA(ecdsaKey)
}

func (p *KeysPool) GetPrivateFlipKey(address common.Address) *ecies.PrivateKey {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	key, ok := p.flipKeys[address]
	if !ok {
		return nil
	}

	ecdsaKey, _ := crypto.ToECDSA(key.Key)
	return ecies.ImportECDSA(ecdsaKey)
}

func (p *KeysPool) Clear() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.knownKeys = mapset.NewSet()
	p.flipKeys = make(map[common.Address]*types.PublicFlipKey)
}

func validateFlipKey(appState *appstate.AppState, key *types.PublicFlipKey) error {
	sender, _ := types.SenderFlipKey(key)
	if sender == (common.Address{}) {
		return errors.New("invalid signature")
	}

	if appState.State.Epoch() != key.Epoch {
		return errors.New("invalid epoch")
	}

	identity := appState.State.GetIdentity(sender)
	if len(identity.Flips) == 0 {
		return errors.New("flips is missing")
	}
	return nil
}
