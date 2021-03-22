package blockchain

import (
	"bytes"
	"encoding/binary"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/shopspring/decimal"
	math2 "math"
	"math/big"
	"math/rand"
	"sort"
)

func rewardValidIdentities(appState *appstate.AppState, config *config.ConsensusConf, validationResults *types.ValidationResults,
	blocks uint64, seed types.Seed, statsCollector collector.StatsCollector) {

	totalReward := big.NewInt(0).Add(config.BlockReward, config.FinalCommitteeReward)
	totalReward = totalReward.Mul(totalReward, big.NewInt(int64(blocks)))

	collector.SetValidationResults(statsCollector, validationResults)
	collector.SetTotalReward(statsCollector, totalReward)

	log.Info("Total validation reward", "reward", ConvertToFloat(totalReward).String())

	totalRewardD := decimal.NewFromBigInt(totalReward, 0)
	addSuccessfulValidationReward(appState, config, validationResults, totalRewardD, statsCollector)
	addFlipReward(appState, config, validationResults, totalRewardD, statsCollector)
	addInvitationReward(appState, config, validationResults, totalRewardD, seed, statsCollector)
	addFoundationPayouts(appState, config, totalRewardD, statsCollector)
	addZeroWalletFund(appState, config, totalRewardD, statsCollector)
}

