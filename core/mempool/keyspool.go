package mempool

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/pushpull"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/log"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/idena-network/idena-go/secstore"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"sync"
	"time"
)

var (
	maxFloat *big.Float
)

type FlipKeysPool interface {
	AddPrivateKeysPackage(keysPackage *types.PrivateFlipKeysPackage, own bool) error
	AddPublicFlipKey(key *types.PublicFlipKey, own bool) error
	GetFlipPackagesHashes() []common.Hash128
	GetFlipKeys() []*types.PublicFlipKey
}

func init() {
	maxFloat = new(big.Float).SetInt(new(big.Int).SetBytes(common.MaxHash[:]))
}

type KeysPool struct {
	db                        dbm.DB
	appState                  *appstate.AppState
	flipKeys                  map[common.Address]*types.PublicFlipKey
	publicKeyMutex            sync.RWMutex
	privateKeysMutex          sync.RWMutex
	bus                       eventbus.Bus
	head                      *types.Header
	log                       log.Logger
	flipKeyPackages           map[common.Address]*types.PrivateFlipKeysPackage
	flipKeyPackagesByHash     map[common.Hash128]*types.PrivateFlipKeysPackage
	privateKeyIndexes         map[common.Address]int // shows which key (by index) use from author's package
	encryptedPrivateKeysCache map[common.Address]*ecies.PrivateKey
	secStore                  *secstore.SecStore
	packagesLoadingCtx        context.Context
	cancelLoadingCtx          context.CancelFunc
	self                      common.Address
	pushTracker               pushpull.PendingPushTracker
}

func NewKeysPool(db dbm.DB, appState *appstate.AppState, bus eventbus.Bus, secStore *secstore.SecStore) *KeysPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &KeysPool{
		db:                        db,
		appState:                  appState,
		bus:                       bus,
		log:                       log.New(),
		flipKeys:                  make(map[common.Address]*types.PublicFlipKey),
		flipKeyPackages:           make(map[common.Address]*types.PrivateFlipKeysPackage),
		flipKeyPackagesByHash:     make(map[common.Hash128]*types.PrivateFlipKeysPackage),
		encryptedPrivateKeysCache: make(map[common.Address]*ecies.PrivateKey),
		secStore:                  secStore,
		packagesLoadingCtx:        ctx,
		cancelLoadingCtx:          cancel,
		pushTracker:               pushpull.NewDefaultPushTracker(time.Second * 5),
	}
	pool.pushTracker.SetHolder(pool)
	return pool
}

func (p *KeysPool) Initialize(head *types.Header) {
	p.head = head
	p.self = p.secStore.GetAddress()

	_ = p.bus.Subscribe(events.AddBlockEventID,
		func(e eventbus.Event) {
			newBlockEvent := e.(*events.NewBlockEvent)
			p.head = newBlockEvent.Block.Header
		})
	p.pushTracker.Run()
}

func (p *KeysPool) Add(hash common.Hash128, entry interface{}, highPriority bool) {
	//ignore it, entries are adding via AddPrivateKeysPackage
}

func (p *KeysPool) PushTracker() pushpull.PendingPushTracker {
	return p.pushTracker
}

func (p *KeysPool) Has(hash common.Hash128) bool {
	p.privateKeysMutex.RLock()
	_, ok := p.flipKeyPackagesByHash[hash]
	p.privateKeysMutex.RUnlock()
	return ok
}

func (p *KeysPool) Get(hash common.Hash128) (entry interface{}, highPriority bool, present bool) {
	p.privateKeysMutex.RLock()
	value, ok := p.flipKeyPackagesByHash[hash]
	p.privateKeysMutex.RUnlock()
	return value, false, ok
}

func (p *KeysPool) MaxParallelPulls() uint32 {
	return 1
}

func (p *KeysPool) SupportPendingRequests() bool {
	return true
}

func (p *KeysPool) AddPublicFlipKey(key *types.PublicFlipKey, own bool) error {

	appState, err := p.appState.Readonly(p.head.Height())
	if err != nil {
		return err
	}

	if err := p.putPublicFlipKey(key, appState, own); err != nil {
		return err
	}

	return nil
}

