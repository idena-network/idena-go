package mempool

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/secstore"
	"github.com/ipfs/go-cid"
	util "github.com/ipfs/go-ipfs-util"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"sync"
	"time"
)

type KeysPackage struct {
	RawPackage *types.PrivateFlipKeysPackage
	Cid        cid.Cid
}

const (
	KeysPackageSaveThreshold = 0.66
)

var (
	maxFloat *big.Float
)

func init() {
	maxFloat = new(big.Float).SetInt(new(big.Int).SetBytes(common.MaxHash[:]))
}

type KeysPool struct {
	db                        dbm.DB
	appState                  *appstate.AppState
	flipKeys                  map[common.Address]*types.PublicFlipKey
	mutex                     sync.Mutex
	bus                       eventbus.Bus
	head                      *types.Header
	log                       log.Logger
	flipKeyPackages           map[common.Address]*KeysPackage
	privateKeyIndexes         map[common.Address]int // shows which key (by index) use from author's package
	encryptedPrivateKeysCache map[common.Address]*ecies.PrivateKey
	ipfsProxy                 ipfs.Proxy
	secStore                  *secstore.SecStore
	packagesLoadingCtx        context.Context
	cancelLoadingCtx          context.CancelFunc
	self                      common.Address
	epochDb                   *database.EpochDb
}

func NewKeysPool(db dbm.DB, appState *appstate.AppState, bus eventbus.Bus, secStore *secstore.SecStore, ipfsProxy ipfs.Proxy) *KeysPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &KeysPool{
		db:                        db,
		appState:                  appState,
		bus:                       bus,
		log:                       log.New(),
		flipKeys:                  make(map[common.Address]*types.PublicFlipKey),
		flipKeyPackages:           make(map[common.Address]*KeysPackage),
		encryptedPrivateKeysCache: make(map[common.Address]*ecies.PrivateKey),
		secStore:                  secStore,
		ipfsProxy:                 ipfsProxy,
		packagesLoadingCtx:        ctx,
		cancelLoadingCtx:          cancel,
	}
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
}

func (p *KeysPool) AddPublicFlipKey(key *types.PublicFlipKey, own bool) error {

	err := func() error {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		hash := key.Hash()

		sender, _ := types.SenderFlipKey(key)

		if _, ok := p.flipKeys[sender]; ok {
			return errors.New("sender has already published his key")
		}

		appState := p.appState.Readonly(p.head.Height())

		if err := validateFlipKey(appState, key); err != nil {
			log.Warn("PublicFlipKey is not valid", "hash", hash.Hex(), "err", err)
			return err
		}

		p.flipKeys[sender] = key

		p.appState.EvidenceMap.NewFlipsKey(sender)

		return nil
	}()

	if err != nil {
		return err
	}

	p.bus.Publish(&events.NewFlipKeyEvent{
		Key: key,
		Own: own,
	})

	return nil
}

func (p *KeysPool) AddPrivateKeysPackage(keysPackage *types.PrivateFlipKeysPackage, own bool) error {

	hash := keysPackage.Hash()
	sender, _ := types.SenderFlipKeysPackage(keysPackage)

	err := func() error {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if _, ok := p.flipKeyPackages[sender]; ok {
			return errors.New("sender has already published his keys package")
		}

		appState := p.appState.Readonly(p.head.Height())

		if err := validateFlipKeysPackage(appState, keysPackage); err != nil {
			log.Warn("PrivateFLipKeysPackage is not valid", "hash", hash.Hex(), "err", err)
			return err
		}

		data, _ := rlp.EncodeToBytes(keysPackage)
		c, err := p.ipfsProxy.Cid(data)

		if err != nil {
			return err
		}

		p.flipKeyPackages[sender] = &KeysPackage{
			RawPackage: keysPackage,
			Cid:        c,
		}

		if own || checkThreshold(p.self, hash) {
			go func() {
				c, err = p.ipfsProxy.Add(data)
				if err != nil {
					log.Error("cannot save private keys package to IPFS", "err", err)
					return
				}
				p.epochDb.WriteKeysPackageCid(c.Bytes())
			}()
		}

		return nil
	}()

	if err != nil {
		return err
	}

	p.bus.Publish(&events.NewFlipKeysPackageEvent{
		Key: keysPackage,
		Own: own,
	})

	return nil
}

