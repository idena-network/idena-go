package collector

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	statsTypes "github.com/idena-network/idena-go/stats/types"
	"math/big"
)

type StatsCollector interface {
	EnableCollecting()
	CompleteCollecting()

	SetValidation(validation *statsTypes.ValidationStats)
	SetMinScoreForInvite(score float32)

	SetValidationResults(authors *types.ValidationResults)

	SetTotalReward(amount *big.Int)
	SetTotalValidationReward(amount *big.Int, share *big.Int)
	SetTotalFlipsReward(amount *big.Int, share *big.Int)
	SetTotalInvitationsReward(amount *big.Int, share *big.Int)
	SetTotalFoundationPayouts(amount *big.Int)
	SetTotalZeroWalletFund(amount *big.Int)
	AddValidationReward(addr common.Address, age uint16, balance *big.Int, stake *big.Int)
	AddFlipsReward(addr common.Address, balance *big.Int, stake *big.Int, flipsToReward []*types.FlipToReward)
	AddReportedFlipsReward(addr common.Address, flipIdx int, balance *big.Int, stake *big.Int)
	AddInvitationsReward(addr common.Address, balance *big.Int, stake *big.Int, age uint16, txHash *common.Hash,
		isSavedInviteWinner bool)
	AddFoundationPayout(addr common.Address, balance *big.Int)
	AddZeroWalletFund(addr common.Address, balance *big.Int)

	AddProposerReward(addr common.Address, balance *big.Int, stake *big.Int)
	AddFinalCommitteeReward(addr common.Address, balance *big.Int, stake *big.Int)

	AfterSubPenalty(addr common.Address, amount *big.Int, appState *appstate.AppState)
	BeforeClearPenalty(addr common.Address, appState *appstate.AppState)
	BeforeSetPenalty(addr common.Address, appState *appstate.AppState)

	AddMintedCoins(amount *big.Int)
	AddPenaltyBurntCoins(addr common.Address, amount *big.Int)
	AddInviteBurntCoins(addr common.Address, amount *big.Int, tx *types.Transaction)
	AddFeeBurntCoins(addr common.Address, feeAmount *big.Int, burntRate float32, tx *types.Transaction)
	AddKilledBurntCoins(addr common.Address, amount *big.Int)
	AddBurnTxBurntCoins(addr common.Address, tx *types.Transaction)

	AfterAddStake(addr common.Address, amount *big.Int, appState *appstate.AppState)

	AddActivationTxBalanceTransfer(tx *types.Transaction, amount *big.Int)
	AddKillTxStakeTransfer(tx *types.Transaction, amount *big.Int)
	AddKillInviteeTxStakeTransfer(tx *types.Transaction, amount *big.Int)

	BeginVerifiedStakeTransferBalanceUpdate(addr common.Address, appState *appstate.AppState)
	BeginTxBalanceUpdate(tx *types.Transaction, appState *appstate.AppState)
	BeginProposerRewardBalanceUpdate(addr common.Address, appState *appstate.AppState)
	BeginCommitteeRewardBalanceUpdate(addr common.Address, appState *appstate.AppState)
	BeginEpochRewardBalanceUpdate(addr common.Address, appState *appstate.AppState)
	BeginFailedValidationBalanceUpdate(addr common.Address, appState *appstate.AppState)
	BeginPenaltyBalanceUpdate(addr common.Address, appState *appstate.AppState)
	BeginEpochPenaltyResetBalanceUpdate(addr common.Address, appState *appstate.AppState)
	BeginDustClearingBalanceUpdate(addr common.Address, appState *appstate.AppState)
	CompleteBalanceUpdate(appState *appstate.AppState)

	SetCommitteeRewardShare(amount *big.Int)

	BeginApplyingTx(tx *types.Transaction, appState *appstate.AppState)
	CompleteApplyingTx(appState *appstate.AppState)
	AddTxFee(feeAmount *big.Int)

	AddFactEvidenceContractDeploy(contractAddress common.Address)
	AddTxReceipt(txReceipt *types.TxReceipt)
}

type collectorStub struct {
}

func NewStatsCollector() StatsCollector {
	return &collectorStub{}
}

func (c *collectorStub) EnableCollecting() {
	// do nothing
}

func (c *collectorStub) SetValidation(validation *statsTypes.ValidationStats) {
	// do nothing
}