func (p *KeysPool) putPublicFlipKey(key *types.PublicFlipKey, appState *appstate.AppState, own bool) error {
	p.publicKeyMutex.Lock()
	defer p.publicKeyMutex.Unlock()

	sender, _ := types.SenderFlipKey(key)

	if old, ok := p.flipKeys[sender]; ok && old.Epoch >= key.Epoch {
		return errors.New("sender has already published his key")
	}

	if err := validateFlipKey(appState, key); err != nil {
		log.Trace("PublicFlipKey is not valid", "sender", sender.Hex(), "err", err)
		return err
	}

	p.flipKeys[sender] = key

	p.appState.EvidenceMap.NewFlipsKey(sender)

	p.bus.Publish(&events.NewFlipKeyEvent{
		Key: key,
		Own: own,
	})
	return nil
}

func (p *KeysPool) AddPrivateKeysPackage(keysPackage *types.PrivateFlipKeysPackage, own bool) error {

	sender, _ := types.SenderFlipKeysPackage(keysPackage)

	appState, err := p.appState.Readonly(p.head.Height())
	if err != nil {
		return err
	}

	err = p.putPrivateFlipKeysPackage(keysPackage, appState, own)

	if err != nil {
		log.Trace("Unable to add private keys package", "err", err, "sender", sender.Hex())
		return err
	}

	return err
}

func (p *KeysPool) putPrivateFlipKeysPackage(keysPackage *types.PrivateFlipKeysPackage, appState *appstate.AppState, own bool) error {
	sender, _ := types.SenderFlipKeysPackage(keysPackage)

	p.privateKeysMutex.Lock()
	defer p.privateKeysMutex.Unlock()

	if old, ok := p.flipKeyPackages[sender]; ok && old.Epoch >= keysPackage.Epoch {
		return errors.New("sender has already published his keys package")
	}

	if err := validateFlipKeysPackage(appState, keysPackage); err != nil {
		log.Trace("PrivateFLipKeysPackage is not valid", "sender", sender.Hex(), "err", err)
		return err
	}

	p.flipKeyPackages[sender] = keysPackage
	shortHash := keysPackage.Hash128()
	p.flipKeyPackagesByHash[shortHash] = keysPackage

	p.pushTracker.RemovePull(shortHash)

	p.bus.Publish(&events.NewFlipKeysPackageEvent{
		Key: keysPackage,
		Own: own,
	})
	return nil
}

func (p *KeysPool) GetFlipKeys() []*types.PublicFlipKey {
	p.publicKeyMutex.RLock()
	defer p.publicKeyMutex.RUnlock()

	var list []*types.PublicFlipKey

	for _, tx := range p.flipKeys {
		list = append(list, tx)
	}
	return list
}

func (p *KeysPool) GetFlipPackagesHashes() []common.Hash128 {
	p.privateKeysMutex.RLock()
	defer p.privateKeysMutex.RUnlock()

	var list []common.Hash128

	for k := range p.flipKeyPackagesByHash {
		list = append(list, k)
	}
	return list
}

func (p *KeysPool) GetPublicFlipKey(address common.Address) *ecies.PrivateKey {
	return p.getPublicFlipKey(address)
}

func (p *KeysPool) getPublicFlipKey(address common.Address) *ecies.PrivateKey {
	p.publicKeyMutex.RLock()
	defer p.publicKeyMutex.RUnlock()
	key, ok := p.flipKeys[address]
	if !ok {
		return nil
	}

	ecdsaKey, _ := crypto.ToECDSA(key.Key)
	return ecies.ImportECDSA(ecdsaKey)
}

func (p *KeysPool) GetPrivateFlipKey(address common.Address) *ecies.PrivateKey {
	p.privateKeysMutex.Lock()
	defer p.privateKeysMutex.Unlock()

	if data, ok := p.encryptedPrivateKeysCache[address]; ok {
		return data
	}

	publicFlipKey := p.getPublicFlipKey(address)
	if publicFlipKey == nil {
		log.Warn("GetPrivateFlipKey: public flip key is missing", "address", address.Hex())
		return nil
	}

	keysPackage, ok := p.flipKeyPackages[address]
	if !ok {
		log.Warn("GetPrivateFlipKey: package is missing", "address", address.Hex())
		return nil
	}

	idx, ok := p.privateKeyIndexes[address]
	if !ok {
		log.Warn("GetPrivateFlipKey: indexes are missing", "address", address.Hex())
		return nil
	}

	encryptedFlipKey, err := getEncryptedKeyFromPackage(publicFlipKey, keysPackage.Data, idx)
	if err != nil {
		log.Warn("GetPrivateFlipKey: Cannot get key from package", "err", err, "len", len(keysPackage.Data), "address", address.Hex())
		return nil
	}

	rawKey, err := p.secStore.DecryptMessage(encryptedFlipKey)
	if err != nil {
		log.Warn("GetPrivateFlipKey: Cannot decrypt key from package", "err", err, "address", address.Hex())
		return nil
	}

	ecdsaKey, err := crypto.ToECDSA(rawKey)
	if err != nil {
		log.Warn("GetPrivateFlipKey: Cannot convert decrypted key to ECDSA", "err", err, "address", address.Hex())
		return nil
	}

	result := ecies.ImportECDSA(ecdsaKey)
	p.encryptedPrivateKeysCache[address] = result
	return result
}

