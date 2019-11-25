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

	SetAuthors(authors *types.ValidationAuthors)

	SetTotalReward(amount *big.Int)
	SetTotalValidationReward(amount *big.Int)
	SetTotalFlipsReward(amount *big.Int)
	SetTotalInvitationsReward(amount *big.Int)
	SetTotalFoundationPayouts(amount *big.Int)
	SetTotalZeroWalletFund(amount *big.Int)
	AddValidationReward(addr common.Address, age uint16, balance *big.Int, stake *big.Int)
	AddFlipsReward(addr common.Address, balance *big.Int, stake *big.Int)
	AddInvitationsReward(addr common.Address, balance *big.Int, stake *big.Int)
	AddFoundationPayout(addr common.Address, balance *big.Int)
	AddZeroWalletFund(addr common.Address, balance *big.Int)

	AddProposerReward(addr common.Address, balance *big.Int, stake *big.Int)
	AddFinalCommitteeReward(addr common.Address, balance *big.Int, stake *big.Int)

	AfterSubPenalty(addr common.Address, amount *big.Int, appState *appstate.AppState)
	BeforeClearPenalty(addr common.Address, appState *appstate.AppState)
	BeforeSetPenalty(addr common.Address, appState *appstate.AppState)

	AfterBalanceUpdate(addr common.Address, appState *appstate.AppState)

	AddMintedCoins(amount *big.Int)
	AddPenaltyBurntCoins(addr common.Address, amount *big.Int)
	AddInviteBurntCoins(addr common.Address, amount *big.Int, tx *types.Transaction)
	AddFeeBurntCoins(addr common.Address, feeAmount *big.Int, burntRate float32, tx *types.Transaction)
	AddKilledBurntCoins(addr common.Address, amount *big.Int)
	AddBurnTxBurntCoins(addr common.Address, tx *types.Transaction)

	AfterKillIdentity(addr common.Address, appState *appstate.AppState)
	AfterAddStake(addr common.Address, amount *big.Int)
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

func (c *collectorStub) SetAuthors(authors *types.ValidationAuthors) {
	// do nothing
}

func SetAuthors(c StatsCollector, authors *types.ValidationAuthors) {
	if c == nil {
		return
	}
	c.SetAuthors(authors)
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

func (c *collectorStub) SetTotalValidationReward(amount *big.Int) {
	// do nothing
}

func SetTotalValidationReward(c StatsCollector, amount *big.Int) {
	if c == nil {
		return
	}
	c.SetTotalValidationReward(amount)
}

func (c *collectorStub) SetTotalFlipsReward(amount *big.Int) {
	// do nothing
}

func SetTotalFlipsReward(c StatsCollector, amount *big.Int) {
	if c == nil {
		return
	}
	c.SetTotalFlipsReward(amount)
}

func (c *collectorStub) SetTotalInvitationsReward(amount *big.Int) {
	// do nothing
}

func SetTotalInvitationsReward(c StatsCollector, amount *big.Int) {
	if c == nil {
		return
	}
	c.SetTotalInvitationsReward(amount)
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

func (c *collectorStub) AddFlipsReward(addr common.Address, balance *big.Int, stake *big.Int) {
	// do nothing
}

func AddFlipsReward(c StatsCollector, addr common.Address, balance *big.Int, stake *big.Int) {
	if c == nil {
		return
	}
	c.AddFlipsReward(addr, balance, stake)
}

func (c *collectorStub) AddInvitationsReward(addr common.Address, balance *big.Int, stake *big.Int) {
	// do nothing
}

func AddInvitationsReward(c StatsCollector, addr common.Address, balance *big.Int, stake *big.Int) {
	if c == nil {
		return
	}
	c.AddInvitationsReward(addr, balance, stake)
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

func (c *collectorStub) AfterBalanceUpdate(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func AfterBalanceUpdate(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.AfterBalanceUpdate(addr, appState)
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

func (c *collectorStub) AfterKillIdentity(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func AfterKillIdentity(c StatsCollector, addr common.Address, appState *appstate.AppState) {
	if c == nil {
		return
	}
	c.AfterKillIdentity(addr, appState)
}

func (c *collectorStub) AfterAddStake(addr common.Address, amount *big.Int) {
	// do nothing
}

func AfterAddStake(c StatsCollector, addr common.Address, amount *big.Int) {
	if c == nil {
		return
	}
	c.AfterAddStake(addr, amount)
}
