package types

import (
	"math/big"
)

const (
	InvitationCoef          = 11000
	SignatureAdditionalSize = 67
)

func CalculateFee(networkSize int, feePerByte *big.Int, tx *Transaction) *big.Int {
	size := tx.Size()
	if tx.Signature == nil {
		size += SignatureAdditionalSize
	}
	return new(big.Int).Mul(calcFeePerByte(networkSize, feePerByte, tx.Type), big.NewInt(int64(size)))
}

func calcFeePerByte(networkSize int, feePerByte *big.Int, txType TxType) *big.Int {
	if txType == SubmitAnswersHashTx || txType == SubmitFlipTx ||
		txType == SubmitShortAnswersTx || txType == SubmitLongAnswersTx || txType == EvidenceTx ||
		txType == ActivationTx || txType == InviteTx || txType == OnlineStatusTx {
		return big.NewInt(0)
	}
	if networkSize == 0 || feePerByte == nil {
		return big.NewInt(0)
	}
	return feePerByte
}

func CalculateCost(networkSize int, feePerByte *big.Int, tx *Transaction) *big.Int {
	result := big.NewInt(0)

	result.Add(result, tx.AmountOrZero())
	result.Add(result, tx.TipsOrZero())

	fee := CalculateFee(networkSize, feePerByte, tx)
	result.Add(result, fee)

	//if tx.Type == InviteTx && networkSize > 0 {
	//
	//	invitationCost := decimal.NewFromFloat(InvitationCoef / float64(networkSize))
	//	coinsPerInvitation := invitationCost.Mul(decimal.NewFromBigInt(common.DnaBase, 0))
	//
	//	result.Add(result, math.ToInt(&coinsPerInvitation))
	//}

	return result
}
