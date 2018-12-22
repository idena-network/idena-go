package types

import (
	"github.com/stretchr/testify/require"
	"idena-go/common"
	"math/big"
	"testing"
)

func TestCalculateFee(t *testing.T) {
	tx := &Transaction{
		Type: RegularTx,
	}
	//tx size = 7
	fee1 := big.NewInt(7e+18)
	fee2 := big.NewInt(35e+17)
	require.Equal(t, 0, fee1.Cmp(CalculateFee(1, tx)))
	require.Equal(t, 0, fee2.Cmp(CalculateFee(2, tx)))
}

func TestCalculateCost(t *testing.T) {
	tx := &Transaction{
		Type:   RegularTx,
		Amount: big.NewInt(1e+18),
	}
	//tx size = 15
	cost := new(big.Int).Add(big.NewInt(15e+16), tx.AmountOrZero())
	require.Equal(t, 0, cost.Cmp(CalculateCost(100, tx)))
}

func TestCalculateCostForInvitation(t *testing.T) {
	//tx size = 15
	tx := &Transaction{
		Type:   InviteTx,
		Amount: big.NewInt(1e+18),
	}
	const networkSize = 100

	//tx cost (amount = 1, fee = 0.15), total 1.15
	cost := new(big.Int).Add(big.NewInt(15e+16), tx.AmountOrZero())

	// invitation cost (110 for 100 networkSize)
	cost.Add(cost, new(big.Int).Mul(big.NewInt(InvitationCoef / networkSize), common.DnaBase))

	require.Equal(t, 0, cost.Cmp(CalculateCost(networkSize, tx)))
}