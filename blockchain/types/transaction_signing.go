package types

import (
	"crypto/ecdsa"
	"errors"
	"idena-go/common"
	"idena-go/crypto"
)

// SignTx returns transaction signed with given private key
func SignTx(tx *Transaction, prv *ecdsa.PrivateKey) (*Transaction, error) {
	h := signatureHash(tx)
	sig, err := crypto.Sign(h[:], prv)
	if err != nil {
		return nil, err
	}

	return &Transaction{
		AccountNonce: tx.AccountNonce,
		Amount:       tx.Amount,
		Payload:      tx.Payload,
		To:           tx.To,
		Type:         tx.Type,
		Signature:    sig,
	}, nil
}

// Sender may cache the address, allowing it to be used regardless of
// signing method.
func Sender(tx *Transaction) (common.Address, error) {
	if from := tx.from.Load(); from != nil {
		return from.(common.Address), nil
	}

	addr, err := recoverPlain(signatureHash(tx), tx.Signature)
	if err != nil {
		return common.Address{}, err
	}
	tx.from.Store(addr)
	return addr, nil
}

// Hash returns the hash to be signed by the sender.
// It does not uniquely identify the transaction.
func signatureHash(tx *Transaction) common.Hash {
	return rlpHash([]interface{}{
		tx.AccountNonce,
		tx.Type,
		tx.To,
		tx.Amount,
		tx.Payload,
	})
}

func recoverPlain(hash common.Hash, signature []byte) (common.Address, error) {
	// recover the public key from the signature
	pub, err := crypto.Ecrecover(hash[:], signature)
	if err != nil {
		return common.Address{}, err
	}
	if len(pub) == 0 || pub[0] != 4 {
		return common.Address{}, errors.New("invalid public key")
	}

	return crypto.PubKeyBytesToAddress(pub)
}