func addSuccessfulValidationReward(appState *appstate.AppState, config *config.ConsensusConf,
	validationResults *types.ValidationResults, totalReward decimal.Decimal, statsCollector collector.StatsCollector) {
	successfulValidationRewardD := totalReward.Mul(decimal.NewFromFloat32(config.SuccessfulValidationRewardPercent))

	epoch := appState.State.Epoch()

	normalizedAges := float32(0)
	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		if identity.State.NewbieOrBetter() {
			if _, ok := validationResults.BadAuthors[addr]; !ok {
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
			if _, ok := validationResults.BadAuthors[addr]; !ok {
				age := epoch - identity.Birthday
				normalAge := normalAge(age)
				totalReward := successfulValidationRewardShare.Mul(decimal.NewFromFloat32(normalAge))
				reward, stake := splitReward(math.ToInt(totalReward), identity.State == state.Newbie, config)
				collector.BeginEpochRewardBalanceUpdate(statsCollector, addr, appState)
				rewardDest := addr
				if identity.Delegatee != nil {
					rewardDest = *identity.Delegatee
				}
				appState.State.AddBalance(rewardDest, reward)
				appState.State.AddStake(addr, stake)
				collector.CompleteBalanceUpdate(statsCollector, appState)
				collector.AddMintedCoins(statsCollector, reward)
				collector.AddMintedCoins(statsCollector, stake)
				collector.AddValidationReward(statsCollector, addr, age, reward, stake)
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

func addFlipReward(appState *appstate.AppState, config *config.ConsensusConf, validationResults *types.ValidationResults,
	totalReward decimal.Decimal, statsCollector collector.StatsCollector) {
	flipRewardD := totalReward.Mul(decimal.NewFromFloat32(config.FlipRewardPercent))

	totalWeight := float32(0)
	for _, author := range validationResults.GoodAuthors {
		if author.Missed {
			continue
		}
		for _, f := range author.FlipsToReward {
			totalWeight += getFlipRewardCoef(f.Grade)
		}
	}
	for _, reporters := range validationResults.ReportersToRewardByFlip {
		if len(reporters) == 0 {
			continue
		}
		totalWeight += 1
	}

	if totalWeight == 0 {
		return
	}
	flipRewardShare := flipRewardD.Div(decimal.NewFromFloat32(totalWeight))
	collector.SetTotalFlipsReward(statsCollector, math.ToInt(flipRewardD), math.ToInt(flipRewardShare))

	for addr, author := range validationResults.GoodAuthors {
		if author.Missed {
			continue
		}
		var weight float32
		for _, f := range author.FlipsToReward {
			weight += getFlipRewardCoef(f.Grade)
		}
		totalReward := flipRewardShare.Mul(decimal.NewFromFloat32(weight))
		reward, stake := splitReward(math.ToInt(totalReward), author.NewIdentityState == uint8(state.Newbie), config)
		collector.BeginEpochRewardBalanceUpdate(statsCollector, addr, appState)
		rewardDest := addr
		if delegatee := appState.State.Delegatee(addr); delegatee != nil {
			rewardDest = *delegatee
		}
		appState.State.AddBalance(rewardDest, reward)
		appState.State.AddStake(addr, stake)
		collector.CompleteBalanceUpdate(statsCollector, appState)
		collector.AddMintedCoins(statsCollector, reward)
		collector.AddMintedCoins(statsCollector, stake)
		collector.AddFlipsReward(statsCollector, addr, reward, stake, author.FlipsToReward)
		collector.AfterAddStake(statsCollector, addr, stake, appState)
	}
	for flipIdx, reporters := range validationResults.ReportersToRewardByFlip {
		if len(reporters) == 0 {
			continue
		}
		totalReward := flipRewardShare.Div(decimal.NewFromInt(int64(len(reporters))))
		for _, reporter := range reporters {
			reward, stake := splitReward(math.ToInt(totalReward), reporter.NewIdentityState == uint8(state.Newbie), config)
			collector.BeginEpochRewardBalanceUpdate(statsCollector, reporter.Address, appState)
			rewardDest := reporter.Address
			if delegatee := appState.State.Delegatee(reporter.Address); delegatee != nil {
				rewardDest = *delegatee
			}
			appState.State.AddBalance(rewardDest, reward)
			appState.State.AddStake(reporter.Address, stake)
			collector.CompleteBalanceUpdate(statsCollector, appState)
			collector.AddMintedCoins(statsCollector, reward)
			collector.AddMintedCoins(statsCollector, stake)
			collector.AddReportedFlipsReward(statsCollector, reporter.Address, flipIdx, reward, stake)
			collector.AfterAddStake(statsCollector, reporter.Address, stake, appState)
		}
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

func addInvitationReward(appState *appstate.AppState, config *config.ConsensusConf, validationResults *types.ValidationResults,
	totalReward decimal.Decimal, seed types.Seed, statsCollector collector.StatsCollector) {
	invitationRewardD := totalReward.Mul(decimal.NewFromFloat32(config.ValidInvitationRewardPercent))

	addAddress := func(data []common.Hash, elem common.Hash) []common.Hash {
		index := sort.Search(len(data), func(i int) bool { return bytes.Compare(data[i][:], elem[:]) > 0 })
		data = append(data, common.Hash{})
		copy(data[index+1:], data[index:])
		data[index] = elem
		return data
	}

	addresses := make([]common.Hash, 0)
	totalWeight := float32(0)
	for addr, inviter := range validationResults.GoodInviters {
		if !inviter.PayInvitationReward {
			continue
		}
		for _, successfulInvite := range inviter.SuccessfulInvites {
			totalWeight += getInvitationRewardCoef(successfulInvite.Age, config)
		}
		for i := uint8(0); i < inviter.SavedInvites; i++ {
			addresses = addAddress(addresses, crypto.Hash(append(addr[:], i)))
		}
	}

	p := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(seed[:]))*55 - 11)).Perm(len(addresses))
	savedInvitesWinnersCount := len(addresses) / 2
	totalWeight += float32(savedInvitesWinnersCount)*config.SavedInviteWinnerRewardCoef + float32(len(p)-savedInvitesWinnersCount)*config.SavedInviteRewardCoef

	win := make(map[common.Hash]struct{})
	for i := 0; i < savedInvitesWinnersCount; i++ {
		win[addresses[p[i]]] = struct{}{}
	}

	if totalWeight == 0 {
		return
	}
	invitationRewardShare := invitationRewardD.Div(decimal.NewFromFloat32(totalWeight))
	collector.SetTotalInvitationsReward(statsCollector, math.ToInt(invitationRewardD), math.ToInt(invitationRewardShare))

	addReward := func(addr common.Address, totalReward decimal.Decimal, isNewbie bool, age uint16, txHash *common.Hash,
		isSavedInviteWinner bool) {
		reward, stake := splitReward(math.ToInt(totalReward), isNewbie, config)
		collector.BeginEpochRewardBalanceUpdate(statsCollector, addr, appState)
		rewardDest := addr
		if delegatee := appState.State.Delegatee(addr); delegatee != nil {
			rewardDest = *delegatee
		}
		appState.State.AddBalance(rewardDest, reward)
		appState.State.AddStake(addr, stake)
		collector.CompleteBalanceUpdate(statsCollector, appState)
		collector.AddMintedCoins(statsCollector, reward)
		collector.AddMintedCoins(statsCollector, stake)
		collector.AddInvitationsReward(statsCollector, addr, reward, stake, age, txHash, isSavedInviteWinner)
		collector.AfterAddStake(statsCollector, addr, stake, appState)
	}

	for addr, inviter := range validationResults.GoodInviters {
		if !inviter.PayInvitationReward {
			continue
		}
		isNewbie := inviter.NewIdentityState == uint8(state.Newbie)
		for _, successfulInvite := range inviter.SuccessfulInvites {
			if weight := getInvitationRewardCoef(successfulInvite.Age, config); weight > 0 {
				totalReward := invitationRewardShare.Mul(decimal.NewFromFloat32(weight))
				addReward(addr, totalReward, isNewbie, successfulInvite.Age, &successfulInvite.TxHash, false)
			}
		}
		for i := uint8(0); i < inviter.SavedInvites; i++ {
			hash := crypto.Hash(append(addr[:], i))
			if _, ok := win[hash]; ok {
				totalReward := invitationRewardShare.Mul(decimal.NewFromFloat32(config.SavedInviteWinnerRewardCoef))
				addReward(addr, totalReward, isNewbie, 0, nil, true)
			} else {
				totalReward := invitationRewardShare.Mul(decimal.NewFromFloat32(config.SavedInviteRewardCoef))
				addReward(addr, totalReward, isNewbie, 0, nil, false)
			}
		}
	}
}

func addFoundationPayouts(appState *appstate.AppState, config *config.ConsensusConf, totalReward decimal.Decimal,
	statsCollector collector.StatsCollector) {
	payout := totalReward.Mul(decimal.NewFromFloat32(config.FoundationPayoutsPercent))
	total := math.ToInt(payout)
	godAddress := appState.State.GodAddress()
	collector.BeginEpochRewardBalanceUpdate(statsCollector, godAddress, appState)
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
	collector.BeginEpochRewardBalanceUpdate(statsCollector, zeroAddress, appState)
	appState.State.AddBalance(zeroAddress, total)
	collector.CompleteBalanceUpdate(statsCollector, appState)
	collector.AddMintedCoins(statsCollector, total)
	collector.SetTotalZeroWalletFund(statsCollector, total)
	collector.AddZeroWalletFund(statsCollector, zeroAddress, total)
}

func normalAge(age uint16) float32 {
	return float32(math2.Pow(float64(age)+1, float64(1)/3))
}
