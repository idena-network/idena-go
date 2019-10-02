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

func rewardValidIdentities(appState *appstate.AppState, config *config.ConsensusConf, authors *types.ValidationAuthors, blocks uint64, collector collector.BlockStatsCollector) {
	totalReward := big.NewInt(0).Add(config.BlockReward, config.FinalCommitteeReward)
	totalReward = totalReward.Mul(totalReward, big.NewInt(int64(blocks)))

	collector.SetAuthors(authors)
	collector.SetTotalReward(totalReward)

	log.Info("Total validation reward", "reward", ConvertToFloat(totalReward).String())

	totalRewardD := decimal.NewFromBigInt(totalReward, 0)
	addSuccessfulValidationReward(appState, config, authors, totalRewardD, collector)
	addFlipReward(appState, config, authors, totalRewardD, collector)
	addInvitationReward(appState, config, authors, totalRewardD, collector)
	addFoundationPayouts(appState, config, totalRewardD, collector)
	addZeroWalletFund(appState, config, totalRewardD, collector)
}

func addSuccessfulValidationReward(appState *appstate.AppState, config *config.ConsensusConf,
	authors *types.ValidationAuthors, totalReward decimal.Decimal, collector collector.BlockStatsCollector) {
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

	collector.SetTotalValidationReward(math.ToInt(successfulValidationRewardD))
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
				collector.AddMintedCoins(reward)
				collector.AddMintedCoins(stake)
				collector.AddValidationReward(addr, age, reward, stake)
			}
		}
	})
}

func addFlipReward(appState *appstate.AppState, config *config.ConsensusConf, authors *types.ValidationAuthors,
	totalReward decimal.Decimal, collector collector.BlockStatsCollector) {
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
	collector.SetTotalFlipsReward(math.ToInt(flipRewardD))
	flipRewardShare := flipRewardD.Div(decimal.NewFromFloat32(totalFlips))

	for addr, author := range authors.GoodAuthors {
		if author.Missed {
			continue
		}
		totalReward := flipRewardShare.Mul(decimal.NewFromFloat32(float32(author.StrongFlips + author.WeakFlips)))
		reward, stake := splitReward(math.ToInt(totalReward), config)
		appState.State.AddBalance(addr, reward)
		appState.State.AddStake(addr, stake)
		collector.AddMintedCoins(reward)
		collector.AddMintedCoins(stake)
		collector.AddFlipsReward(addr, reward, stake)
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
	totalReward decimal.Decimal, collector collector.BlockStatsCollector) {
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
	collector.SetTotalInvitationsReward(math.ToInt(invitationRewardD))
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
				collector.AddMintedCoins(reward)
				collector.AddMintedCoins(stake)
				collector.AddInvitationsReward(addr, reward, stake)
			}
		}
	}
}

func addFoundationPayouts(appState *appstate.AppState, config *config.ConsensusConf, totalReward decimal.Decimal,
	collector collector.BlockStatsCollector) {
	payout := totalReward.Mul(decimal.NewFromFloat32(config.FoundationPayoutsPercent))
	total := math.ToInt(payout)
	godAddress := appState.State.GodAddress()
	appState.State.AddBalance(godAddress, total)
	collector.AddMintedCoins(total)
	collector.SetTotalFoundationPayouts(total)
	collector.AddFoundationPayout(godAddress, total)
}

func addZeroWalletFund(appState *appstate.AppState, config *config.ConsensusConf, totalReward decimal.Decimal,
	collector collector.BlockStatsCollector) {
	payout := totalReward.Mul(decimal.NewFromFloat32(config.ZeroWalletPercent))
	total := math.ToInt(payout)
	zeroAddress := common.Address{}
	appState.State.AddBalance(zeroAddress, total)
	collector.AddMintedCoins(total)
	collector.SetTotalZeroWalletFund(total)
	collector.AddZeroWalletFund(zeroAddress, total)
}

func normalAge(age uint16) float32 {
	return float32(math2.Pow(float64(age)+1, float64(1)/3))
}
