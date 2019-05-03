package flip

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/mempool"
	"idena-go/crypto"
	"idena-go/crypto/ecies"
	"idena-go/database"
	"idena-go/ipfs"
	"idena-go/log"
	"idena-go/rlp"
	"idena-go/secstore"
	"sync"
)

type Flipper struct {
	repo      *database.Repo
	log       log.Logger
	keyspool  *mempool.KeysPool
	ipfsProxy ipfs.Proxy
	hasFlips  bool
	mutex     sync.Mutex
	secStore  *secstore.SecStore
	flips     map[common.Hash]*IpfsFlip
}

func NewFlipper(db dbm.DB, ipfsProxy ipfs.Proxy, keyspool *mempool.KeysPool, secStore *secstore.SecStore) *Flipper {
	return &Flipper{
		repo:      database.NewRepo(db),
		log:       log.New(),
		ipfsProxy: ipfsProxy,
		keyspool:  keyspool,
		secStore:  secStore,
		flips:     make(map[common.Hash]*IpfsFlip),
	}
}

type IpfsFlip struct {
	Data   []byte
	Epoch  uint16
	PubKey []byte
}

func (fp *Flipper) AddNewFlip(flip types.Flip) error {
	pubKey, err := types.SenderPubKey(flip.Tx)
	if err != nil {
		return errors.Errorf("flip tx has invalid pubkey, tx: %v", flip.Tx.Hash())
	}
	ipf := IpfsFlip{
		Data:   flip.Data,
		Epoch:  flip.Tx.Epoch,
		PubKey: pubKey,
	}

	data, _ := rlp.EncodeToBytes(ipf)

	c, err := fp.ipfsProxy.Cid(data)

	if err != nil {
		return err
	}

	if bytes.Compare(c.Bytes(), flip.Tx.Payload) != 0 {
		return errors.Errorf("tx cid and flip cid mismatch, tx: %v", flip.Tx.Hash())
	}

	_, err = fp.ipfsProxy.Add(data)

	return err
}

func (fp *Flipper) PrepareFlip(epoch uint16, hex []byte) (cid.Cid, []byte, error) {

	encryptionKey := fp.GetFlipEncryptionKey(epoch)

	encrypted, err := ecies.Encrypt(rand.Reader, &encryptionKey.PublicKey, hex, nil, nil)

	if err != nil {
		return cid.Cid{}, nil, err
	}

	ipf := IpfsFlip{
		Data:   encrypted,
		Epoch:  epoch,
		PubKey: fp.secStore.GetPubKey(),
	}

	ipfsData, _ := rlp.EncodeToBytes(ipf)

	c, err := fp.ipfsProxy.Cid(ipfsData)

	if err != nil {
		return cid.Cid{}, nil, err
	}

	return c, encrypted, nil
}

func (fp *Flipper) GetFlip(key []byte) ([]byte, uint16, error) {

	ipfsFlip := fp.flips[common.Hash(rlp.Hash(key))]

	if ipfsFlip == nil {
		data, err := fp.ipfsProxy.Get(key)
		if err != nil {
			return nil, 0, err
		}
		if err := rlp.Decode(bytes.NewReader(data), ipfsFlip); err != nil {
			return nil, 0, err
		}
	}

	// if flip is mine
	var encryptionKey *ecies.PrivateKey
	if bytes.Compare(ipfsFlip.PubKey, fp.secStore.GetPubKey()) == 0 {
		encryptionKey = fp.GetFlipEncryptionKey(ipfsFlip.Epoch)
		if encryptionKey == nil {
			return nil, 0, errors.New("flip key is missing")
		}
	} else {
		addr, _ := crypto.PubKeyBytesToAddress(ipfsFlip.PubKey)
		flipKey := fp.keyspool.GetFlipKey(addr)
		if flipKey == nil {
			return nil, 0, errors.New("flip key is missing")
		}
		ecdsaKey, _ := crypto.ToECDSA(flipKey.Key)
		encryptionKey = ecies.ImportECDSA(ecdsaKey)
	}

	decryptedFlip, err := encryptionKey.Decrypt(ipfsFlip.Data, nil, nil)

	if err != nil {
		return nil, 0, err
	}

	return decryptedFlip, ipfsFlip.Epoch, nil
}

func (fp *Flipper) GetFlipEncryptionKey(epoch uint16) *ecies.PrivateKey {
	fp.mutex.Lock()
	defer fp.mutex.Unlock()

	key := fp.repo.ReadFlipKey(epoch)
	var ecdsaKey *ecdsa.PrivateKey

	if key == nil {
		ecdsaKey, _ = crypto.GenerateKey()
		key = crypto.FromECDSA(ecdsaKey)
		fp.repo.WriteFlipKey(epoch, key)
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

		fp.flips[common.Hash(rlp.Hash(key))] = ipfsFlip
	}
	fp.log.Info("All flips were loaded")
	fp.hasFlips = true
}

func (fp *Flipper) Reset() {
	fp.hasFlips = false
	fp.flips = make(map[common.Hash]*IpfsFlip)
	fp.keyspool.Clear()
}

func (fp *Flipper) HasFlips() bool {
	return fp.hasFlips
}

func (fp *Flipper) IsFlipReady(cid []byte) bool {
	flip := fp.flips[common.Hash(rlp.Hash(cid))]
	if flip == nil {
		return false
	}

	addr, _ := crypto.PubKeyBytesToAddress(flip.PubKey)

	return fp.keyspool.GetFlipKey(addr) != nil
}
