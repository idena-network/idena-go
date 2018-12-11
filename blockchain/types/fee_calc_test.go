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

	fee1 := big.NewInt(5e+18)
	fee2 := big.NewInt(2e+18)
	fee3 := big.NewInt(1e+18)
	fee4 := big.NewInt(1e+17)
	fee5 := big.NewInt(1e+16)

	require.Equal(t, 0, fee1.Cmp(CalculateFee(19, tx)))
	require.Equal(t, 0, fee2.Cmp(CalculateFee(50, tx)))
	require.Equal(t, 0, fee3.Cmp(CalculateFee(100, tx)))

	require.Equal(t, 0, fee4.Cmp(CalculateFee(101, tx)))
	require.Equal(t, 0, fee4.Cmp(CalculateFee(999, tx)))
	require.Equal(t, 0, fee4.Cmp(CalculateFee(1000, tx)))

	require.Equal(t, 0, fee5.Cmp(CalculateFee(1001, tx)))
}

func TestCalculateCost(t *testing.T) {
	tx := &Transaction{
		Type:   SendTx,
		Amount: big.NewInt(1e+18),
	}
	cost := big.NewInt(2e+18)
	require.Equal(t, 0, cost.Cmp(CalculateCost(100, tx)))
}
