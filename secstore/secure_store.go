package secstore

import (
	"fmt"
	"github.com/awnumar/memguard"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/crypto"
	"idena-go/crypto/vrf/p256"
)

type SecStore struct {
	buffer *memguard.LockedBuffer
}

func NewSecStore() *SecStore {
	s := &SecStore{}
	memguard.CatchInterrupt(func() {
		fmt.Println("Memguard: interrupt signal received. Exiting...")
		s.Destroy()
	})
	return s
}

func (s *SecStore) AddKey(secret []byte) {
	buffer, err := memguard.NewImmutableFromBytes(secret)
	if err != nil {
		memguard.DestroyAll()
		panic(err)
	}
	s.buffer = buffer
}

func (s *SecStore) SignTx(tx *types.Transaction) (*types.Transaction, error) {
	sec, _ := crypto.ToECDSA(s.buffer.Buffer())
	return types.SignTx(tx, sec)
}

func (s *SecStore) SignFlipKey(fk *types.FlipKey) (*types.FlipKey, error) {
	sec, _ := crypto.ToECDSA(s.buffer.Buffer())
	return types.SignFlipKey(fk, sec)
}

func (s *SecStore) GetAddress() common.Address {
	sec, _ := crypto.ToECDSA(s.buffer.Buffer())
	return crypto.PubkeyToAddress(sec.PublicKey)
}

func (s *SecStore) GetPubKey() []byte {
	sec, _ := crypto.ToECDSA(s.buffer.Buffer())
	return crypto.FromECDSAPub(&sec.PublicKey)
}

func (s *SecStore) VrfEvaluate(data []byte) (index [32]byte, proof []byte) {
	sec, _ := crypto.ToECDSA(s.buffer.Buffer())
	signer, err := p256.NewVRFSigner(sec)
	if err != nil {
		panic(err)
	}
	return signer.Evaluate(data)
}

func (s *SecStore) Sign(data []byte) []byte {
	sec, _ := crypto.ToECDSA(s.buffer.Buffer())
	sig, _ := crypto.Sign(data, sec)
	return sig
}

func (s *SecStore) Destroy() {
	if s.buffer != nil {
		s.buffer.Destroy()
	}
}
