package blockchain

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/shopspring/decimal"
	math2 "math"
	"math/big"
)

func rewardValidIdentities(appState *appstate.AppState, config *config.ConsensusConf, authors *types.ValidationAuthors,
	blocks uint64, statsCollector collector.StatsCollector) {

	totalReward := big.NewInt(0).Add(config.BlockReward, config.FinalCommitteeReward)
	totalReward = totalReward.Mul(totalReward, big.NewInt(int64(blocks)))

	collector.SetAuthors(statsCollector, authors)
	collector.SetTotalReward(statsCollector, totalReward)

	log.Info("Total validation reward", "reward", ConvertToFloat(totalReward).String())

	totalRewardD := decimal.NewFromBigInt(totalReward, 0)
	addSuccessfulValidationReward(appState, config, authors, totalRewardD, statsCollector)
	addFlipReward(appState, config, authors, totalRewardD, statsCollector)
	addInvitationReward(appState, config, authors, totalRewardD, statsCollector)
	addFoundationPayouts(appState, config, totalRewardD, statsCollector)
	addZeroWalletFund(appState, config, totalRewardD, statsCollector)
}

func addSuccessfulValidationReward(appState *appstate.AppState, config *config.ConsensusConf,
	authors *types.ValidationAuthors, totalReward decimal.Decimal, statsCollector collector.StatsCollector) {
	successfulValidationRewardD := totalReward.Mul(decimal.NewFromFloat32(config.SuccessfulValidationRewardPercent))

	epoch := appState.State.Epoch()

	normalizedAges := float32(0)
	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		switch identity.State {
		case state.Verified, state.Newbie:
			if _, ok := authors.BadAuthors[addr]; !ok {
				normalizedAges += normalAge(epoch - identity.Birthday)
			}
		}
	})

	if normalizedAges == 0 {
		return
	}

	collector.SetTotalValidationReward(statsCollector, math.ToInt(successfulValidationRewardD))
	successfulValidationRewardShare := successfulValidationRewardD.Div(decimal.NewFromFloat32(normalizedAges))

	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		switch identity.State {
		case state.Verified, state.Newbie:
			if _, ok := authors.BadAuthors[addr]; !ok {
				age := epoch - identity.Birthday
				normalAge := normalAge(age)
				totalReward := successfulValidationRewardShare.Mul(decimal.NewFromFloat32(normalAge))
				reward, stake := splitReward(math.ToInt(totalReward), config)
				appState.State.AddBalance(addr, reward)
				appState.State.AddStake(addr, stake)
				collector.AfterBalanceUpdate(statsCollector, addr, appState)
				collector.AddMintedCoins(statsCollector, reward)
				collector.AddMintedCoins(statsCollector, stake)
				collector.AddValidationReward(statsCollector, addr, age, reward, stake)
				collector.AfterAddStake(statsCollector, addr, stake)
			}
		}
	})
}

func addFlipReward(appState *appstate.AppState, config *config.ConsensusConf, authors *types.ValidationAuthors,
	totalReward decimal.Decimal, statsCollector collector.StatsCollector) {
	flipRewardD := totalReward.Mul(decimal.NewFromFloat32(config.FlipRewardPercent))

	totalFlips := float32(0)
	for _, author := range authors.GoodAuthors {
		if author.Missed {
			continue
		}
		totalFlips += float32(author.WeakFlips + author.StrongFlips)
	}
	if totalFlips == 0 {
		return
	}
	collector.SetTotalFlipsReward(statsCollector, math.ToInt(flipRewardD))
	flipRewardShare := flipRewardD.Div(decimal.NewFromFloat32(totalFlips))

	for addr, author := range authors.GoodAuthors {
		if author.Missed {
			continue
		}
		totalReward := flipRewardShare.Mul(decimal.NewFromFloat32(float32(author.StrongFlips + author.WeakFlips)))
		reward, stake := splitReward(math.ToInt(totalReward), config)
		appState.State.AddBalance(addr, reward)
		appState.State.AddStake(addr, stake)
		collector.AfterBalanceUpdate(statsCollector, addr, appState)
		collector.AddMintedCoins(statsCollector, reward)
		collector.AddMintedCoins(statsCollector, stake)
		collector.AddFlipsReward(statsCollector, addr, reward, stake)
		collector.AfterAddStake(statsCollector, addr, stake)
	}
}