func SetValidation(c StatsCollector, validation *statsTypes.ValidationStats) {
	if c == nil {
		return
	}
	c.SetValidation(validation)
}

func (c *collectorStub) SetMinScoreForInvite(score float32) {
	// do nothing
}

func SetMinScoreForInvite(c StatsCollector, score float32) {
	if c == nil {
		return
	}
	c.SetMinScoreForInvite(score)
}

func (c *collectorStub) SetValidationResults(validationResults *types.ValidationResults) {
	// do nothing
}

func SetValidationResults(c StatsCollector, validationResults *types.ValidationResults) {
	if c == nil {
		return
	}
	c.SetValidationResults(validationResults)
}

func (c *collectorStub) SetTotalReward(amount *big.Int) {
	// do nothing
}

func SetTotalReward(c StatsCollector, amount *big.Int) {
	if c == nil {
		return
	}
	c.SetTotalReward(amount)
}

func (c *collectorStub) SetTotalValidationReward(amount *big.Int, share *big.Int) {
	// do nothing
}

func SetTotalValidationReward(c StatsCollector, amount *big.Int, share *big.Int) {
	if c == nil {
		return
	}
	c.SetTotalValidationReward(amount, share)
}

func (c *collectorStub) SetTotalFlipsReward(amount *big.Int, share *big.Int) {
	// do nothing
}

func SetTotalFlipsReward(c StatsCollector, amount *big.Int, share *big.Int) {
	if c == nil {
		return
	}
	c.SetTotalFlipsReward(amount, share)
}

func (c *collectorStub) SetTotalInvitationsReward(amount *big.Int, share *big.Int) {
	// do nothing
}

func SetTotalInvitationsReward(c StatsCollector, amount *big.Int, share *big.Int) {
	if c == nil {
		return
	}
	c.SetTotalInvitationsReward(amount, share)
}

func (c *collectorStub) SetTotalFoundationPayouts(amount *big.Int) {
	// do nothing
}

func SetTotalFoundationPayouts(c StatsCollector, amount *big.Int) {
	if c == nil {
		return
	}
	c.SetTotalFoundationPayouts(amount)
}

func (c *collectorStub) SetTotalZeroWalletFund(amount *big.Int) {
	// do nothing
}

func SetTotalZeroWalletFund(c StatsCollector, amount *big.Int) {
	if c == nil {
		return
	}
	c.SetTotalZeroWalletFund(amount)
}

func (c *collectorStub) AddValidationReward(addr common.Address, age uint16, balance *big.Int, stake *big.Int) {
	// do nothing
}

func AddValidationReward(c StatsCollector, addr common.Address, age uint16, balance *big.Int, stake *big.Int) {
	if c == nil {
		return
	}
	c.AddValidationReward(addr, age, balance, stake)
}

func (c *collectorStub) AddFlipsReward(addr common.Address, balance *big.Int, stake *big.Int,
	flipsToReward []*types.FlipToReward) {
	// do nothing
}

func AddFlipsReward(c StatsCollector, addr common.Address, balance *big.Int, stake *big.Int,
	flipsToReward []*types.FlipToReward) {
	if c == nil {
		return
	}
	c.AddFlipsReward(addr, balance, stake, flipsToReward)
}

func (c *collectorStub) AddReportedFlipsReward(addr common.Address, flipIdx int, balance *big.Int, stake *big.Int) {
	// do nothing
}

func AddReportedFlipsReward(c StatsCollector, addr common.Address, flipIdx int, balance *big.Int, stake *big.Int) {
	if c == nil {
		return
	}
	c.AddReportedFlipsReward(addr, flipIdx, balance, stake)
}

func (c *collectorStub) AddInvitationsReward(addr common.Address, balance *big.Int, stake *big.Int, age uint16,
	txHash *common.Hash, isSavedInviteWinner bool) {
	// do nothing
}

func AddInvitationsReward(c StatsCollector, addr common.Address, balance *big.Int, stake *big.Int, age uint16,
	txHash *common.Hash, isSavedInviteWinner bool) {
	if c == nil {
		return
	}
	c.AddInvitationsReward(addr, balance, stake, age, txHash, isSavedInviteWinner)
}

func (c *collectorStub) AddFoundationPayout(addr common.Address, balance *big.Int) {
	// do nothing
}

func AddFoundationPayout(c StatsCollector, addr common.Address, balance *big.Int) {
	if c == nil {
		return
	}
	c.AddFoundationPayout(addr, balance)
}

