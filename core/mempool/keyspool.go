package mempool

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/common/pushpull"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/log"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/idena-network/idena-go/secstore"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"sync"
	"time"
)

const (
	maxPrivateKeysPackageDataSize = 1024 * 100
	publicFlipKeySize             = 32
	maxFlipKeySyncCounts          = 10
)

var (
	maxFloat *big.Float
)

var (
	KeyIsAlreadyPublished = errors.New("sender has already published his keys")
	KeySkipped            = errors.New("key has been skipped")
)

type FlipKeysPool interface {
	AddPrivateKeysPackage(keysPackage *types.PrivateFlipKeysPackage, own bool) error
	AddPublicFlipKey(key *types.PublicFlipKey, own bool) error
	GetFlipPackagesHashesForSync(shardId common.ShardId, noFilter bool) []common.Hash128
	GetFlipKeysForSync(shardId common.ShardId, noFilter bool) []*types.PublicFlipKey
}

func init() {
	maxFloat = new(big.Float).SetInt(new(big.Int).SetBytes(common.MaxHash[:]))
}

type flipKeyPackageWrapper struct {
	keyPackage *types.PrivateFlipKeysPackage
	shardId    common.ShardId
}

type KeysPool struct {
	db       dbm.DB
	epochDb  *database.EpochDb
	appState *appstate.AppState
	bus      eventbus.Bus
	head     *types.Header
	log      log.Logger

	flipKeys               map[common.Address]*types.PublicFlipKey
	publicKeyMutex         sync.RWMutex
	flipKeysSyncCounts     map[common.Address]int

	flipKeyPackages               map[common.Address]*types.PrivateFlipKeysPackage
	privateKeysMutex              sync.RWMutex
	flipKeyPackagesSyncCounts     map[common.Hash128]int

	flipKeyPackagesByHash map[common.Hash128]*flipKeyPackageWrapper
	privateKeysArrayCache map[common.Address]*keysArray
	secStore              *secstore.SecStore
	packagesLoadingCtx    context.Context
	cancelLoadingCtx      context.CancelFunc
	self                  common.Address
	pushTracker           pushpull.PendingPushTracker
	stopSync              bool
}

func NewKeysPool(db dbm.DB, appState *appstate.AppState, bus eventbus.Bus, secStore *secstore.SecStore) *KeysPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &KeysPool{
		db:                        db,
		appState:                  appState,
		bus:                       bus,
		log:                       log.New(),
		flipKeys:                  make(map[common.Address]*types.PublicFlipKey),
		flipKeysSyncCounts:        make(map[common.Address]int),
		flipKeyPackages:           make(map[common.Address]*types.PrivateFlipKeysPackage),
		flipKeyPackagesByHash:     make(map[common.Hash128]*flipKeyPackageWrapper),
		flipKeyPackagesSyncCounts: make(map[common.Hash128]int),
		privateKeysArrayCache:     make(map[common.Address]*keysArray),
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
	p.epochDb = database.NewEpochDb(p.db, p.appState.State.Epoch())
	_ = p.bus.Subscribe(events.AddBlockEventID,
		func(e eventbus.Event) {
			newBlockEvent := e.(*events.NewBlockEvent)
			p.head = newBlockEvent.Block.Header
		})
	p.pushTracker.Run()

	appState, _ := p.appState.Readonly(p.head.Height())
	for _, k := range p.epochDb.ReadPublicFlipKeys() {
		_ = p.putPublicFlipKey(k, appState, false)
	}

	for _, k := range p.epochDb.ReadPrivateFlipKeys() {
		_ = p.putPrivateFlipKeysPackage(k, appState, false)
	}
}

