package blockchain

import (
	"bytes"
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
	"sort"
)

func rewardValidIdentities(appState *appstate.AppState, config *config.ConsensusConf, validationResults map[common.ShardId]*types.ValidationResults,
	epochDurations []uint32, seed types.Seed, statsCollector collector.StatsCollector) {

	totalReward := big.NewInt(0).Add(config.BlockReward, config.FinalCommitteeReward)
	currentEpochDuration := epochDurations[len(epochDurations)-1]
	totalReward = totalReward.Mul(totalReward, big.NewInt(int64(currentEpochDuration)))
	collector.SetValidationResults(statsCollector, validationResults)
	collector.SetTotalReward(statsCollector, totalReward)

	log.Info("Total validation reward", "reward", ConvertToFloat(totalReward).String())

	totalRewardD := decimal.NewFromBigInt(totalReward, 0)
	addSuccessfulValidationReward(appState, config, validationResults, totalRewardD, statsCollector)
	addFlipReward(appState, config, validationResults, totalRewardD, statsCollector)
	addReportReward(appState, config, validationResults, totalRewardD, statsCollector)
	addInvitationReward(appState, config, validationResults, totalRewardD, seed, epochDurations, statsCollector)
	addFoundationPayouts(appState, config, totalRewardD, statsCollector)
	addZeroWalletFund(appState, config, totalRewardD, statsCollector)
}

func addSuccessfulValidationReward(appState *appstate.AppState, config *config.ConsensusConf,
	validationResults map[common.ShardId]*types.ValidationResults, totalReward decimal.Decimal, statsCollector collector.StatsCollector) {
	successfulValidationRewardD := totalReward.Mul(decimal.NewFromFloat32(config.SuccessfulValidationRewardPercent))

	epoch := appState.State.Epoch()

	normalizedAges := float32(0)
	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		if identity.State.NewbieOrBetter() {
			if _, ok := validationResults[identity.ShiftedShardId()].BadAuthors[addr]; !ok {
				normalizedAges += normalAge(epoch - identity.Birthday)
			}
		}
	})

	if normalizedAges == 0 {
		return
	}

	successfulValidationRewardShare := successfulValidationRewardD.Div(decimal.NewFromFloat32(normalizedAges))
	collector.SetTotalValidationReward(statsCollector, math.ToInt(successfulValidationRewardD),
		math.ToInt(successfulValidationRewardShare))

	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		if identity.State.NewbieOrBetter() {
			if _, ok := validationResults[identity.ShiftedShardId()].BadAuthors[addr]; !ok {
				age := epoch - identity.Birthday
				normalAge := normalAge(age)
				totalReward := successfulValidationRewardShare.Mul(decimal.NewFromFloat32(normalAge))
				reward, stake := splitReward(math.ToInt(totalReward), identity.State == state.Newbie, config)
				rewardDest := addr
				if identity.Delegatee != nil {
					rewardDest = *identity.Delegatee
				}
				collector.BeginEpochRewardBalanceUpdate(statsCollector, rewardDest, addr, appState)
				appState.State.AddBalance(rewardDest, reward)
				appState.State.AddStake(addr, stake)
				collector.CompleteBalanceUpdate(statsCollector, appState)
				collector.AddMintedCoins(statsCollector, reward)
				collector.AddMintedCoins(statsCollector, stake)
				collector.AddValidationReward(statsCollector, rewardDest, addr, age, reward, stake)
				collector.AfterAddStake(statsCollector, addr, stake, appState)
			}
		}
	})
}

func getFlipRewardCoef(grade types.Grade) float32 {
	switch grade {
	case types.GradeD:
		return 1
	case types.GradeC:
		return 2
	case types.GradeB:
		return 4
	case types.GradeA:
		return 8
	default:
		return 0
	}
}

