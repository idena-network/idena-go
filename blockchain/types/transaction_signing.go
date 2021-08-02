package types

import (
	"crypto/ecdsa"
	"errors"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/rlp"
)

// SignTx returns transaction signed with given private key
func SignTx(tx *Transaction, prv *ecdsa.PrivateKey) (*Transaction, error) {
	h := crypto.SignatureHash(tx)
	sig, err := crypto.Sign(h[:], prv)
	if err != nil {
		return nil, err
	}

	return &Transaction{
		AccountNonce: tx.AccountNonce,
		Epoch:        tx.Epoch,
		Amount:       tx.Amount,
		MaxFee:       tx.MaxFee,
		Tips:         tx.Tips,
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

	var hash common.Hash
	if tx.UseRlp {
		hash = signatureHash(tx)
	} else {
		hash = crypto.SignatureHash(tx)
	}

	addr, err := recoverPlain(hash, tx.Signature)
	if err != nil {
		return common.Address{}, err
	}
	tx.from.Store(addr)
	return addr, nil
}

func IsValidLongSessionAnswers(tx *Transaction) bool {
	if valid := tx.validLongSessionAnswersProof.Load(); valid != nil {
		return valid.(bool)
	}
	return false
}

func MarkAsValidLongSessionAnswers(tx *Transaction) {
	tx.validLongSessionAnswersProof.Store(true)
}

// Sender may cache the address, allowing it to be used regardless of
// signing method.
func SenderPubKey(tx *Transaction) ([]byte, error) {
	var hash common.Hash
	if tx.UseRlp {
		hash = signatureHash(tx)
	} else {
		hash = crypto.SignatureHash(tx)
	}
	return crypto.Ecrecover(hash[:], tx.Signature)
}

// Hash returns the hash to be signed by the sender.
// It does not uniquely identify the transaction.
// Deprecated: use crypto.SignatureHash instead of this
func signatureHash(tx *Transaction) common.Hash {
	return rlp.Hash([]interface{}{
		tx.AccountNonce,
		tx.Epoch,
		tx.Type,
		tx.To,
		tx.Amount,
		tx.MaxFee,
		tx.Tips,
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
