package types

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/rlp"
)

// SignFlipKey returns flip key signed with given private key
func SignFlipKey(fk *PublicFlipKey, prv *ecdsa.PrivateKey) (*PublicFlipKey, error) {
	h := signatureFlipKeyHash(fk)
	sig, err := crypto.Sign(h[:], prv)
	if err != nil {
		return nil, err
	}

	return &PublicFlipKey{
		Key:       fk.Key,
		Epoch:     fk.Epoch,
		Signature: sig,
	}, nil
}

// Sender may cache the address, allowing it to be used regardless of
// signing method.
func SenderFlipKey(fk *PublicFlipKey) (common.Address, error) {
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

// SignFlipKey returns flip key signed with given private key
func SignFlipKeysPackage(fk *PrivateFlipKeysPackage, prv *ecdsa.PrivateKey) (*PrivateFlipKeysPackage, error) {
	h := signatureFlipKeysPackageHash(fk)
	sig, err := crypto.Sign(h[:], prv)
	if err != nil {
		return nil, err
	}

	return &PrivateFlipKeysPackage{
		Data:      fk.Data,
		Epoch:     fk.Epoch,
		Signature: sig,
	}, nil
}

// Sender may cache the address, allowing it to be used regardless of
// signing method.
func SenderFlipKeysPackage(fk *PrivateFlipKeysPackage) (common.Address, error) {
	if from := fk.from.Load(); from != nil {
		return from.(common.Address), nil
	}

	addr, err := recoverPlain(signatureFlipKeysPackageHash(fk), fk.Signature)
	if err != nil {
		return common.Address{}, err
	}
	fk.from.Store(addr)
	return addr, nil
}

func SenderFlipKeyPubKey(fk *PublicFlipKey) ([]byte, error) {
	return crypto.Ecrecover(signatureFlipKeyHash(fk).Bytes(), fk.Signature)
}

// Hash returns the hash to be signed by the sender.
// It does not uniquely identify the transaction.
func signatureFlipKeysPackageHash(fk *PrivateFlipKeysPackage) common.Hash {
	return rlp.Hash([]interface{}{
		fk.Data,
		fk.Epoch,
	})
}

// Hash returns the hash to be signed by the sender.
// It does not uniquely identify the transaction.
func signatureFlipKeyHash(fk *PublicFlipKey) common.Hash {
	return rlp.Hash([]interface{}{
		fk.Key,
		fk.Epoch,
	})
}
