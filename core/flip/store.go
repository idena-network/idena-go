package flip

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/crypto"
	"idena-go/crypto/ecies"
	"idena-go/crypto/sha3"
	"idena-go/database"
	"idena-go/log"
	"idena-go/rlp"
	"sync"
)

type Store interface {
	PrepareFlip(epoch uint16, category uint16, left []byte, right []byte) (common.Hash, error)

	GetFlip(hash common.Hash) (*types.Flip, error)

	AddMinedFlip(hash common.Hash, epoch uint16)

	Commit()

	Reset()
}

type EmptyFlipStore struct{}

func NewEmptyStore() *EmptyFlipStore {
	return &EmptyFlipStore{}
}

func (EmptyFlipStore) PrepareFlip(epoch uint16, category uint16, left []byte, right []byte) (common.Hash, error) {
	panic("implement me")
}

func (EmptyFlipStore) GetFlip(hash common.Hash) (*types.Flip, error) {
	panic("implement me")
}

func (EmptyFlipStore) AddMinedFlip(hash common.Hash, epoch uint16) {
}

func (EmptyFlipStore) Commit() {
}

func (EmptyFlipStore) Reset() {
}

type FlipStore struct {
	repo   *database.Repo
	log    log.Logger
	toSave []*FlipToSave
	lock   sync.Mutex
}

type FlipToSave struct {
	hash  common.Hash
	epoch uint16
}

func NewStore(db dbm.DB) *FlipStore {
	return &FlipStore{
		repo: database.NewRepo(db),
		log:  log.New(),
	}
}

func (store *FlipStore) PrepareFlip(epoch uint16, category uint16, left []byte, right []byte) (common.Hash, error) {
	store.lock.Lock()
	defer store.lock.Unlock()

	encryptionKey := store.getFlipEncryptionKey(epoch)

	flipData := &types.FlipData{
		Category: category,
		Left:     left,
		Right:    right,
	}

	flipRlp, _ := rlp.EncodeToBytes(flipData)

	encFlip, err := ecies.Encrypt(rand.Reader, &encryptionKey.PublicKey, flipRlp, nil, nil)

	flipHash := rlpHash(encFlip)

	store.repo.WriteFlip(flipHash, &types.EncryptedFlip{
		Mined: false,
		Epoch: epoch,
		Data:  encFlip,
	})

	if err != nil {
		return common.Hash{}, err
	}

	return flipHash, nil
}

func (store *FlipStore) GetFlip(hash common.Hash) (*types.Flip, error) {
	flip := store.repo.ReadFlip(hash)
	encryptionKey := store.getFlipEncryptionKey(flip.Epoch)
	if encryptionKey == nil {
		return nil, errors.New("flip key is missing")
	}

	decryptedFlip, err := encryptionKey.Decrypt(flip.Data, nil, nil)

	if err != nil {
		return nil, err
	}

	flipData := new(types.FlipData)
	if err := rlp.Decode(bytes.NewReader(decryptedFlip), flipData); err != nil {
		log.Error("invalid flip", "err", err)
		return nil, err
	}

	return &types.Flip{
		Data:  flipData,
		Epoch: flip.Epoch,
		Mined: flip.Mined,
	}, nil
}

func (store *FlipStore) AddMinedFlip(hash common.Hash, epoch uint16) {
	store.toSave = append(store.toSave, &FlipToSave{
		epoch: epoch,
		hash:  hash,
	})
}

func (store *FlipStore) Commit() {
	store.lock.Lock()
	defer store.lock.Unlock()

	for _, flip := range store.toSave {
		store.repo.SetFlipMined(flip.hash, flip.epoch)
	}

	store.toSave = []*FlipToSave{}
}

func (store *FlipStore) Reset() {
	store.toSave = []*FlipToSave{}
}

func (store *FlipStore) getFlipEncryptionKey(epoch uint16) *ecies.PrivateKey {
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

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}
