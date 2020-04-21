package flip

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/secstore"
	"github.com/ipfs/go-cid"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
	"sync"
	"time"
)

var (
	DuplicateFlipError = errors.New("duplicate flip")
)

type Flipper struct {
	epochDb          *database.EpochDb
	db               dbm.DB
	log              log.Logger
	keyspool         *mempool.KeysPool
	ipfsProxy        ipfs.Proxy
	hasFlips         bool
	mutex            sync.Mutex
	secStore         *secstore.SecStore
	flips            map[common.Hash]*IpfsFlip
	flipReadiness    map[common.Hash]bool
	appState         *appstate.AppState
	txpool           *mempool.TxPool
	loadingCtx       context.Context
	cancelLoadingCtx context.CancelFunc
	bus              eventbus.Bus
	flipsQueue       chan *types.Flip
	flipsCache       *cache.Cache
	flipPublicKey    *ecies.PrivateKey
	flipPrivateKey   *ecies.PrivateKey
}
type IpfsFlip struct {
	PubKey      []byte
	PublicPart  []byte
	PrivatePart []byte
}

type IpfsFlipOld struct {
	Data   []byte
	PubKey []byte
}

func NewFlipper(db dbm.DB, ipfsProxy ipfs.Proxy, keyspool *mempool.KeysPool, txpool *mempool.TxPool, secStore *secstore.SecStore, appState *appstate.AppState, bus eventbus.Bus) *Flipper {
	ctx, cancel := context.WithCancel(context.Background())
	fp := &Flipper{
		db:               db,
		log:              log.New(),
		ipfsProxy:        ipfsProxy,
		keyspool:         keyspool,
		txpool:           txpool,
		secStore:         secStore,
		flips:            make(map[common.Hash]*IpfsFlip),
		flipReadiness:    make(map[common.Hash]bool),
		appState:         appState,
		loadingCtx:       ctx,
		cancelLoadingCtx: cancel,
		bus:              bus,
		flipsQueue:       make(chan *types.Flip, 1000),
		flipsCache:       cache.New(time.Minute, time.Minute*2),
	}
	go fp.writeLoop()
	return fp
}

func (fp *Flipper) Initialize() {
	fp.epochDb = database.NewEpochDb(fp.db, fp.appState.State.Epoch())
}

func (fp *Flipper) writeLoop() {
	for {
		select {
		case flip := <-fp.flipsQueue:
			if err := fp.addNewFlip(flip, false); err != nil && err != DuplicateFlipError {
				fp.log.Error("invalid flip", "err", err)
			}
		}
	}
}

func (fp *Flipper) addNewFlip(flip *types.Flip, local bool) error {
	fp.mutex.Lock()
	defer fp.mutex.Unlock()

	pubKey, err := types.SenderPubKey(flip.Tx)
	if err != nil {
		return errors.Errorf("flip tx has invalid pubkey, tx: %v", flip.Tx.Hash())
	}
	ipf := IpfsFlip{
		PublicPart:  flip.PublicPart,
		PrivatePart: flip.PrivatePart,
		PubKey:      pubKey,
	}

	data, _ := rlp.EncodeToBytes(ipf)

	if len(data) > common.MaxFlipSize {
		return errors.Errorf("flip is too big, max expected size %v, actual %v", common.MaxFlipSize, len(data))
	}

	c, err := fp.ipfsProxy.Cid(data)

	if err != nil {
		return err
	}

	if fp.epochDb.HasFlipCid(c.Bytes()) {
		return DuplicateFlipError
	}

	attachment := attachments.ParseFlipSubmitAttachment(flip.Tx)

	if attachment == nil {
		return errors.New("flip tx payload is invalid")
	}

	if bytes.Compare(c.Bytes(), attachment.Cid) != 0 {
		return errors.Errorf("tx cid and flip cid mismatch, tx: %v", flip.Tx.Hash())
	}

	if err := fp.txpool.Validate(flip.Tx); err != nil && err != mempool.DuplicateTxError {
		log.Warn("Flip Tx is not valid", "hash", flip.Tx.Hash().Hex(), "err", err)
		return err
	}

	_, err = fp.ipfsProxy.Add(data, fp.ipfsProxy.ShouldPin(ipfs.Flip) || local)

	if err != nil {
		return err
	}

	fp.flipsCache.Add(string(c.Bytes()), flip, cache.DefaultExpiration)

	fp.bus.Publish(&events.NewFlipEvent{Flip: flip})

	fp.epochDb.WriteFlipCid(c.Bytes())

	if local {
		log.Info("Sending new flip tx", "hash", flip.Tx.Hash().Hex(), "nonce", flip.Tx.AccountNonce, "epoch", flip.Tx.Epoch)
	}

	if err := fp.txpool.Add(flip.Tx); err != nil && err != mempool.DuplicateTxError {
		return err
	}
	return err
}