func (p *KeysPool) Clear() {
	p.privateKeysMutex.Lock()
	p.publicKeyMutex.Lock()

	defer p.privateKeysMutex.Unlock()
	defer p.publicKeyMutex.Unlock()

	p.cancelLoadingCtx()
	p.privateKeyIndexes = nil
	p.flipKeys = make(map[common.Address]*types.PublicFlipKey)
	p.flipKeyPackages = make(map[common.Address]*types.PrivateFlipKeysPackage)
	p.flipKeyPackagesByHash = make(map[common.Hash128]*types.PrivateFlipKeysPackage)
	p.encryptedPrivateKeysCache = make(map[common.Address]*ecies.PrivateKey)
	p.packagesLoadingCtx, p.cancelLoadingCtx = context.WithCancel(context.Background())
}

func (p *KeysPool) InitializePrivateKeyIndexes(indexes map[common.Address]int) {
	p.privateKeysMutex.Lock()
	p.privateKeyIndexes = indexes
	p.privateKeysMutex.Unlock()
}

func (p *KeysPool) AddPublicFlipKeys(batch []*types.PublicFlipKey) {
	appState, err := p.appState.Readonly(p.head.Height())

	if err != nil {
		p.log.Warn("keyspool.AddPublicFlipKeys: failed to create readonly appState", "err", err)
		return
	}

	for _, k := range batch {
		p.putPublicFlipKey(k, appState, false)
	}
}

func (p *KeysPool) AddPrivateFlipKeysPackages(batch []*types.PrivateFlipKeysPackage) {
	appState, err := p.appState.Readonly(p.head.Height())

	if err != nil {
		p.log.Warn("keyspool.AddPrivateFlipKeysPackages: failed to create readonly appState", "err", err)
		return
	}

	for _, k := range batch {
		p.putPrivateFlipKeysPackage(k, appState, false)
	}
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

type keysArray struct {
	Pairs [][]byte
}

func (k *keysArray) ToBytes() ([]byte, error) {
	protoArray := new(models.ProtoFlipPrivateKeys)
	protoArray.Keys = append(k.Pairs[:0:0], k.Pairs...)
	return proto.Marshal(protoArray)
}

func (k *keysArray) FromBytes(data []byte) error {
	protoArray := new(models.ProtoFlipPrivateKeys)
	if err := proto.Unmarshal(data, protoArray); err != nil {
		return err
	}
	k.Pairs = append(protoArray.Keys[:0:0], protoArray.Keys...)
	return nil
}

func EncryptPrivateKeysPackage(publicFlipKey *ecies.PrivateKey, privateFlipKey *ecies.PrivateKey, pubKeys [][]byte) []byte {
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

	arr := &keysArray{encryptedKeyPairs}

	arrayToEncrypt, _ := arr.ToBytes()

	encryptedArray, _ := ecies.Encrypt(rand.Reader, &publicFlipKey.PublicKey, arrayToEncrypt, nil, nil)

	return encryptedArray
}

func getEncryptedKeyFromPackage(publicFlipKey *ecies.PrivateKey, data []byte, index int) ([]byte, error) {
	decryptedPackage, err := publicFlipKey.Decrypt(data, nil, nil)
	if err != nil {
		return nil, err
	}
	keysArray := new(keysArray)
	if err := keysArray.FromBytes(decryptedPackage); err != nil {
		return nil, err
	}

	if index > len(keysArray.Pairs)-1 {
		return nil, errors.New(fmt.Sprintf("ecnrypted private keys package length is invalid, current: %v, need index: %v", len(keysArray.Pairs), index))
	}

	return keysArray.Pairs[index], nil
}
