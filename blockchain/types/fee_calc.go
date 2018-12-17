package types

import (
	"math/big"
)

func CalculateFee(networkSize int, tx *Transaction) *big.Int {
	if tx.Type == ApprovingTx || tx.Type == RevokeTx {
		return big.NewInt(0)
	}
	base := big.NewInt(1e+18)
	feePerByte := new(big.Int).Div(base, big.NewInt(int64(networkSize)))

	return new(big.Int).Mul(feePerByte, big.NewInt(int64(tx.Size())))
}

func CalculateCost(networkSize int, tx *Transaction) *big.Int {
	fee := CalculateFee(networkSize, tx)
	if tx.Amount == nil {
		return fee
	}
	return new(big.Int).Add(tx.Amount, fee)
}