func (fp *Flipper) AddNewFlip(flip *types.Flip, local bool) error {
	if local {
		return fp.addNewFlip(flip, local)
	}
	select {
	case fp.flipsQueue <- flip:
	default:
	}
	return nil
}

func (fp *Flipper) PrepareFlip(flipPublicPart []byte, flipPrivatePart []byte) (cid.Cid, []byte, []byte, error) {

	publicEncryptionKey, privateEncryptionKey := fp.GetFlipPublicEncryptionKey(), fp.GetFlipPrivateEncryptionKey()

	encryptedPublic, err := ecies.Encrypt(rand.Reader, &publicEncryptionKey.PublicKey, flipPublicPart, nil, nil)

	if err != nil {
		return cid.Cid{}, nil, nil, err
	}

	var encryptedPrivate []byte
	if len(flipPrivatePart) > 0 {
		encryptedPrivate, err = ecies.Encrypt(rand.Reader, &privateEncryptionKey.PublicKey, flipPrivatePart, nil, nil)

		if err != nil {
			return cid.Cid{}, nil, nil, err
		}
	}

	ipf := IpfsFlip{
		PublicPart:  encryptedPublic,
		PrivatePart: encryptedPrivate,
		PubKey:      fp.secStore.GetPubKey(),
	}

	ipfsData, _ := rlp.EncodeToBytes(ipf)

	c, err := fp.ipfsProxy.Cid(ipfsData)

	if err != nil {
		return cid.Cid{}, nil, nil, err
	}

	return c, encryptedPublic, encryptedPrivate, nil
}

func (fp *Flipper) GetFlip(key []byte) (publicPart []byte, privatePart []byte, err error) {

	fp.mutex.Lock()
	ipfsFlip := fp.flips[common.Hash(rlp.Hash(key))]
	fp.mutex.Unlock()

	if ipfsFlip == nil {
		return nil, nil, errors.New("flip is missing")
	}

	var publicEncryptionKey *ecies.PrivateKey
	var privateEncryptionKey *ecies.PrivateKey
	if bytes.Compare(ipfsFlip.PubKey, fp.secStore.GetPubKey()) == 0 {
		publicEncryptionKey, privateEncryptionKey = fp.GetFlipPublicEncryptionKey(), fp.GetFlipPrivateEncryptionKey()
	} else {
		addr, _ := crypto.PubKeyBytesToAddress(ipfsFlip.PubKey)
		publicEncryptionKey, privateEncryptionKey = fp.keyspool.GetPublicFlipKey(addr), fp.keyspool.GetPrivateFlipKey(addr)
		if publicEncryptionKey == nil {
			return nil, nil, errors.New("flip public key is missing")
		}
	}

	decryptedPublicPart, err := publicEncryptionKey.Decrypt(ipfsFlip.PublicPart, nil, nil)

	if err != nil {
		return nil, nil, errors.Wrap(err, "cannot decrypt flip public part")
	}

	var decryptedPrivatePart []byte
	if len(ipfsFlip.PrivatePart) > 0 {
		if privateEncryptionKey == nil {
			return nil, nil, errors.New("flip private key is missing")
		}

		decryptedPrivatePart, err = privateEncryptionKey.Decrypt(ipfsFlip.PrivatePart, nil, nil)

		if err != nil {
			return nil, nil, errors.Wrap(err, "cannot decrypt flip private part")
		}
	}

	return decryptedPublicPart, decryptedPrivatePart, nil
}

func (fp *Flipper) GetFlipPublicEncryptionKey() *ecies.PrivateKey {
	fp.mutex.Lock()
	defer fp.mutex.Unlock()

	if fp.flipPublicKey != nil {
		return fp.flipPublicKey
	}

	fp.flipPublicKey = fp.generateFlipEncryptionKey(true)
	return fp.flipPublicKey
}

func (fp *Flipper) GetFlipPrivateEncryptionKey() *ecies.PrivateKey {
	fp.mutex.Lock()
	defer fp.mutex.Unlock()

	if fp.flipPrivateKey != nil {
		return fp.flipPrivateKey
	}

	fp.flipPrivateKey = fp.generateFlipEncryptionKey(false)
	return fp.flipPrivateKey
}

