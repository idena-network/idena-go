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
	"os"
)

type SecStore struct {
	buffer *memguard.LockedBuffer
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
