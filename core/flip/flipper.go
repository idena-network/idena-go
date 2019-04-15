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
)

type Flipper struct {
	repo      *database.Repo
	log       log.Logger
	ipfsProxy ipfs.Proxy
	pubKey    []byte
}

func NewFlipper(db dbm.DB, ipfsProxy ipfs.Proxy) *Flipper {
	return &Flipper{
		repo:      database.NewRepo(db),
		log:       log.New(),
		ipfsProxy: ipfsProxy,
	}
}

type IpfsFlip struct {
	Data   []byte
	Epoch  uint16
	PubKey []byte
}

type FlipKey struct {
	Key *ecies.PrivateKey
	Cid cid.Cid
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

	encryptionKey := fp.getFlipEncryptionKey(epoch)

	encrypted, err := ecies.Encrypt(rand.Reader, &encryptionKey.PublicKey, hex, nil, nil)

	if err != nil {
		return cid.Cid{}, nil, err
	}

	ipf := IpfsFlip{
		Data:   encrypted,
		Epoch:  epoch,
		PubKey: fp.pubKey,
	}

	ipfsData, _ := rlp.EncodeToBytes(ipf)

	c, err := fp.ipfsProxy.Cid(ipfsData)

	if err != nil {
		return cid.Cid{}, nil, err
	}

	return c, encrypted, nil
}

func (fp *Flipper) GetFlip(key []byte) ([]byte, uint16, error) {
	data, err := fp.ipfsProxy.Get(key)
	if err != nil {
		return nil, 0, err
	}

	ipf := new(IpfsFlip)
	if err := rlp.Decode(bytes.NewReader(data), ipf); err != nil {
		return nil, 0, err
	}

	encryptionKey := fp.getFlipEncryptionKey(ipf.Epoch)
	if encryptionKey == nil {
		return nil, 0, errors.New("flip key is missing")
	}

	decryptedFlip, err := encryptionKey.Decrypt(ipf.Data, nil, nil)

	if err != nil {
		return nil, 0, err
	}

	return decryptedFlip, ipf.Epoch, nil
}

func (fp *Flipper) GetMyEncryptionKeys(epoch uint16) []FlipKey {
	return make([]FlipKey, 0)
}

func (fp *Flipper) getFlipEncryptionKey(epoch uint16) *ecies.PrivateKey {
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