func (fp *Flipper) generateFlipEncryptionKey(public bool) *ecies.PrivateKey {
	var seed []byte
	if public {
		seed = []byte(fmt.Sprintf("flip-key-for-epoch-%v", fp.appState.State.Epoch()))
	} else {
		seed = []byte(fmt.Sprintf("flip-private-key-for-epoch-%v", fp.appState.State.Epoch()))
	}

	hash := common.Hash(rlp.Hash(seed))

	sig := fp.secStore.Sign(hash.Bytes())

	flipKey, _ := crypto.GenerateKeyFromSeed(bytes.NewReader(sig))

	return ecies.ImportECDSA(flipKey)
}

func (fp *Flipper) Load(cids [][]byte) {
	ctx := fp.loadingCtx

	for len(cids) > 0 {

		select {
		case <-ctx.Done():
			return
		default:
		}

		key := cids[0]
		cids = cids[1:]

		cid, _ := cid.Cast(key)

		data, err := fp.ipfsProxy.Get(key, ipfs.Flip)

		if err != nil {
			fp.log.Warn("Can't get flip by cid", "cid", cid.String(), "err", err)
			cids = append(cids, key)
			continue
		}

		ipfsFlip := new(IpfsFlip)
		if err := rlp.Decode(bytes.NewReader(data), ipfsFlip); err != nil {
			oldIpfsFlip := new(IpfsFlipOld)
			if err2 := rlp.Decode(bytes.NewReader(data), oldIpfsFlip); err2 != nil {
				fp.log.Warn("Can't decode flip", "cid", cid.String(), "err", err)
				continue
			} else {
				ipfsFlip.PublicPart = oldIpfsFlip.Data
				ipfsFlip.PubKey = oldIpfsFlip.PubKey
			}
		}
		fp.mutex.Lock()
		fp.flips[common.Hash(rlp.Hash(key))] = ipfsFlip
		fp.mutex.Unlock()
	}
	fp.log.Info("All flips were loaded")
	fp.hasFlips = true
}

func (fp *Flipper) Clear() {
	fp.mutex.Lock()
	defer fp.mutex.Unlock()

	fp.cancelLoadingCtx()
	fp.hasFlips = false
	fp.flips = make(map[common.Hash]*IpfsFlip)
	fp.flipReadiness = make(map[common.Hash]bool)
	fp.Initialize()
	fp.flipPrivateKey = nil
	fp.flipPublicKey = nil
	fp.loadingCtx, fp.cancelLoadingCtx = context.WithCancel(context.Background())
}

func (fp *Flipper) HasFlips() bool {
	return fp.hasFlips
}

func (fp *Flipper) IsFlipReady(key []byte) bool {
	hash := common.Hash(rlp.Hash(key))

	fp.mutex.Lock()
	flip := fp.flips[hash]
	isReady := fp.flipReadiness[hash]
	fp.mutex.Unlock()

	if flip == nil {
		return false
	}

	if !isReady {
		if _, _, err := fp.GetFlip(key); err == nil {
			fp.mutex.Lock()
			isReady = true
			fp.flipReadiness[hash] = true
			fp.mutex.Unlock()
		} else {
			c, _ := cid.Cast(key)
			log.Warn("flip is not ready", "err", err, "cid", c.String())
		}
	}

	return isReady
}

func (fp *Flipper) UnpinFlip(flipCid []byte) {
	fp.ipfsProxy.Unpin(flipCid)
}

func (fp *Flipper) GetRawFlip(flipCid []byte) (*IpfsFlip, error) {
	data, err := fp.ipfsProxy.Get(flipCid, ipfs.Flip)
	if err != nil {
		return nil, err
	}
	ipfsFlip := new(IpfsFlip)
	if err := rlp.Decode(bytes.NewReader(data), ipfsFlip); err != nil {
		oldIpfsFlip := new(IpfsFlipOld)
		if err2 := rlp.Decode(bytes.NewReader(data), oldIpfsFlip); err2 != nil {
			return nil, err
		} else {
			ipfsFlip.PublicPart = oldIpfsFlip.Data
			ipfsFlip.PubKey = oldIpfsFlip.PubKey
		}
	}
	return ipfsFlip, nil
}

func (fp *Flipper) Has(c []byte) bool {
	return fp.epochDb.HasFlipCid(c)
}

func (fp *Flipper) ReadFlip(cid []byte) (*types.Flip, error) {
	if flip, ok := fp.flipsCache.Get(string(cid)); ok {
		return flip.(*types.Flip), nil
	}
	return nil, errors.New("flip is not found")
}
