package blockchain

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/shopspring/decimal"
	"math/big"
)

func BuildTx(appState *appstate.AppState, from common.Address, to *common.Address, txType types.TxType,
	amount decimal.Decimal, maxFee decimal.Decimal, tips decimal.Decimal, nonce uint32, epoch uint16,
	payload []byte) *types.Transaction {

	tx := types.Transaction{
		AccountNonce: nonce,
		Type:         txType,
		To:           to,
		Amount:       ConvertToInt(amount),
		MaxFee:       ConvertToInt(maxFee),
		Tips:         ConvertToInt(tips),
		Payload:      payload,
		Epoch:        epoch,
	}

	state := appState.State

	if tx.Epoch == 0 {
		tx.Epoch = state.Epoch()
	}

	if tx.AccountNonce == 0 {
		currentNonce := appState.NonceCache.GetNonce(from, tx.Epoch)
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

	return math.ToInt(result)
}

func ConvertToFloat(amount *big.Int) decimal.Decimal {
	if amount == nil {
		return decimal.Zero
	}
	decimalAmount := decimal.NewFromBigInt(amount, 0)

	return decimalAmount.Div(decimal.NewFromBigInt(common.DnaBase, 0))
}

func splitReward(totalReward *big.Int, isNewbie bool, conf *config.ConsensusConf) (reward, stake *big.Int) {
	rate := conf.StakeRewardRate
	if isNewbie {
		rate = conf.StakeRewardRateForNewbie
	}

	stakeD := decimal.NewFromBigInt(totalReward, 0).Mul(decimal.NewFromFloat32(rate))
	stake = math.ToInt(stakeD)

	reward = big.NewInt(0)
	reward = reward.Sub(totalReward, stake)
	return reward, stake
}
