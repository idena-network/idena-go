package math

import (
	"github.com/shopspring/decimal"
	"math/big"
)

func abs(n int64) int64 {
	y := n >> 63
	return (n ^ y) - y
}
func ToInt(value *decimal.Decimal) *big.Int {

	m := value.Coefficient()
	exp := value.Exponent()

	if exp == 0 {
		return m
	}

	coef := big.NewInt(1)

	for i := int64(0); i < abs(int64(exp)); i++ {
		coef.Mul(coef, big.NewInt(10))
	}

	if exp < 0 {
		m.Quo(m, coef)
	} else {
		m.Mul(m, coef)
	}
	return m
}