func addFlipReward(appState *appstate.AppState, config *config.ConsensusConf, validationResults map[common.ShardId]*types.ValidationResults,
	totalReward decimal.Decimal, statsCollector collector.StatsCollector) {
	flipRewardD := totalReward.Mul(decimal.NewFromFloat32(config.FlipRewardPercent))

	totalWeight := float32(0)

	for i := uint32(1); i <= appState.State.ShardsNum(); i++ {
		validationResult, ok := validationResults[common.ShardId(i)]
		if !ok {
			continue
		}
		for _, author := range validationResult.GoodAuthors {
			if author.Missed {
				continue
			}
			for _, f := range author.FlipsToReward {
				totalWeight += getFlipRewardCoef(f.Grade)
			}
		}
		if config.ReportsRewardPercent > 0 {
			continue
		}
		for _, reporters := range validationResult.ReportersToRewardByFlip {
			if len(reporters) == 0 {
				continue
			}
			totalWeight += 1
		}
	}

	if totalWeight == 0 {
		return
	}
	flipRewardShare := flipRewardD.Div(decimal.NewFromFloat32(totalWeight))
	collector.SetTotalFlipsReward(statsCollector, math.ToInt(flipRewardD), math.ToInt(flipRewardShare))

	for i := uint32(1); i <= appState.State.ShardsNum(); i++ {
		validationResult, ok := validationResults[common.ShardId(i)]
		if !ok {
			continue
		}

		for addr, author := range validationResult.GoodAuthors {
			if author.Missed {
				continue
			}
			var weight float32
			for _, f := range author.FlipsToReward {
				weight += getFlipRewardCoef(f.Grade)
			}
			totalReward := flipRewardShare.Mul(decimal.NewFromFloat32(weight))
			reward, stake := splitReward(math.ToInt(totalReward), author.NewIdentityState == uint8(state.Newbie), config)
			rewardDest := addr
			if delegatee := appState.State.Delegatee(addr); delegatee != nil {
				rewardDest = *delegatee
			}
			collector.BeginEpochRewardBalanceUpdate(statsCollector, rewardDest, addr, appState)
			appState.State.AddBalance(rewardDest, reward)
			appState.State.AddStake(addr, stake)
			collector.CompleteBalanceUpdate(statsCollector, appState)
			collector.AddMintedCoins(statsCollector, reward)
			collector.AddMintedCoins(statsCollector, stake)
			collector.AddFlipsReward(statsCollector, rewardDest, addr, reward, stake, author.FlipsToReward)
			collector.AfterAddStake(statsCollector, addr, stake, appState)
		}
	}
	if config.ReportsRewardPercent > 0 {
		return
	}
	for i := uint32(1); i <= appState.State.ShardsNum(); i++ {
		shardId := common.ShardId(i)
		validationResult, ok := validationResults[shardId]
		if !ok {
			continue
		}
		for flipIdx, reporters := range validationResult.ReportersToRewardByFlip {
			if len(reporters) == 0 {
				continue
			}
			totalReward := flipRewardShare.Div(decimal.NewFromInt(int64(len(reporters))))
			for _, reporter := range reporters {
				reward, stake := splitReward(math.ToInt(totalReward), reporter.NewIdentityState == uint8(state.Newbie), config)
				rewardDest := reporter.Address
				if delegatee := appState.State.Delegatee(reporter.Address); delegatee != nil {
					rewardDest = *delegatee
				}
				collector.BeginEpochRewardBalanceUpdate(statsCollector, rewardDest, reporter.Address, appState)
				appState.State.AddBalance(rewardDest, reward)
				appState.State.AddStake(reporter.Address, stake)
				collector.CompleteBalanceUpdate(statsCollector, appState)
				collector.AddMintedCoins(statsCollector, reward)
				collector.AddMintedCoins(statsCollector, stake)
				collector.AddReportedFlipsReward(statsCollector, rewardDest, reporter.Address, shardId, flipIdx, reward, stake)
				collector.AfterAddStake(statsCollector, reporter.Address, stake, appState)
			}
		}
	}
}

