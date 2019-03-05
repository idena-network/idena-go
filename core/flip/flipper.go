package flip

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/crypto"
	"idena-go/crypto/ecies"
	"idena-go/database"
	"idena-go/ipfs"
	"idena-go/log"
	"idena-go/rlp"
	"sync"
)

type Flipper struct {
	repo      *database.Repo
	log       log.Logger
	lock      sync.Mutex
	ipfsProxy ipfs.Proxy
}

func NewStore(db dbm.DB, ipfsProxy ipfs.Proxy) *Flipper {
	return &Flipper{
		repo:      database.NewRepo(db),
		log:       log.New(),
		ipfsProxy: ipfsProxy,
	}
}

type IpfsFlip struct {
	Data  []byte
	Epoch uint16
}

func (store *Flipper) AddNewFlip(flip types.Flip) error {
	ipf := IpfsFlip{
		Data:  flip.Data,
		Epoch: flip.Tx.Epoch,
	}

	data, _ := rlp.EncodeToBytes(ipf)

	c, err := store.ipfsProxy.Cid(data)

	if err != nil {
		return err
	}

	if bytes.Compare(c.Bytes(), flip.Tx.Payload) != 0 {
		return errors.Errorf("tx cid and flip cid mismatch, tx: %v", flip.Tx.Hash())
	}

	_, err = store.ipfsProxy.Add(data)

	return err
}

func (store *Flipper) PrepareFlip(epoch uint16, hex []byte) (cid.Cid, []byte, error) {
	store.lock.Lock()
	defer store.lock.Unlock()

	encryptionKey := store.getFlipEncryptionKey(epoch)

	encrypted, err := ecies.Encrypt(rand.Reader, &encryptionKey.PublicKey, hex, nil, nil)

	if err != nil {
		return cid.Cid{}, nil, err
	}

	ipf := IpfsFlip{
		Data:  encrypted,
		Epoch: epoch,
	}

	ipfsData, _ := rlp.EncodeToBytes(ipf)

	c, err := store.ipfsProxy.Cid(ipfsData)

	if err != nil {
		return cid.Cid{}, nil, err
	}

	return c, encrypted, nil
}

func (store *Flipper) GetFlip(key []byte) ([]byte, uint16, error) {
	data, err := store.ipfsProxy.Get(key)
	if err != nil {
		return nil, 0, err
	}

	ipf := new(IpfsFlip)
	if err := rlp.Decode(bytes.NewReader(data), ipf); err != nil {
		return nil, 0, err
	}

	encryptionKey := store.getFlipEncryptionKey(ipf.Epoch)
	if encryptionKey == nil {
		return nil, 0, errors.New("flip key is missing")
	}

	decryptedFlip, err := encryptionKey.Decrypt(ipf.Data, nil, nil)

	if err != nil {
		return nil, 0, err
	}

	return decryptedFlip, ipf.Epoch, nil
}

func (store *Flipper) getFlipEncryptionKey(epoch uint16) *ecies.PrivateKey {
	key := store.repo.ReadFlipKey(epoch)
	var ecdsaKey *ecdsa.PrivateKey

	if key == nil {
		ecdsaKey, _ = crypto.GenerateKey()
		key = crypto.FromECDSA(ecdsaKey)
		store.repo.WriteFlipKey(epoch, key)
	} else {
		ecdsaKey, _ = crypto.ToECDSA(key)
	}

	return ecies.ImportECDSA(ecdsaKey)
}
