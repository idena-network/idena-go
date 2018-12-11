package types

import (
	"math"
	"math/big"
)

func CalculateFee(networkSize int, tx *Transaction) *big.Int {
	fee := new(big.Int)
	if tx.Type == ApprovingTx {
		return fee
	}
	size := networkSize
	if size <= 20 {
		return big.NewInt(5e+18)
	}
	if size <= 50 {
		return big.NewInt(2e+18)
	}
	pow := (int)(math.Log10(float64(size-1))+1) - 2
	coef := big.NewInt(int64(math.Pow(float64(10), float64(pow))))
	return new(big.Int).Div(big.NewInt(1e+18), coef)
}

func CalculateCost(networkSize int, tx *Transaction) *big.Int {
	return new(big.Int).Add(tx.Amount, CalculateFee(networkSize, tx))
}