func (p *KeysPool) AddPrivateKeysPackageCid(packageCid *types.PrivateFlipKeysPackageCid) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	c, err := cid.Parse(packageCid.Cid)
	if err != nil {
		log.Warn("invalid cid during keys packages sync")
		return
	}

	if _, ok := p.flipKeyPackages[packageCid.Address]; !ok {
		p.flipKeyPackages[packageCid.Address] = &KeysPackage{
			Cid: c,
		}
	}
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

func (p *KeysPool) GetFlipPackagesCids() []*types.PrivateFlipKeysPackageCid {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	var list []*types.PrivateFlipKeysPackageCid

	for k, p := range p.flipKeyPackages {
		list = append(list, &types.PrivateFlipKeysPackageCid{
			Address: k,
			Cid:     p.Cid.Bytes(),
		})
	}
	return list
}

func (p *KeysPool) GetPublicFlipKey(address common.Address) *ecies.PrivateKey {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.getPublicFlipKey(address)
}

func (p *KeysPool) getPublicFlipKey(address common.Address) *ecies.PrivateKey {
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

	publicFlipKey := p.getPublicFlipKey(address)
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

	p.cancelLoadingCtx()
	p.privateKeyIndexes = nil
	p.flipKeys = make(map[common.Address]*types.PublicFlipKey)
	p.flipKeyPackages = make(map[common.Address]*KeysPackage)
	p.encryptedPrivateKeysCache = make(map[common.Address]*ecies.PrivateKey)
	p.packagesLoadingCtx, p.cancelLoadingCtx = context.WithCancel(context.Background())
	p.epochDb = database.NewEpochDb(p.db, p.appState.State.Epoch())
}

func (p *KeysPool) LoadNecessaryPackages(indexes map[common.Address]int) {
	ctx := p.packagesLoadingCtx
	p.privateKeyIndexes = indexes

	allPackagesLoaded := false
	for !allPackagesLoaded {
		allPackagesLoaded = true
		for author := range p.privateKeyIndexes {
			select {
			case <-ctx.Done():
				return
			default:
			}

			p.mutex.Lock()
			keyPackage, ok := p.flipKeyPackages[author]
			p.mutex.Unlock()

			if ok && keyPackage.RawPackage != nil {
				continue
			} else if ok {
				allPackagesLoaded = false
				data, err := p.ipfsProxy.Get(keyPackage.Cid.Bytes())
				if err != nil {
					p.log.Warn("Can't get flip package by cid", "cid", keyPackage.Cid.String(), "err", err)
					continue
				}
				flipKeysPackage := new(types.PrivateFlipKeysPackage)
				if err := rlp.Decode(bytes.NewReader(data), flipKeysPackage); err != nil {
					p.log.Warn("Can't decode keys package", "cid", keyPackage.Cid.String(), "err", err)
					continue
				}
				p.mutex.Lock()
				p.flipKeyPackages[author].RawPackage = flipKeysPackage
				p.mutex.Unlock()
			} else {
				time.Sleep(100 * time.Millisecond)
				allPackagesLoaded = false
			}
		}
	}
}

func (p *KeysPool) UnpinPackage(key []byte) {
	p.ipfsProxy.Unpin(key)
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

	arrayToEncrypt, _ := rlp.EncodeToBytes(&keysArray{encryptedKeyPairs})

	encryptedArray, _ := ecies.Encrypt(rand.Reader, &publicFlipKey.PublicKey, arrayToEncrypt, nil, nil)

	return encryptedArray
}

func getEncryptedKeyFromPackage(publicFlipKey *ecies.PrivateKey, data []byte, index int) ([]byte, error) {
	decryptedPackage, err := publicFlipKey.Decrypt(data, nil, nil)
	if err != nil {
		return nil, err
	}
	keysArray := new(keysArray)
	if err := rlp.DecodeBytes(decryptedPackage, keysArray); err != nil {
		return nil, err
	}

	if index > len(keysArray.Pairs)-1 {
		return nil, errors.New(fmt.Sprintf("ecnrypted private keys package length is invalid, current: %v, need index: %v", len(keysArray.Pairs), index))
	}

	return keysArray.Pairs[index], nil
}

func checkThreshold(address common.Address, hash common.Hash) bool {
	addrHash := rlp.Hash(address)
	result := util.XOR(addrHash[:], hash[:])

	a := new(big.Float).SetInt(new(big.Int).SetBytes(result[:]))
	q := new(big.Float).Quo(a, maxFloat).SetPrec(10)

	f, _ := q.Float64()
	return f >= KeysPackageSaveThreshold
}
