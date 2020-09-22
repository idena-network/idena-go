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
	MinFeePerGas = big.NewInt(10)
)

func GetFeePerGasForNetwork(networkSize int) *big.Int {
	if networkSize == 0 {
		networkSize = 1
	}
	minFeePerGasD := decimal.NewFromFloat(0.01).
		Div(decimal.NewFromInt(int64(networkSize))).
		Mul(decimal.NewFromBigInt(common.DnaBase, 0))

	minFeePerGas := math.ToInt(minFeePerGasD)

	if minFeePerGas.Cmp(MinFeePerGas) == -1 {
		minFeePerGas = new(big.Int).Set(MinFeePerGas)
	}

	return minFeePerGas
}

func CalculateFee(networkSize int, feePerGas *big.Int, tx *types.Transaction) *big.Int {
	txFeePerGas := getFeePerGasForTx(networkSize, feePerGas, tx)
	if txFeePerGas.Sign() == 0 {
		return big.NewInt(0)
	}
	gas := CalculateGas(tx)
	return new(big.Int).Mul(txFeePerGas, big.NewInt(int64(gas)))
}

func CalculateGas(tx *types.Transaction) int {
	return getTxSizeForFee(tx) * 10
}

func getFeePerGasForTx(networkSize int, feePerGas *big.Int, tx *types.Transaction) *big.Int {
	if networkSize == 0 || common.ZeroOrNil(feePerGas) {
		return big.NewInt(0)
	}
	if tx.Type == types.SubmitFlipTx || tx.Type == types.SubmitAnswersHashTx || tx.Type == types.SubmitShortAnswersTx ||
		tx.Type == types.SubmitLongAnswersTx || tx.Type == types.EvidenceTx || tx.Type == types.ActivationTx ||
		tx.Type == types.InviteTx || tx.Type == types.KillTx {
		return big.NewInt(0)
	}
	if tx.Type == types.OnlineStatusTx {
		attachment := attachments.ParseOnlineStatusAttachment(tx)
		if attachment != nil && attachment.Online {
			return new(big.Int).Mul(big.NewInt(2), feePerGas)
		}
		return big.NewInt(0)
	}
	return feePerGas
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

func CalculateCost(networkSize int, feePerGas *big.Int, tx *types.Transaction) *big.Int {
	result := big.NewInt(0)

	result.Add(result, tx.AmountOrZero())
	result.Add(result, tx.TipsOrZero())

	fee := CalculateFee(networkSize, feePerGas, tx)
	result.Add(result, fee)

	return result
}

func CalculateMaxCost(tx *types.Transaction) *big.Int {
	result := big.NewInt(0)
	result.Add(result, tx.AmountOrZero())
	result.Add(result, tx.TipsOrZero())
	result.Add(result, tx.MaxFeeOrZero())
	return result
}