func addReportReward(appState *appstate.AppState, config *config.ConsensusConf, validationResults map[common.ShardId]*types.ValidationResults,
	totalReward decimal.Decimal, statsCollector collector.StatsCollector) {
	if config.ReportsRewardPercent == 0 {
		return
	}
	rewardD := totalReward.Mul(decimal.NewFromFloat32(config.ReportsRewardPercent))

	totalWeight := uint64(0)

	for i := uint32(1); i <= appState.State.ShardsNum(); i++ {
		validationResult, ok := validationResults[common.ShardId(i)]
		if !ok {
			continue
		}
		for _, reporters := range validationResult.ReportersToRewardByFlip {
			totalWeight += uint64(len(reporters))
		}
	}

	if totalWeight == 0 {
		return
	}

	rewardShare := rewardD.Div(decimal.NewFromBigInt(new(big.Int).SetUint64(totalWeight), 0))

	collector.SetTotalReportsReward(statsCollector, math.ToInt(rewardD), math.ToInt(rewardShare))

	for i := uint32(1); i <= appState.State.ShardsNum(); i++ {
		shardId := common.ShardId(i)
		validationResult, ok := validationResults[shardId]
		if !ok {
			continue
		}
		for flipIdx, reporters := range validationResult.ReportersToRewardByFlip {
			if len(reporters) == 0 {
				continue
			}
			for _, reporter := range reporters {
				reward, stake := splitReward(math.ToInt(rewardShare), reporter.NewIdentityState == uint8(state.Newbie), config)
				rewardDest := reporter.Address
				if delegatee := appState.State.Delegatee(reporter.Address); delegatee != nil {
					rewardDest = *delegatee
				}
				collector.BeginEpochRewardBalanceUpdate(statsCollector, rewardDest, reporter.Address, appState)
				appState.State.AddBalance(rewardDest, reward)
				appState.State.AddStake(reporter.Address, stake)
				collector.CompleteBalanceUpdate(statsCollector, appState)
				collector.AddMintedCoins(statsCollector, reward)
				collector.AddMintedCoins(statsCollector, stake)
				collector.AddReportedFlipsReward(statsCollector, rewardDest, reporter.Address, shardId, flipIdx, reward, stake)
				collector.AfterAddStake(statsCollector, reporter.Address, stake, appState)
			}
		}
	}
}

func getInvitationRewardCoef(age uint16, epochHeight uint32, epochDurations []uint32, config *config.ConsensusConf) float32 {
	var baseCoef float32
	switch age {
	case 1:
		baseCoef = config.FirstInvitationRewardCoef
	case 2:
		baseCoef = config.SecondInvitationRewardCoef
	case 3:
		baseCoef = config.ThirdInvitationRewardCoef
	default:
		return 0
	}
	if len(epochDurations) < int(age) {
		return baseCoef
	}
	epochDuration := epochDurations[len(epochDurations)-int(age)]
	if epochDuration == 0 {
		return baseCoef
	}
	t := math2.Min(float64(epochHeight)/float64(epochDuration), 1.0)
	return baseCoef * float32(1-math2.Pow(t, 4)*0.5)
}

