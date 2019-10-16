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
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
	"sync"
)

const (
	MaxFlipSize = 1024 * 600
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
	flipKey          *ecies.PrivateKey
	flipsQueue       chan *types.Flip
}
type IpfsFlip struct {
	Data   []byte
	PubKey []byte
}
type IpfsFlipOld struct {
	Data   []byte
	PubKey []byte
	Pair   uint8
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
		flipsQueue:       make(chan *types.Flip, 10000),
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
		Data:   flip.Data,
		PubKey: pubKey,
	}

	data, _ := rlp.EncodeToBytes(ipf)

	if len(data) > MaxFlipSize {
		return errors.Errorf("flip is too big, max expected size %v, actual %v", MaxFlipSize, len(data))
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

	key, err := fp.ipfsProxy.Add(data)

	if err != nil {
		return err
	}

	fp.bus.Publish(&events.NewFlipEvent{Flip: flip})

	fp.epochDb.WriteFlipCid(c.Bytes())

	if local {
		if err := fp.ipfsProxy.Pin(key.Bytes()); err != nil {
			return err
		}
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
	fp.flipsQueue <- flip
	return nil
}

func (fp *Flipper) PrepareFlip(hex []byte) (cid.Cid, []byte, error) {

	encryptionKey := fp.GetFlipEncryptionKey()

	encrypted, err := ecies.Encrypt(rand.Reader, &encryptionKey.PublicKey, hex, nil, nil)

	if err != nil {
		return cid.Cid{}, nil, err
	}

	ipf := IpfsFlip{
		Data:   encrypted,
		PubKey: fp.secStore.GetPubKey(),
	}

	ipfsData, _ := rlp.EncodeToBytes(ipf)

	c, err := fp.ipfsProxy.Cid(ipfsData)

	if err != nil {
		return cid.Cid{}, nil, err
	}

	return c, encrypted, nil
}

func (fp *Flipper) GetFlip(key []byte) ([]byte, error) {

	fp.mutex.Lock()
	ipfsFlip := fp.flips[common.Hash(rlp.Hash(key))]
	fp.mutex.Unlock()

	if ipfsFlip == nil {
		return nil, errors.New("flip is missing")
	}

	// if flip is mine
	var encryptionKey *ecies.PrivateKey
	if bytes.Compare(ipfsFlip.PubKey, fp.secStore.GetPubKey()) == 0 {
		encryptionKey = fp.GetFlipEncryptionKey()
		if encryptionKey == nil {
			return nil, errors.New("flip key is missing")
		}
	} else {
		addr, _ := crypto.PubKeyBytesToAddress(ipfsFlip.PubKey)
		flipKey := fp.keyspool.GetFlipKey(addr)
		if flipKey == nil {
			return nil, errors.New("flip key is missing")
		}
		ecdsaKey, _ := crypto.ToECDSA(flipKey.Key)
		encryptionKey = ecies.ImportECDSA(ecdsaKey)
	}

	decryptedFlip, err := encryptionKey.Decrypt(ipfsFlip.Data, nil, nil)

	if err != nil {
		return nil, err
	}

	return decryptedFlip, nil
}

func (fp *Flipper) GetFlipEncryptionKey() *ecies.PrivateKey {
	fp.mutex.Lock()
	defer fp.mutex.Unlock()

	if fp.flipKey != nil {
		return fp.flipKey
	}

	seed := []byte(fmt.Sprintf("flip-key-for-epoch-%v", fp.appState.State.Epoch()))

	hash := common.Hash(rlp.Hash(seed))

	sig := fp.secStore.Sign(hash.Bytes())

	flipKey, _ := crypto.GenerateKeyFromSeed(bytes.NewReader(sig))

	fp.flipKey = ecies.ImportECDSA(flipKey)

	return fp.flipKey
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

		data, err := fp.ipfsProxy.Get(key)

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
				ipfsFlip.Data = oldIpfsFlip.Data
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

func (fp *Flipper) Reset() {
	fp.mutex.Lock()
	defer fp.mutex.Unlock()

	fp.cancelLoadingCtx()
	fp.hasFlips = false
	fp.flips = make(map[common.Hash]*IpfsFlip)
	fp.flipReadiness = make(map[common.Hash]bool)
	fp.keyspool.Clear()
	fp.Initialize()
	fp.flipKey = nil
	fp.loadingCtx, fp.cancelLoadingCtx = context.WithCancel(context.Background())
}

func (fp *Flipper) HasFlips() bool {
	return fp.hasFlips
}

func (fp *Flipper) IsFlipReady(cid []byte) bool {
	hash := common.Hash(rlp.Hash(cid))

	fp.mutex.Lock()
	flip := fp.flips[hash]
	isReady := fp.flipReadiness[hash]
	fp.mutex.Unlock()

	if flip == nil {
		return false
	}

	if !isReady {
		if _, err := fp.GetFlip(cid); err == nil {
			fp.mutex.Lock()
			isReady = true
			fp.flipReadiness[hash] = true
			fp.mutex.Unlock()
		}
	}

	return isReady
}

func (fp *Flipper) UnpinFlip(flipCid []byte) {
	fp.ipfsProxy.Unpin(flipCid)
}

func (fp *Flipper) GetRawFlip(flipCid []byte) (*IpfsFlip, error) {
	data, err := fp.ipfsProxy.Get(flipCid)
	if err != nil {
		return nil, err
	}
	ipfsFlip := new(IpfsFlip)
	if err := rlp.Decode(bytes.NewReader(data), ipfsFlip); err != nil {
		oldIpfsFlip := new(IpfsFlipOld)
		if err2 := rlp.Decode(bytes.NewReader(data), oldIpfsFlip); err2 != nil {
			return nil, err
		} else {
			ipfsFlip.Data = oldIpfsFlip.Data
			ipfsFlip.PubKey = oldIpfsFlip.PubKey
		}
	}
	return ipfsFlip, nil
}