func (c *collectorStub) AddZeroWalletFund(addr common.Address, balance *big.Int) {
	// do nothing
}

func AddZeroWalletFund(c StatsCollector, addr common.Address, balance *big.Int) {
	if c == nil {
		return
	}
	c.AddZeroWalletFund(addr, balance)
}

func (c *collectorStub) AddProposerReward(addr common.Address, balance *big.Int, stake *big.Int) {
	// do nothing
}

func AddProposerReward(c StatsCollector, addr common.Address, balance *big.Int, stake *big.Int) {
	if c == nil {
		return
	}
	c.AddProposerReward(addr, balance, stake)
}

func (c *collectorStub) AddFinalCommitteeReward(addr common.Address, balance *big.Int, stake *big.Int) {
	// do nothing
}

func AddFinalCommitteeReward(c StatsCollector, addr common.Address, balance *big.Int, stake *big.Int) {
	if c == nil {
		return
	}
	c.AddFinalCommitteeReward(addr, balance, stake)
}

func (c *collectorStub) CompleteCollecting() {
	// do nothing
}

func (c *collectorStub) AfterSubPenalty(addr common.Address, amount *big.Int, appState *appstate.AppState) {
	// do nothing
}

func AfterSubPenalty(c StatsCollector, addr common.Address, amount *big.Int, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.AfterSubPenalty(addr, amount, appState)
}