func addInvitationReward(appState *appstate.AppState, config *config.ConsensusConf, validationResults map[common.ShardId]*types.ValidationResults,
	totalReward decimal.Decimal, seed types.Seed, epochDurations []uint32, statsCollector collector.StatsCollector) {
	invitationRewardD := totalReward.Mul(decimal.NewFromFloat32(config.ValidInvitationRewardPercent))

	totalWeight := float32(0)

	type inviterWrapper struct {
		address common.Address
		inviter *types.InviterValidationResult
	}
	addInviter := func(data []inviterWrapper, elem inviterWrapper) []inviterWrapper {
		index := sort.Search(len(data), func(i int) bool { return bytes.Compare(data[i].address[:], elem.address[:]) > 0 })
		data = append(data, inviterWrapper{})
		copy(data[index+1:], data[index:])
		data[index] = elem
		return data
	}
	goodInviters := make([]inviterWrapper, 0)

	for i := uint32(1); i <= appState.State.ShardsNum(); i++ {
		if shard, ok := validationResults[common.ShardId(i)]; ok {
			for addr, inviter := range shard.GoodInviters {
				goodInviters = addInviter(goodInviters, inviterWrapper{addr, inviter})
			}
		}
	}

	for _, inviterWrapper := range goodInviters {
		inviter := inviterWrapper.inviter
		if !inviter.PayInvitationReward {
			continue
		}
		for _, successfulInvite := range inviter.SuccessfulInvites {
			totalWeight += getInvitationRewardCoef(successfulInvite.Age, successfulInvite.EpochHeight, epochDurations, config)
		}
	}

	if totalWeight == 0 {
		return
	}
	invitationRewardShare := invitationRewardD.Div(decimal.NewFromFloat32(totalWeight))
	collector.SetTotalInvitationsReward(statsCollector, math.ToInt(invitationRewardD), math.ToInt(invitationRewardShare))

	addReward := func(addr common.Address, totalReward decimal.Decimal, isNewbie bool, age uint16, txHash *common.Hash,
		epochHeight uint32, isSavedInviteWinner bool) {
		reward, stake := splitReward(math.ToInt(totalReward), isNewbie, config)
		rewardDest := addr
		if delegatee := appState.State.Delegatee(addr); delegatee != nil {
			rewardDest = *delegatee
		}
		collector.BeginEpochRewardBalanceUpdate(statsCollector, rewardDest, addr, appState)
		appState.State.AddBalance(rewardDest, reward)
		appState.State.AddStake(addr, stake)
		collector.CompleteBalanceUpdate(statsCollector, appState)
		collector.AddMintedCoins(statsCollector, reward)
		collector.AddMintedCoins(statsCollector, stake)
		collector.AddInvitationsReward(statsCollector, rewardDest, addr, reward, stake, age, txHash, epochHeight, isSavedInviteWinner)
		collector.AfterAddStake(statsCollector, addr, stake, appState)
	}

	for _, inviterWrapper := range goodInviters {
		inviter := inviterWrapper.inviter
		addr := inviterWrapper.address
		if !inviter.PayInvitationReward {
			continue
		}
		isNewbie := inviter.NewIdentityState == uint8(state.Newbie)
		for _, successfulInvite := range inviter.SuccessfulInvites {
			if weight := getInvitationRewardCoef(successfulInvite.Age, successfulInvite.EpochHeight, epochDurations, config); weight > 0 {
				totalReward := invitationRewardShare.Mul(decimal.NewFromFloat32(weight))
				addReward(addr, totalReward, isNewbie, successfulInvite.Age, &successfulInvite.TxHash, successfulInvite.EpochHeight, false)
			}
		}
	}
}

func addFoundationPayouts(appState *appstate.AppState, config *config.ConsensusConf, totalReward decimal.Decimal,
	statsCollector collector.StatsCollector) {
	payout := totalReward.Mul(decimal.NewFromFloat32(config.FoundationPayoutsPercent))
	total := math.ToInt(payout)
	godAddress := appState.State.GodAddress()
	collector.BeginEpochRewardBalanceUpdate(statsCollector, godAddress, godAddress, appState)
	appState.State.AddBalance(godAddress, total)
	collector.CompleteBalanceUpdate(statsCollector, appState)
	collector.AddMintedCoins(statsCollector, total)
	collector.SetTotalFoundationPayouts(statsCollector, total)
	collector.AddFoundationPayout(statsCollector, godAddress, total)
}

func addZeroWalletFund(appState *appstate.AppState, config *config.ConsensusConf, totalReward decimal.Decimal,
	statsCollector collector.StatsCollector) {
	payout := totalReward.Mul(decimal.NewFromFloat32(config.ZeroWalletPercent))
	total := math.ToInt(payout)
	zeroAddress := common.Address{}
	collector.BeginEpochRewardBalanceUpdate(statsCollector, zeroAddress, zeroAddress, appState)
	appState.State.AddBalance(zeroAddress, total)
	collector.CompleteBalanceUpdate(statsCollector, appState)
	collector.AddMintedCoins(statsCollector, total)
	collector.SetTotalZeroWalletFund(statsCollector, total)
	collector.AddZeroWalletFund(statsCollector, zeroAddress, total)
}

func normalAge(age uint16) float32 {
	return float32(math2.Pow(float64(age)+1, float64(1)/3))
}
