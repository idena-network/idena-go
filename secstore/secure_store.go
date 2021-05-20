package secstore

import (
	"encoding/hex"
	"fmt"
	"github.com/awnumar/memguard"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	"github.com/idena-network/idena-go/crypto/vrf/p256"
	"github.com/idena-network/idena-go/log"
	"github.com/pkg/errors"
	"os"
	"sync"
)

type SecStore struct {
	buffer                *memguard.LockedBuffer
	externalKeysMutes     sync.Mutex
	externalBuffersByAddr map[common.Address]*memguard.LockedBuffer
}

func NewSecStore() *SecStore {
	s := &SecStore{}
	memguard.CatchSignal(func(signal os.Signal) {
		fmt.Println("Memguard: interrupt signal received. Exiting...")
		s.Destroy()
	}, os.Interrupt)
	return s
}

func (s *SecStore) AddKey(secret []byte) {
	buffer := memguard.NewBufferFromBytes(secret)
	s.buffer = buffer
}

func (s *SecStore) AddExternalKey(secret []byte) error {
	s.externalKeysMutes.Lock()
	defer s.externalKeysMutes.Unlock()

	if s.externalBuffersByAddr == nil {
		s.externalBuffersByAddr = make(map[common.Address]*memguard.LockedBuffer)
	}
	buffer := memguard.NewBufferFromBytes(secret)
	privateKey, err := crypto.ToECDSA(buffer.Bytes())
	if err != nil {
		return errors.Wrap(err, "failed to get private key")
	}
	addr := crypto.PubkeyToAddress(privateKey.PublicKey)
	s.externalBuffersByAddr[addr] = buffer
	log.Info(fmt.Sprintf("Added external key for address %v", addr.Hex()))
	return nil
}

func (s *SecStore) ClearExternalKeys() {
	s.externalKeysMutes.Lock()
	defer s.externalKeysMutes.Unlock()

	if s.externalBuffersByAddr == nil {
		return
	}
	for _, buffer := range s.externalBuffersByAddr {
		buffer.Destroy()
	}
	s.externalBuffersByAddr = nil
}

func (s *SecStore) GetExternalAddresses() map[common.Address]struct{} {
	s.externalKeysMutes.Lock()
	defer s.externalKeysMutes.Unlock()

	res := make(map[common.Address]struct{}, len(s.externalBuffersByAddr))
	for addr := range s.externalBuffersByAddr {
		res[addr] = struct{}{}
	}
	return res
}

func (s *SecStore) SignTx(tx *types.Transaction) (*types.Transaction, error) {
	sec, _ := crypto.ToECDSA(s.buffer.Bytes())
	return types.SignTx(tx, sec)
}

func (s *SecStore) SignFlipKey(fk *types.PublicFlipKey) (*types.PublicFlipKey, error) {
	sec, _ := crypto.ToECDSA(s.buffer.Bytes())
	return types.SignFlipKey(fk, sec)
}

func (s *SecStore) SignFlipKeysPackage(fk *types.PrivateFlipKeysPackage) (*types.PrivateFlipKeysPackage, error) {
	sec, _ := crypto.ToECDSA(s.buffer.Bytes())
	return types.SignFlipKeysPackage(fk, sec)
}

func (s *SecStore) GetAddress() common.Address {
	sec, _ := crypto.ToECDSA(s.buffer.Bytes())
	return crypto.PubkeyToAddress(sec.PublicKey)
}

func (s *SecStore) GetPubKey() []byte {
	sec, _ := crypto.ToECDSA(s.buffer.Bytes())
	return crypto.FromECDSAPub(&sec.PublicKey)
}

func (s *SecStore) VrfEvaluate(data []byte) (index [32]byte, proof []byte) {
	sec, _ := crypto.ToECDSA(s.buffer.Bytes())
	signer, err := p256.NewVRFSigner(sec)
	if err != nil {
		panic(err)
	}
	return signer.Evaluate(data)
}

func (s *SecStore) Sign(data []byte) []byte {
	sec, _ := crypto.ToECDSA(s.buffer.Bytes())
	sig, _ := crypto.Sign(data, sec)
	return sig
}

func (s *SecStore) Destroy() {
	if s.buffer != nil {
		s.buffer.Destroy()
	}
	s.ClearExternalKeys()
}

func (s *SecStore) ExportKey(password string) (string, error) {
	key := s.buffer.Bytes()
	encrypted, err := crypto.Encrypt(key, password)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(encrypted), nil
}

func (s *SecStore) DecryptMessage(data []byte) ([]byte, error) {
	sec, _ := crypto.ToECDSA(s.buffer.Bytes())
	return ecies.ImportECDSA(sec).Decrypt(data, nil, nil)
}

func (s *SecStore) DecryptExternalMessage(data []byte, address common.Address) ([]byte, error) {
	sec, _ := crypto.ToECDSA(s.externalBuffersByAddr[address].Bytes())
	return ecies.ImportECDSA(sec).Decrypt(data, nil, nil)
}