func getInvitationRewardCoef(age uint16, config *config.ConsensusConf) float32 {
	switch age {
	case 1:
		return config.FirstInvitationRewardCoef
	case 2:
		return config.SecondInvitationRewardCoef
	case 3:
		return config.ThirdInvitationRewardCoef
	default:
		return 0
	}
}

func addInvitationReward(appState *appstate.AppState, config *config.ConsensusConf, authors *types.ValidationAuthors,
	totalReward decimal.Decimal, statsCollector collector.StatsCollector) {
	invitationRewardD := totalReward.Mul(decimal.NewFromFloat32(config.ValidInvitationRewardPercent))

	totalWeight := float32(0)
	for _, author := range authors.GoodAuthors {
		if !author.Validated {
			continue
		}
		for _, successfulInviteAge := range author.SuccessfulInviteAges {
			totalWeight += getInvitationRewardCoef(successfulInviteAge, config)
		}
	}
	if totalWeight == 0 {
		return
	}
	collector.SetTotalInvitationsReward(statsCollector, math.ToInt(invitationRewardD))
	invitationRewardShare := invitationRewardD.Div(decimal.NewFromFloat32(totalWeight))

	for addr, author := range authors.GoodAuthors {
		if !author.Validated {
			continue
		}
		for _, successfulInviteAge := range author.SuccessfulInviteAges {
			if weight := getInvitationRewardCoef(successfulInviteAge, config); weight > 0 {
				totalReward := invitationRewardShare.Mul(decimal.NewFromFloat32(weight))
				reward, stake := splitReward(math.ToInt(totalReward), config)
				appState.State.AddBalance(addr, reward)
				appState.State.AddStake(addr, stake)
				collector.AfterBalanceUpdate(statsCollector, addr, appState)
				collector.AddMintedCoins(statsCollector, reward)
				collector.AddMintedCoins(statsCollector, stake)
				collector.AddInvitationsReward(statsCollector, addr, reward, stake)
				collector.AfterAddStake(statsCollector, addr, stake)
			}
		}
	}
}

func addFoundationPayouts(appState *appstate.AppState, config *config.ConsensusConf, totalReward decimal.Decimal,
	statsCollector collector.StatsCollector) {
	payout := totalReward.Mul(decimal.NewFromFloat32(config.FoundationPayoutsPercent))
	total := math.ToInt(payout)
	godAddress := appState.State.GodAddress()
	appState.State.AddBalance(godAddress, total)
	collector.AfterBalanceUpdate(statsCollector, godAddress, appState)
	collector.AddMintedCoins(statsCollector, total)
	collector.SetTotalFoundationPayouts(statsCollector, total)
	collector.AddFoundationPayout(statsCollector, godAddress, total)
}

func addZeroWalletFund(appState *appstate.AppState, config *config.ConsensusConf, totalReward decimal.Decimal,
	statsCollector collector.StatsCollector) {
	payout := totalReward.Mul(decimal.NewFromFloat32(config.ZeroWalletPercent))
	total := math.ToInt(payout)
	zeroAddress := common.Address{}
	appState.State.AddBalance(zeroAddress, total)
	collector.AfterBalanceUpdate(statsCollector, zeroAddress, appState)
	collector.AddMintedCoins(statsCollector, total)
	collector.SetTotalZeroWalletFund(statsCollector, total)
	collector.AddZeroWalletFund(statsCollector, zeroAddress, total)
}

func normalAge(age uint16) float32 {
	return float32(math2.Pow(float64(age)+1, float64(1)/3))
}
