package types

import (
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestCalculateFee(t *testing.T) {
	tx := &Transaction{
		Type: ApprovingTx,
	}

	require.Equal(t, 0, CalculateFee(10000, tx).Sign())

	tx = &Transaction{
		Type: SendTx,
	}
	//tx size = 7
	fee1 := big.NewInt(7e+18)
	fee2 := big.NewInt(35e+17)
	require.Equal(t, 0, fee1.Cmp(CalculateFee(1, tx)))
	require.Equal(t, 0, fee2.Cmp(CalculateFee(2, tx)))
}

func TestCalculateCost(t *testing.T) {
	tx := &Transaction{
		Type:   SendTx,
		Amount: big.NewInt(1e+18),
	}
	//tx size = 15
	cost := new(big.Int).Add(big.NewInt(15e+16), tx.Amount)
	require.Equal(t, 0, cost.Cmp(CalculateCost(100, tx)))
}
