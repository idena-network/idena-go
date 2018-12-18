package math

import (
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestToInt(t *testing.T) {
	v := decimal.NewFromBigInt(big.NewInt(77777000000000000), -14)
	s := "777"
	require.Equal(t, s, ToInt(&v).String())

	v = decimal.NewFromBigInt(big.NewInt(100), -10)
	s = "0"
	require.Equal(t, s, ToInt(&v).String())

	v = decimal.NewFromBigInt(big.NewInt(123), 3)
	s = "123000"
	require.Equal(t, s, ToInt(&v).String())

	v = decimal.NewFromBigInt(big.NewInt(123), 120)
	s = "123000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	require.Equal(t, s, ToInt(&v).String())
}
