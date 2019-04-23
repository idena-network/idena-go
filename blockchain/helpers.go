package blockchain

import (
	"github.com/shopspring/decimal"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/math"
	"idena-go/core/appstate"
	"math/big"
)

func BuildTx(appState *appstate.AppState, from common.Address, to common.Address, txType types.TxType, amount decimal.Decimal,
	nonce uint32, epoch uint16, payload []byte) *types.Transaction {

	tx := types.Transaction{
		AccountNonce: nonce,
		Type:         txType,
		To:           &to,
		Amount:       ConvertToInt(amount),
		Payload:      payload,
		Epoch:        epoch,
	}

	state := appState.State

	if tx.Epoch == 0 {
		tx.Epoch = state.Epoch()
	}

	if tx.AccountNonce == 0 && tx.Epoch == state.Epoch() {
		currentNonce := appState.NonceCache.GetNonce(from)
		// if epoch was increased, we should reset nonce to 1
		if state.GetEpoch(from) < state.Epoch() {
			//TODO: should not be zero if we switched to new epoch
			currentNonce = 0
		}
		tx.AccountNonce = currentNonce + 1
	}

	return &tx
}

func ConvertToInt(amount decimal.Decimal) *big.Int {
	if amount == (decimal.Decimal{}) {
		return nil
	}
	initial := decimal.NewFromBigInt(common.DnaBase, 0)
	result := amount.Mul(initial)

	return math.ToInt(&result)
}

func ConvertToFloat(amount *big.Int) decimal.Decimal {
	if amount == nil {
		return decimal.Zero
	}
	decimalAmount := decimal.NewFromBigInt(amount, 0)

	return decimalAmount.Div(decimal.NewFromBigInt(common.DnaBase, 0))
}
