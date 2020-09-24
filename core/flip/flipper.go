package flip

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"github.com/golang/protobuf/proto"
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
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/idena-network/idena-go/secstore"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
	"sync"
)

var (
	DuplicateFlipError = errors.New("duplicate flip")
	FlipIsMissingError = errors.New("flip is missing")
)

type Flipper struct {
	epochDb          *database.EpochDb
	db               dbm.DB
	log              log.Logger
	keyspool         *mempool.KeysPool
	ipfsProxy        ipfs.Proxy
	mutex            sync.RWMutex
	secStore         *secstore.SecStore
	flips            map[common.Hash]*IpfsFlip
	flipReadiness    map[common.Hash]bool
	appState         *appstate.AppState
	txpool           *mempool.TxPool
	loadingCtx       context.Context
	cancelLoadingCtx context.CancelFunc
	bus              eventbus.Bus
	flipsQueue       chan *types.Flip
	flipPublicKey    *ecies.PrivateKey
	flipPrivateKey   *ecies.PrivateKey
}

type IpfsFlip struct {
	PubKey      []byte
	PublicPart  []byte
	PrivatePart []byte
}

func (f *IpfsFlip) ToBytes() ([]byte, error) {
	protoFlip := new(models.ProtoIpfsFlip)
	protoFlip.PubKey = f.PubKey
	protoFlip.PublicPart = f.PublicPart
	protoFlip.PrivatePart = f.PrivatePart
	return proto.Marshal(protoFlip)
}

func (f *IpfsFlip) FromBytes(data []byte) error {
	protoFlip := new(models.ProtoIpfsFlip)
	if err := proto.Unmarshal(data, protoFlip); err != nil {
		return err
	}
	f.PubKey = protoFlip.PubKey
	f.PublicPart = protoFlip.PublicPart
	f.PrivatePart = protoFlip.PrivatePart
	return nil
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
	ipf := &IpfsFlip{
		PublicPart:  flip.PublicPart,
		PrivatePart: flip.PrivatePart,
		PubKey:      pubKey,
	}

	data, _ := ipf.ToBytes()

	if len(data) == 0 {
		return errors.New("flip is empty")
	}

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

	ipf := &IpfsFlip{
		PublicPart:  encryptedPublic,
		PrivatePart: encryptedPrivate,
		PubKey:      fp.secStore.GetPubKey(),
	}

	ipfsData, _ := ipf.ToBytes()

	c, err := fp.ipfsProxy.Cid(ipfsData)

	if err != nil {
		return cid.Cid{}, nil, nil, err
	}

	return c, encryptedPublic, encryptedPrivate, nil
}

func (fp *Flipper) GetFlipFromMemory(key []byte) (publicPart []byte, privatePart []byte, err error) {
	fp.mutex.RLock()
	ipfsFlip := fp.flips[common.Hash(crypto.Hash(key))]
	fp.mutex.RUnlock()

	if ipfsFlip == nil {
		return nil, nil, FlipIsMissingError
	}

	return ipfsFlip.PublicPart, ipfsFlip.PrivatePart, nil
}

func (fp *Flipper) GetFlipPublicEncryptionKey() *ecies.PrivateKey {
	fp.mutex.RLock()
	defer fp.mutex.RUnlock()

	if fp.flipPublicKey != nil {
		return fp.flipPublicKey
	}

	fp.flipPublicKey = fp.generateFlipEncryptionKey(true)
	return fp.flipPublicKey
}

func (fp *Flipper) GetFlipPrivateEncryptionKey() *ecies.PrivateKey {
	fp.mutex.RLock()
	defer fp.mutex.RUnlock()

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

	hash := common.Hash(crypto.Hash(seed))

	sig := fp.secStore.Sign(hash.Bytes())

	flipKey, _ := crypto.GenerateKeyFromSeed(bytes.NewReader(sig))

	return ecies.ImportECDSA(flipKey)
}

func (fp *Flipper) LoadInMemory(cids [][]byte) {
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
		if err := ipfsFlip.FromBytes(data); err != nil {
			fp.log.Warn("Can't decode flip", "cid", cid.String(), "err", err)
			continue
		}
		fp.mutex.Lock()
		fp.flips[common.Hash(crypto.Hash(key))] = ipfsFlip
		fp.mutex.Unlock()
	}
	fp.log.Info("All flips were loaded")
}

func (fp *Flipper) Clear() {
	fp.mutex.Lock()
	defer fp.mutex.Unlock()

	fp.cancelLoadingCtx()
	fp.flips = make(map[common.Hash]*IpfsFlip)
	fp.flipReadiness = make(map[common.Hash]bool)
	fp.Initialize()
	fp.flipPrivateKey = nil
	fp.flipPublicKey = nil
	fp.loadingCtx, fp.cancelLoadingCtx = context.WithCancel(context.Background())
}

func (fp *Flipper) HasFlipInMemory(hash common.Hash) bool {
	fp.mutex.RLock()
	defer fp.mutex.RUnlock()

	_, ok := fp.flips[hash]
	return ok
}

func (fp *Flipper) SetFlipReadiness(hash common.Hash) {
	fp.mutex.RLock()
	defer fp.mutex.RUnlock()

	fp.flipReadiness[hash] = true
}

func (fp *Flipper) GetFlipReadiness(hash common.Hash) bool {
	fp.mutex.RLock()
	defer fp.mutex.RUnlock()

	ready := fp.flipReadiness[hash]
	return ready
}

func (fp *Flipper) IsFlipAvailable(key []byte) bool {
	hash := common.Hash(crypto.Hash(key))

	fp.mutex.RLock()
	_, ok := fp.flips[hash]
	fp.mutex.RUnlock()

	return ok
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
	if err := ipfsFlip.FromBytes(data); err != nil {
		return nil, err
	}

	return ipfsFlip, nil
}
