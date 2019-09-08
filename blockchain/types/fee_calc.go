package types

import (
	"github.com/idena-network/idena-go/common"
	"math/big"
)

const (
	InvitationCoef          = 11000
	SignatureAdditionalSize = 67
)

func CalculateFee(networkSize int, tx *Transaction) *big.Int {
	size := tx.Size()
	if tx.Signature == nil {
		size += SignatureAdditionalSize
	}
	return new(big.Int).Mul(calcFeePerByte(networkSize, tx.Type), big.NewInt(int64(size)))
}

func calcFeePerByte(networkSize int, txType TxType) *big.Int {
	if txType == SubmitAnswersHashTx || txType == SubmitFlipTx ||
		txType == SubmitShortAnswersTx || txType == SubmitLongAnswersTx || txType == EvidenceTx ||
		txType == ActivationTx || txType == InviteTx || txType == OnlineStatusTx {
		return big.NewInt(0)
	}
	if networkSize == 0 {
		return big.NewInt(0)
	}
	var feePerByte *big.Int
	if networkSize <= 10 {
		feePerByte = new(big.Int).Div(common.DnaBase, big.NewInt(1000))
	} else {
		feePerByte = new(big.Int).Div(common.DnaBase, big.NewInt(int64(networkSize)))
	}

	return feePerByte
}

func CalculateCost(networkSize int, tx *Transaction) *big.Int {
	result := big.NewInt(0)

	result.Add(result, tx.AmountOrZero())

	fee := CalculateFee(networkSize, tx)
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
