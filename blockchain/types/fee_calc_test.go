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
	//tx size = 8
	fee1 := big.NewInt(8e+15)
	fee2 := big.NewInt(4e+16)
	require.Equal(t, 0, fee1.Cmp(CalculateFee(1, tx)))
	require.Equal(t, 0, fee2.Cmp(CalculateFee(200, tx)))
}

func TestCalculateCost(t *testing.T) {
	tx := &Transaction{
		Type:   RegularTx,
		Amount: big.NewInt(1e+18),
	}
	//tx size = 16
	cost := new(big.Int).Add(big.NewInt(16e+16), tx.AmountOrZero())
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
	cost := new(big.Int).Add(big.NewInt(16e+16), tx.AmountOrZero())

	// invitation cost (110 for 100 networkSize)
	cost.Add(cost, new(big.Int).Mul(big.NewInt(InvitationCoef/networkSize), common.DnaBase))

	require.Equal(t, 0, cost.Cmp(CalculateCost(networkSize, tx)))
}
