package fee

import (
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/shopspring/decimal"
	"math/big"
)

const (
	SignatureAdditionalSize = 67

	deleteFlipTxAdditionalSize = 1024 * 120
)

var (
	MinFeePerByte = big.NewInt(1e+2)
)

func GetFeePerByteForNetwork(networkSize int) *big.Int {
	if networkSize == 0 {
		networkSize = 1
	}
	minFeePerByteD := decimal.NewFromFloat(0.1).
		Div(decimal.NewFromInt(int64(networkSize))).
		Mul(decimal.NewFromBigInt(common.DnaBase, 0))

	minFeePerByte := math.ToInt(minFeePerByteD)

	if minFeePerByte.Cmp(MinFeePerByte) == -1 {
		minFeePerByte = new(big.Int).Set(MinFeePerByte)
	}

	return minFeePerByte
}

func CalculateFee(networkSize int, feePerByte *big.Int, tx *types.Transaction) *big.Int {
	txFeePerByte := getFeePerByteForTx(networkSize, feePerByte, tx)
	if txFeePerByte.Sign() == 0 {
		return big.NewInt(0)
	}
	size := getTxSizeForFee(tx)
	return new(big.Int).Mul(txFeePerByte, big.NewInt(int64(size)))
}

func getFeePerByteForTx(networkSize int, feePerByte *big.Int, tx *types.Transaction) *big.Int {
	if networkSize == 0 || common.ZeroOrNil(feePerByte) {
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
		size += deleteFlipTxAdditionalSize
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