func (c *collectorStub) BeforeClearPenalty(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func BeforeClearPenalty(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeforeClearPenalty(addr, appState)
}

func (c *collectorStub) BeforeSetPenalty(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func BeforeSetPenalty(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeforeSetPenalty(addr, appState)
}

func (c *collectorStub) AddMintedCoins(amount *big.Int) {
	// do nothing
}

func AddMintedCoins(c StatsCollector, amount *big.Int) {
	if c == nil {
		return
	}
	c.AddMintedCoins(amount)
}

func (c *collectorStub) AddPenaltyBurntCoins(addr common.Address, amount *big.Int) {
	// do nothing
}

func AddPenaltyBurntCoins(c StatsCollector, addr common.Address, amount *big.Int) {
	if c == nil {
		return
	}
	c.AddPenaltyBurntCoins(addr, amount)
}

func (c *collectorStub) AddInviteBurntCoins(addr common.Address, amount *big.Int, tx *types.Transaction) {
	// do nothing
}

func AddInviteBurntCoins(c StatsCollector, addr common.Address, amount *big.Int, tx *types.Transaction) {
	if c == nil {
		return
	}
	c.AddInviteBurntCoins(addr, amount, tx)
}

func (c *collectorStub) AddFeeBurntCoins(addr common.Address, feeAmount *big.Int, burntRate float32,
	tx *types.Transaction) {
	// do nothing
}

func AddFeeBurntCoins(c StatsCollector, addr common.Address, feeAmount *big.Int, burntRate float32,
	tx *types.Transaction) {
	if c == nil {
		return
	}
	c.AddFeeBurntCoins(addr, feeAmount, burntRate, tx)
}

func (c *collectorStub) AddKilledBurntCoins(addr common.Address, amount *big.Int) {
	// do nothing
}

func AddKilledBurntCoins(c StatsCollector, addr common.Address, amount *big.Int) {
	if c == nil {
		return
	}
	c.AddKilledBurntCoins(addr, amount)
}

func (c *collectorStub) AddBurnTxBurntCoins(addr common.Address, tx *types.Transaction) {
	// do nothing
}

func AddBurnTxBurntCoins(c StatsCollector, addr common.Address, tx *types.Transaction) {
	if c == nil {
		return
	}
	c.AddBurnTxBurntCoins(addr, tx)
}

func (c *collectorStub) AfterAddStake(addr common.Address, amount *big.Int, appState *appstate.AppState) {
	// do nothing
}

func AfterAddStake(c StatsCollector, addr common.Address, amount *big.Int, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.AfterAddStake(addr, amount, appState)
}

func (c *collectorStub) AddActivationTxBalanceTransfer(tx *types.Transaction, amount *big.Int) {
	// do nothing
}

func AddActivationTxBalanceTransfer(c StatsCollector, tx *types.Transaction, amount *big.Int) {
	if c == nil {
		return
	}
	c.AddActivationTxBalanceTransfer(tx, amount)
}

func (c *collectorStub) AddKillTxStakeTransfer(tx *types.Transaction, amount *big.Int) {
	// do nothing
}

func AddKillTxStakeTransfer(c StatsCollector, tx *types.Transaction, amount *big.Int) {
	if c == nil {
		return
	}
	c.AddKillTxStakeTransfer(tx, amount)
}

func (c *collectorStub) AddKillInviteeTxStakeTransfer(tx *types.Transaction, amount *big.Int) {
	// do nothing
}

func AddKillInviteeTxStakeTransfer(c StatsCollector, tx *types.Transaction, amount *big.Int) {
	if c == nil {
		return
	}
	c.AddKillInviteeTxStakeTransfer(tx, amount)
}

func (c *collectorStub) BeginVerifiedStakeTransferBalanceUpdate(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func BeginVerifiedStakeTransferBalanceUpdate(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeginVerifiedStakeTransferBalanceUpdate(addr, appState)
}

func (c *collectorStub) BeginTxBalanceUpdate(tx *types.Transaction, appState *appstate.AppState) {
	// do nothing
}

func BeginTxBalanceUpdate(c StatsCollector, tx *types.Transaction, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeginTxBalanceUpdate(tx, appState)
}

func (c *collectorStub) BeginProposerRewardBalanceUpdate(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func BeginProposerRewardBalanceUpdate(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeginProposerRewardBalanceUpdate(addr, appState)
}

func (c *collectorStub) BeginCommitteeRewardBalanceUpdate(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func BeginCommitteeRewardBalanceUpdate(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeginCommitteeRewardBalanceUpdate(addr, appState)
}

func (c *collectorStub) BeginEpochRewardBalanceUpdate(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func BeginEpochRewardBalanceUpdate(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeginEpochRewardBalanceUpdate(addr, appState)
}

func (c *collectorStub) BeginFailedValidationBalanceUpdate(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func BeginFailedValidationBalanceUpdate(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeginFailedValidationBalanceUpdate(addr, appState)
}

func (c *collectorStub) BeginPenaltyBalanceUpdate(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func BeginPenaltyBalanceUpdate(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeginPenaltyBalanceUpdate(addr, appState)
}

func (c *collectorStub) BeginEpochPenaltyResetBalanceUpdate(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func BeginEpochPenaltyResetBalanceUpdate(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeginEpochPenaltyResetBalanceUpdate(addr, appState)
}

func (c *collectorStub) BeginDustClearingBalanceUpdate(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func BeginDustClearingBalanceUpdate(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeginDustClearingBalanceUpdate(addr, appState)
}

func (c *collectorStub) CompleteBalanceUpdate(appState *appstate.AppState) {
	// do nothing
}

func CompleteBalanceUpdate(c StatsCollector, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.CompleteBalanceUpdate(appState)
}

func (c *collectorStub) SetCommitteeRewardShare(amount *big.Int) {
	// do nothing
}

func SetCommitteeRewardShare(c StatsCollector, amount *big.Int) {
	if c == nil {
		return
	}
	c.SetCommitteeRewardShare(amount)
}

func (c *collectorStub) BeginApplyingTx(tx *types.Transaction, appState *appstate.AppState) {
	// do nothing
}

func BeginApplyingTx(c StatsCollector, tx *types.Transaction, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.BeginApplyingTx(tx, appState)
}

func (c *collectorStub) CompleteApplyingTx(appState *appstate.AppState) {
	// do nothing
}

func CompleteApplyingTx(c StatsCollector, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.CompleteApplyingTx(appState)
}

func (c *collectorStub) AddTxFee(feeAmount *big.Int) {
	// do nothing
}

func AddTxFee(c StatsCollector, feeAmount *big.Int) {
	if c == nil {
		return
	}
	c.AddTxFee(feeAmount)
}

func (c *collectorStub) AddFactEvidenceContractDeploy(contractAddress common.Address) {
	// do nothing
}

func AddFactEvidenceContractDeploy(c StatsCollector, contractAddress common.Address) {
	if c == nil {
		return
	}
	c.AddFactEvidenceContractDeploy(contractAddress)
}

func (c *collectorStub) AddTxReceipt(txReceipt *types.TxReceipt) {
	// do nothing
}

func AddTxReceipt(c StatsCollector, txReceipt *types.TxReceipt) {
	if c == nil {
		return
	}
	c.AddTxReceipt(txReceipt)
}
