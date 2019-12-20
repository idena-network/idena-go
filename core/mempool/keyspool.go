package mempool

import (
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/secstore"
	"github.com/ipfs/go-cid"
	"sync"
)

type KeysPackage struct {
	RawPackage *types.PrivateFlipKeysPackage
	Cid        cid.Cid
}

type KeysPool struct {
	appState                  *appstate.AppState
	flipKeys                  map[common.Address]*types.PublicFlipKey
	knownKeys                 mapset.Set
	mutex                     sync.Mutex
	bus                       eventbus.Bus
	head                      *types.Header
	log                       log.Logger
	flipKeyPackages           map[common.Address]*KeysPackage
	knownKeyPackages          mapset.Set
	privateKeyIndexes         map[common.Address]int // shows which key (by index) use from author's package
	encryptedPrivateKeysCache map[common.Address]*ecies.PrivateKey
	ipfsProxy                 ipfs.Proxy
	secStore                  *secstore.SecStore
}

func NewKeysPool(appState *appstate.AppState, bus eventbus.Bus, secStore *secstore.SecStore, ipfsProxy ipfs.Proxy) *KeysPool {
	return &KeysPool{
		appState:                  appState,
		bus:                       bus,
		knownKeys:                 mapset.NewSet(),
		log:                       log.New(),
		flipKeys:                  make(map[common.Address]*types.PublicFlipKey),
		knownKeyPackages:          mapset.NewSet(),
		flipKeyPackages:           make(map[common.Address]*KeysPackage),
		encryptedPrivateKeysCache: make(map[common.Address]*ecies.PrivateKey),
		secStore:                  secStore,
		ipfsProxy:                 ipfsProxy,
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

func (p *KeysPool) AddPrivateKeysPackage(keysPackage *types.PrivateFlipKeysPackage, own bool) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	hash := keysPackage.Hash()

	if p.knownKeys.Contains(hash) {
		return errors.New("keys package with same hash already exists")
	}

	sender, _ := types.SenderFlipKeysPackage(keysPackage)

	if _, ok := p.flipKeys[sender]; ok {
		return errors.New("sender has already published his keys package")
	}

	appState := p.appState.Readonly(p.head.Height())

	if err := validateFlipKeysPackage(appState, keysPackage); err != nil {
		log.Warn("PrivateFLipKeysPackage is not valid", "hash", hash.Hex(), "err", err)
		return err
	}

	var c cid.Cid
	var err error
	data, _ := rlp.EncodeToBytes(keysPackage)
	if own {
		c, err = p.ipfsProxy.Add(data)
	} else {
		c, err = p.ipfsProxy.Cid(data)
	}
	if err != nil {
		return err
	}

	p.knownKeyPackages.Add(hash)
	p.flipKeyPackages[sender] = &KeysPackage{
		RawPackage: keysPackage,
		Cid:        c,
	}

	p.bus.Publish(&events.NewFlipKeysPackageEvent{
		Key: keysPackage,
		Own: own,
	})

	return nil
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

	if data, ok := p.encryptedPrivateKeysCache[address]; ok {
		return data
	}

	publicFlipKey := p.GetPublicFlipKey(address)
	if publicFlipKey == nil {
		return nil
	}

	keysPackage, ok := p.flipKeyPackages[address]
	if !ok {
		return nil
	}

	idx, ok := p.privateKeyIndexes[address]
	if !ok {
		return nil
	}

	encryptedFlipKey, err := getEncryptedKeyFromPackage(publicFlipKey, keysPackage.RawPackage.Data, idx)
	if err != nil {
		log.Error("Cannot get key from package", "err", err)
		return nil
	}

	rawKey, err := p.secStore.DecryptMessage(encryptedFlipKey)
	if err != nil {
		log.Error("Cannot decrypt key from package", "err", err)
		return nil
	}

	ecdsaKey, err := crypto.ToECDSA(rawKey)
	if err != nil {
		log.Error("Cannot convert decrypted key to ECDSA", "err", err)
		return nil
	}

	result := ecies.ImportECDSA(ecdsaKey)
	p.encryptedPrivateKeysCache[address] = result
	return result
}

func (p *KeysPool) Clear() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.privateKeyIndexes = nil
	p.knownKeys = mapset.NewSet()
	p.knownKeyPackages = mapset.NewSet()
	p.flipKeys = make(map[common.Address]*types.PublicFlipKey)
	p.flipKeyPackages = make(map[common.Address]*KeysPackage)
	p.encryptedPrivateKeysCache = make(map[common.Address]*ecies.PrivateKey)
}

func (p *KeysPool) ProvideAuthorIndexes(indexes map[common.Address]int) {
	p.privateKeyIndexes = indexes
}

func validateFlipKey(appState *appstate.AppState, key *types.PublicFlipKey) error {
	sender, _ := types.SenderFlipKey(key)
	return validateKey(sender, key.Epoch, appState)
}

func validateFlipKeysPackage(appState *appstate.AppState, keysPackage *types.PrivateFlipKeysPackage) error {
	sender, _ := types.SenderFlipKeysPackage(keysPackage)
	return validateKey(sender, keysPackage.Epoch, appState)
}

func validateKey(sender common.Address, epoch uint16, appState *appstate.AppState) error {
	if sender == (common.Address{}) {
		return errors.New("invalid signature")
	}

	if appState.State.Epoch() != epoch {
		return errors.New("invalid epoch")
	}

	identity := appState.State.GetIdentity(sender)
	if len(identity.Flips) == 0 {
		return errors.New("flips is missing")
	}
	return nil
}

func encryptPrivateKeysPackage(publicFlipKey *ecies.PrivateKey, privateFlipKey *ecies.PrivateKey, pubKeys [][]byte) []byte {
	keyToEncrypt := crypto.FromECDSA(privateFlipKey.ExportECDSA())

	var encryptedKeyPairs [][]byte
	for _, item := range pubKeys {
		ecdsaPubKey, err := crypto.UnmarshalPubkey(item)
		if err != nil {
			encryptedKeyPairs = append(encryptedKeyPairs, []byte{})
			continue
		}

		encryptedKey, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(ecdsaPubKey), keyToEncrypt, nil, nil)
		encryptedKeyPairs = append(encryptedKeyPairs, encryptedKey)
	}

	arrayToEncrypt, _ := rlp.EncodeToBytes(encryptedKeyPairs)

	encryptedArray, _ := ecies.Encrypt(rand.Reader, &publicFlipKey.PublicKey, arrayToEncrypt, nil, nil)

	return encryptedArray
}

func getEncryptedKeyFromPackage(publicFlipKey *ecies.PrivateKey, data []byte, index int) ([]byte, error) {
	decryptedPackage, err := publicFlipKey.Decrypt(data, nil, nil)
	if err != nil {
		return nil, err
	}
	var encryptedKeyPairs [][]byte
	if err := rlp.DecodeBytes(decryptedPackage, encryptedKeyPairs); err != nil {
		return nil, err
	}

	if index > len(encryptedKeyPairs)-1 {
		return nil, errors.New(fmt.Sprintf("ecnrypted private keys package length is invalid, current: %v, need index: %v", len(encryptedKeyPairs), index))
	}

	return encryptedKeyPairs[index], nil
}
