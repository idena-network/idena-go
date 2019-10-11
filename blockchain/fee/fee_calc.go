package fee

import (
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"math/big"
)

const (
	SignatureAdditionalSize = 67
)

func CalculateFee(networkSize int, feePerByte *big.Int, tx *types.Transaction) *big.Int {
	size := tx.Size()
	if tx.Signature == nil {
		size += SignatureAdditionalSize
	}
	if tx.Type == types.SubmitAnswersHashTx || tx.Type == types.SubmitFlipTx ||
		tx.Type == types.SubmitShortAnswersTx || tx.Type == types.SubmitLongAnswersTx || tx.Type == types.EvidenceTx ||
		tx.Type == types.ActivationTx || tx.Type == types.InviteTx {
		feePerByte = big.NewInt(0)
	}
	if tx.Type == types.OnlineStatusTx {
		attachment := attachments.ParseOnlineStatusAttachment(tx)
		if attachment != nil && attachment.Online {
			feePerByte = new(big.Int).Mul(big.NewInt(2), feePerByte)
		} else {
			feePerByte = big.NewInt(0)
		}
	}
	if networkSize == 0 || feePerByte == nil {
		feePerByte = big.NewInt(0)
	}

	return new(big.Int).Mul(feePerByte, big.NewInt(int64(size)))
}

func CalculateCost(networkSize int, feePerByte *big.Int, tx *types.Transaction) *big.Int {
	result := big.NewInt(0)

	result.Add(result, tx.AmountOrZero())
	result.Add(result, tx.TipsOrZero())

	fee := CalculateFee(networkSize, feePerByte, tx)
	result.Add(result, fee)

	return result
}
