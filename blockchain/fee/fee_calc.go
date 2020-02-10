package fee

import (
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"math/big"
)

const (
	SignatureAdditionalSize = 67

	// Approximate size to calculate delete flip tx fee
	submitFlipTxSize = 116
)

func CalculateFee(networkSize int, feePerByte *big.Int, tx *types.Transaction) *big.Int {
	txFeePerByte := getFeePerByteForTx(networkSize, feePerByte, tx)
	if txFeePerByte.Sign() == 0 {
		return big.NewInt(0)
	}
	size := getTxSizeForFee(tx)
	return new(big.Int).Mul(txFeePerByte, big.NewInt(int64(size)))
}

func getFeePerByteForTx(networkSize int, feePerByte *big.Int, tx *types.Transaction) *big.Int {
	if networkSize == 0 || feePerByte == nil {
		return big.NewInt(0)
	}
	if tx.Type == types.SubmitFlipTx || tx.Type == types.SubmitAnswersHashTx || tx.Type == types.SubmitShortAnswersTx ||
		tx.Type == types.SubmitLongAnswersTx || tx.Type == types.EvidenceTx || tx.Type == types.ActivationTx ||
		tx.Type == types.InviteTx {
		return big.NewInt(0)
	}
	if tx.Type == types.OnlineStatusTx {
		attachment := attachments.ParseOnlineStatusAttachment(tx)
		if attachment != nil && attachment.Online {
			return new(big.Int).Mul(big.NewInt(2), feePerByte)
		}
		return big.NewInt(0)
	}
	return feePerByte
}

func getTxSizeForFee(tx *types.Transaction) int {
	size := tx.Size()
	if tx.Signature == nil {
		size += SignatureAdditionalSize
	}
	if tx.Type == types.DeleteFlipTx {
		size += common.MaxFlipSize + submitFlipTxSize
	}
	return size
}

func CalculateCost(networkSize int, feePerByte *big.Int, tx *types.Transaction) *big.Int {
	result := big.NewInt(0)

	result.Add(result, tx.AmountOrZero())
	result.Add(result, tx.TipsOrZero())

	fee := CalculateFee(networkSize, feePerByte, tx)
	result.Add(result, fee)

	return result
}
