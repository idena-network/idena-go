package types

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/rlp"
)

// SignFlipKey returns flip key signed with given private key
func SignFlipKey(fk *FlipKey, prv *ecdsa.PrivateKey) (*FlipKey, error) {
	h := signatureFlipKeyHash(fk)
	sig, err := crypto.Sign(h[:], prv)
	if err != nil {
		return nil, err
	}

	return &FlipKey{
		Key:       fk.Key,
		Epoch:     fk.Epoch,
		Signature: sig,
	}, nil
}

// Sender may cache the address, allowing it to be used regardless of
// signing method.
func SenderFlipKey(fk *FlipKey) (common.Address, error) {
	if from := fk.from.Load(); from != nil {
		return from.(common.Address), nil
	}

	addr, err := recoverPlain(signatureFlipKeyHash(fk), fk.Signature)
	if err != nil {
		return common.Address{}, err
	}
	fk.from.Store(addr)
	return addr, nil
}

func SenderFlipKeyPubKey(fk *FlipKey) ([]byte, error) {
	return crypto.Ecrecover(signatureFlipKeyHash(fk).Bytes(), fk.Signature)
}

// Hash returns the hash to be signed by the sender.
// It does not uniquely identify the transaction.
func signatureFlipKeyHash(fk *FlipKey) common.Hash {
	return rlp.Hash([]interface{}{
		fk.Key,
		fk.Epoch,
	})
}
