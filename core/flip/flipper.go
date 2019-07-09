package flip

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/secstore"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tendermint/libs/db"
	"sync"
)

type Flipper struct {
	epochDb    *database.EpochDb
	db         dbm.DB
	log        log.Logger
	keyspool   *mempool.KeysPool
	ipfsProxy  ipfs.Proxy
	hasFlips   bool
	mutex      sync.Mutex
	flipsMutex sync.Mutex
	secStore   *secstore.SecStore
	flips      map[common.Hash]*IpfsFlip
	appState   *appstate.AppState
	txpool     *mempool.TxPool
}
type IpfsFlip struct {
	Data   []byte
	PubKey []byte
}

func NewFlipper(db dbm.DB, ipfsProxy ipfs.Proxy, keyspool *mempool.KeysPool, txpool *mempool.TxPool, secStore *secstore.SecStore, appState *appstate.AppState) *Flipper {
	fp := &Flipper{
		db:        db,
		log:       log.New(),
		ipfsProxy: ipfsProxy,
		keyspool:  keyspool,
		txpool:    txpool,
		secStore:  secStore,
		flips:     make(map[common.Hash]*IpfsFlip),
		appState:  appState,
	}

	return fp
}

func (fp *Flipper) Initialize() {
	fp.epochDb = database.NewEpochDb(fp.db, fp.appState.State.Epoch())
}

func (fp *Flipper) AddNewFlip(flip types.Flip) error {
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

	c, err := fp.ipfsProxy.Cid(data)

	if err != nil {
		return err
	}

	if fp.epochDb.HasFlipCid(c.Bytes()) {
		return errors.New("duplicate flip")
	}

	if bytes.Compare(c.Bytes(), flip.Tx.Payload) != 0 {
		return errors.Errorf("tx cid and flip cid mismatch, tx: %v", flip.Tx.Hash())
	}

	if err := fp.txpool.Add(flip.Tx); err != nil && err != mempool.DuplicateTxError {
		log.Warn("Flip Tx is not valid", "hash", flip.Tx.Hash().Hex(), "err", err)
		return err
	}

	_, err = fp.ipfsProxy.Add(data)

	fp.epochDb.WriteFlipCid(c.Bytes())

	return err
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

	fp.flipsMutex.Lock()
	ipfsFlip := fp.flips[common.Hash(rlp.Hash(key))]
	fp.flipsMutex.Unlock()

	if ipfsFlip == nil {
		data, err := fp.ipfsProxy.Get(key)
		if err != nil {
			return nil, err
		}
		ipfsFlip = new(IpfsFlip)
		if err := rlp.Decode(bytes.NewReader(data), ipfsFlip); err != nil {
			return nil, err
		}
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

	key := fp.epochDb.ReadFlipKey()
	var ecdsaKey *ecdsa.PrivateKey

	if key == nil {
		ecdsaKey, _ = crypto.GenerateKey()
		key = crypto.FromECDSA(ecdsaKey)
		fp.epochDb.WriteFlipKey(key)
	} else {
		ecdsaKey, _ = crypto.ToECDSA(key)
	}

	return ecies.ImportECDSA(ecdsaKey)
}

func (fp *Flipper) Load(cids [][]byte) {
	for len(cids) > 0 {

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
			fp.log.Warn("Can't decode flip", "cid", cid.String(), "err", err)
			continue
		}
		fp.flipsMutex.Lock()
		fp.flips[common.Hash(rlp.Hash(key))] = ipfsFlip
		fp.flipsMutex.Unlock()
	}
	fp.log.Info("All flips were loaded")
	fp.hasFlips = true
}

func (fp *Flipper) Reset() {
	fp.hasFlips = false
	fp.flips = make(map[common.Hash]*IpfsFlip)
	fp.keyspool.Clear()
	fp.Initialize()
}

func (fp *Flipper) HasFlips() bool {
	return fp.hasFlips
}

func (fp *Flipper) IsFlipReady(cid []byte) bool {
	fp.flipsMutex.Lock()
	flip := fp.flips[common.Hash(rlp.Hash(cid))]
	fp.flipsMutex.Unlock()
	if flip == nil {
		return false
	}

	addr, _ := crypto.PubKeyBytesToAddress(flip.PubKey)

	return fp.keyspool.GetFlipKey(addr) != nil || addr == fp.secStore.GetAddress()
}

func (fp *Flipper) UnpinFlip(flipCid []byte) {
	fp.ipfsProxy.Unpin(flipCid)
}
