package types

import (
	"github.com/stretchr/testify/require"
	"idena-go/crypto"
	"math/big"
	"testing"
)

func TestCalculateFee(t *testing.T) {
	tx := &Transaction{
		Type: SendTx,
	}
	//tx size = 8
	fee1 := big.NewInt(75e+15)
	fee2 := big.NewInt(375e+15)

	// not signed
	require.Equal(t, 0, fee1.Cmp(CalculateFee(1, tx)))
	require.Equal(t, 0, fee2.Cmp(CalculateFee(200, tx)))

	key, _ := crypto.GenerateKey()
	signed, _ := SignTx(tx, key)

	// signed
	require.Equal(t, 0, fee1.Cmp(CalculateFee(1, signed)))
	require.Equal(t, 0, fee2.Cmp(CalculateFee(200, signed)))
}

func TestCalculateCost(t *testing.T) {
	tx := &Transaction{
		Type:   SendTx,
		Amount: big.NewInt(1e+18),
	}
	//tx size = 16
	cost := new(big.Int).Add(big.NewInt(83e+16), tx.AmountOrZero())
	require.Equal(t, 0, cost.Cmp(CalculateCost(100, tx)))
}

func TestCalculateCostForInvitation(t *testing.T) {
	//tx size = 16
	tx := &Transaction{
		Type:   InviteTx,
		Amount: big.NewInt(1e+18),
	}
	const networkSize = 100

	//tx cost (amount = 1, fee = 0.16), total 1.16
	cost := new(big.Int).Add(big.NewInt(83e+16), tx.AmountOrZero())

	// invitation cost (110 for 100 networkSize)
	//cost.Add(cost, new(big.Int).Mul(big.NewInt(InvitationCoef/networkSize), common.DnaBase))

	require.Equal(t, 0, cost.Cmp(CalculateCost(networkSize, tx)))
}