func (p *KeysPool) Add(hash common.Hash128, entry interface{}, id common.ShardId, highPriority bool) {
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

func (p *KeysPool) Get(hash common.Hash128) (entry interface{}, id common.ShardId, highPriority bool, present bool) {
	p.privateKeysMutex.RLock()
	value, ok := p.flipKeyPackagesByHash[hash]
	p.privateKeysMutex.RUnlock()
	if ok {
		return value.keyPackage, value.shardId, false, true
	}
	return nil, common.MultiShard, false, ok
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

	sender, _ := types.SenderFlipKey(key)

	if old, ok := p.flipKeys[sender]; ok && old.Epoch >= key.Epoch {
		p.publicKeyMutex.Unlock()
		return KeyIsAlreadyPublished
	}

	if err := validateFlipKey(appState, key); err != nil {
		p.publicKeyMutex.Unlock()
		log.Trace("PublicFlipKey is not valid", "sender", sender.Hex(), "err", err)
		return err
	}

	key.SetShardId(appState.State.ShardId(sender))

	p.flipKeys[sender] = key

	p.appState.EvidenceMap.NewFlipsKey(sender)

	p.epochDb.WritePublicFlipKey(key)

	p.publicKeyMutex.Unlock()

	p.bus.Publish(&events.NewFlipKeyEvent{
		Key:     key,
		Own:     own,
		ShardId: appState.State.ShardId(sender),
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

	if old, ok := p.flipKeyPackages[sender]; ok && old.Epoch >= keysPackage.Epoch {
		p.privateKeysMutex.Unlock()
		return KeyIsAlreadyPublished
	}

	if err := validateFlipKeysPackage(appState, keysPackage); err != nil {
		p.privateKeysMutex.Unlock()
		log.Trace("PrivateFLipKeysPackage is not valid", "sender", sender.Hex(), "err", err)
		return err
	}

	p.flipKeyPackages[sender] = keysPackage
	shortHash := keysPackage.Hash128()
	p.flipKeyPackagesByHash[shortHash] = &flipKeyPackageWrapper{keysPackage, appState.State.ShardId(sender)}
	p.pushTracker.RemovePull(shortHash)

	p.epochDb.WritePrivateFlipKey(keysPackage)

	p.privateKeysMutex.Unlock()

	p.appState.EvidenceMap.NewFlipKeyPackage(sender)

	p.bus.Publish(&events.NewFlipKeysPackageEvent{
		Key:     keysPackage,
		ShardId: appState.State.ShardId(sender),
		Own:     own,
	})
	return nil
}

func (p *KeysPool) GetFlipKeysForSync(shardId common.ShardId, noFilter bool) []*types.PublicFlipKey {
	p.publicKeyMutex.Lock()
	defer p.publicKeyMutex.Unlock()

	var list []*types.PublicFlipKey
	if noFilter {
		list = make([]*types.PublicFlipKey, 0, len(p.flipKeys))
	}
	if !p.stopSync {
		for h, key := range p.flipKeys {
			if key.LoadShardId() != shardId && shardId != common.MultiShard {
				continue
			}
			if noFilter || p.flipKeysSyncCounts[h] <= maxFlipKeySyncCounts {
				list = append(list, key)
				p.flipKeysSyncCounts[h]++
			}
		}
	}
	return list
}

func (p *KeysPool) GetFlipPackagesHashesForSync(shardId common.ShardId, noFilter bool) []common.Hash128 {
	p.privateKeysMutex.Lock()
	defer p.privateKeysMutex.Unlock()

	var list []common.Hash128
	if noFilter {
		list = make([]common.Hash128, 0, len(p.flipKeyPackagesByHash))
	}
	if !p.stopSync {
		for k, pkg := range p.flipKeyPackagesByHash {
			if pkg.shardId != shardId && shardId != common.MultiShard {
				continue
			}
			if noFilter || p.flipKeyPackagesSyncCounts[k] <= maxFlipKeySyncCounts {
				list = append(list, k)
				p.flipKeyPackagesSyncCounts[k]++
			}
		}
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

	ecdsaKey, err := crypto.ToECDSA(key.Key)
	if err != nil {
		p.log.Warn("Public flip key is not valid ECDSA key", "err", err, "key", hexutil.Encode(key.Key), "address", address.Hex())
		return nil
	}
	return ecies.ImportECDSA(ecdsaKey)
}

func (p *KeysPool) GetEncryptedPrivateFlipKey(indexInPackage int, address common.Address) []byte {
	log.Info("Getting private key by index", "idx", indexInPackage, "address", address.Hex())

	publicFlipKey := p.getPublicFlipKey(address)
	if publicFlipKey == nil {
		log.Warn("GetEncryptedPrivateFlipKey: public flip key is missing", "address", address.Hex())
		return nil
	}

	p.privateKeysMutex.Lock()
	defer p.privateKeysMutex.Unlock()

	if data, ok := p.privateKeysArrayCache[address]; ok {
		if indexInPackage > len(data.Pairs)-1 {
			return nil
		}
		return data.Pairs[indexInPackage]
	}

	keysPackage, ok := p.flipKeyPackages[address]
	if !ok {
		log.Warn("GetEncryptedPrivateFlipKey: package is missing", "address", address.Hex())
		return nil
	}

	decryptedPackage, err := publicFlipKey.Decrypt(keysPackage.Data, nil, nil)
	if err != nil {
		log.Warn("GetEncryptedPrivateFlipKey: cannot decrypt package", "address", address.Hex())
		return nil
	}
	keysArray := new(keysArray)
	if err := keysArray.FromBytes(decryptedPackage); err != nil {
		log.Warn("GetEncryptedPrivateFlipKey: proto deserialization failed", "address", address.Hex())
		return nil
	}
	p.privateKeysArrayCache[address] = keysArray

	if indexInPackage > len(keysArray.Pairs)-1 {
		return nil
	}
	return keysArray.Pairs[indexInPackage]
}

func (p *KeysPool) Clear() {
	p.privateKeysMutex.Lock()
	p.publicKeyMutex.Lock()

	defer p.privateKeysMutex.Unlock()
	defer p.publicKeyMutex.Unlock()

	p.cancelLoadingCtx()
	p.flipKeys = make(map[common.Address]*types.PublicFlipKey)
	p.flipKeyPackagesSyncCounts = map[common.Hash128]int{}
	p.flipKeyPackages = make(map[common.Address]*types.PrivateFlipKeysPackage)
	p.flipKeyPackagesByHash = make(map[common.Hash128]*flipKeyPackageWrapper)
	p.flipKeyPackagesSyncCounts = map[common.Hash128]int{}
	p.privateKeysArrayCache = make(map[common.Address]*keysArray)
	p.packagesLoadingCtx, p.cancelLoadingCtx = context.WithCancel(context.Background())
	p.epochDb = database.NewEpochDb(p.db, p.appState.State.Epoch())
	p.stopSync = false
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

func (p *KeysPool) StopSyncing() {
	p.stopSync = true
}

func validateFlipKey(appState *appstate.AppState, key *types.PublicFlipKey) error {
	if len(key.Key) != publicFlipKeySize {
		return errors.Errorf("invalid flip key length %d", len(key.Key))
	}
	sender, _ := types.SenderFlipKey(key)
	return validateKey(sender, key.Epoch, appState)
}

func validateFlipKeysPackage(appState *appstate.AppState, keysPackage *types.PrivateFlipKeysPackage) error {
	if len(keysPackage.Data) > maxPrivateKeysPackageDataSize {
		return errors.Errorf("too big flip keys package, size: %d", len(keysPackage.Data))
	}
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
