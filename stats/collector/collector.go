package collector

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	statsTypes "github.com/idena-network/idena-go/stats/types"
	"math/big"
)

type BlockStatsCollector interface {
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
}

type collectorStub struct {
}

func NewBlockStatsCollector() BlockStatsCollector {
	return &collectorStub{}
}

func (c *collectorStub) EnableCollecting() {
	// do nothing
}

func (c *collectorStub) SetValidation(validation *statsTypes.ValidationStats) {
	// do nothing
}

func (c *collectorStub) SetAuthors(authors *types.ValidationAuthors) {
	// do nothing
}

func (c *collectorStub) SetTotalReward(amount *big.Int) {
	// do nothing
}

func (c *collectorStub) SetTotalValidationReward(amount *big.Int) {
	// do nothing
}

func (c *collectorStub) SetTotalFlipsReward(amount *big.Int) {
	// do nothing
}

func (c *collectorStub) SetTotalInvitationsReward(amount *big.Int) {
	// do nothing
}

func (c *collectorStub) SetTotalFoundationPayouts(amount *big.Int) {
	// do nothing
}

func (c *collectorStub) SetTotalZeroWalletFund(amount *big.Int) {
	// do nothing
}

func (c *collectorStub) AddValidationReward(addr common.Address, age uint16, balance *big.Int, stake *big.Int) {
	// do nothing
}

func (c *collectorStub) AddFlipsReward(addr common.Address, balance *big.Int, stake *big.Int) {
	// do nothing
}

func (c *collectorStub) AddInvitationsReward(addr common.Address, balance *big.Int, stake *big.Int) {
	// do nothing
}

func (c *collectorStub) AddFoundationPayout(addr common.Address, balance *big.Int) {
	// do nothing
}

func (c *collectorStub) AddZeroWalletFund(addr common.Address, balance *big.Int) {
	// do nothing
}

func (c *collectorStub) AddProposerReward(addr common.Address, balance *big.Int, stake *big.Int) {
	// do nothing
}

func (c *collectorStub) AddFinalCommitteeReward(addr common.Address, balance *big.Int, stake *big.Int) {
	// do nothing
}

func (c *collectorStub) CompleteCollecting() {
	// do nothing
}

func (c *collectorStub) AfterSubPenalty(addr common.Address, amount *big.Int, appState *appstate.AppState) {
	// do nothing
}

func (c *collectorStub) BeforeClearPenalty(addr common.Address, appState *appstate.AppState) {
	// do nothing
}

func (c *collectorStub) BeforeSetPenalty(addr common.Address, appState *appstate.AppState) {
	// do nothing
}
