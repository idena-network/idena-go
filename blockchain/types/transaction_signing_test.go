package types

import (
	"idena-go/crypto"
	"math/big"
	"testing"
)

func TestSignTx(t *testing.T) {
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	tx := Transaction{
		AccountNonce: 0,
		Type:         ActivationTx,
		To:           &addr,
		Amount:       new(big.Int),
	}

	signedTx, err := SignTx(&tx, key)
	if err != nil {
		t.Fatal(err)
	}

	from, err := Sender(signedTx)
	if err != nil {
		t.Fatal(err)
	}
	if from != addr {
		t.Errorf("exected from and address to be equal. Got %x want %x", from, addr)
	}
}
