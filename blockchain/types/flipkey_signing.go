package types

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
)

// SignFlipKey returns flip key signed with given private key
func SignFlipKey(fk *PublicFlipKey, prv *ecdsa.PrivateKey) (*PublicFlipKey, error) {
	h := crypto.SignatureHash(fk)
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

	addr, err := recoverPlain(crypto.SignatureHash(fk), fk.Signature)
	if err != nil {
		return common.Address{}, err
	}
	fk.from.Store(addr)
	return addr, nil
}

// SignFlipKey returns flip key signed with given private key
func SignFlipKeysPackage(fk *PrivateFlipKeysPackage, prv *ecdsa.PrivateKey) (*PrivateFlipKeysPackage, error) {
	h := crypto.SignatureHash(fk)
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

	addr, err := recoverPlain(crypto.SignatureHash(fk), fk.Signature)
	if err != nil {
		return common.Address{}, err
	}
	fk.from.Store(addr)
	return addr, nil
}
