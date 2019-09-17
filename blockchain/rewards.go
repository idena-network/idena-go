package blockchain

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/log"
	"github.com/shopspring/decimal"
	math2 "math"
	"math/big"
)

func rewardValidIdentities(appState *appstate.AppState, config *config.ConsensusConf, authors *types.ValidationAuthors, blocks uint64) {
	totalReward := big.NewInt(0).Add(config.BlockReward, config.FinalCommitteeReward)
	totalReward = totalReward.Mul(totalReward, big.NewInt(int64(blocks)))

	log.Info("Total validation reward", "reward", ConvertToFloat(totalReward).String())

	totalRewardD := decimal.NewFromBigInt(totalReward, 0)
	addSuccessfullValidationReward(appState, config, authors, totalRewardD)
	addFlipReward(appState, config, authors, totalRewardD)
	addInvitationReward(appState, config, authors, totalRewardD)
	addFoundationPayouts(appState, config, totalRewardD)
	addZeroWalletFund(appState, config, totalRewardD)
}

func addSuccessfullValidationReward(appState *appstate.AppState, config *config.ConsensusConf, authors *types.ValidationAuthors, totalReward decimal.Decimal) {
	successfullValidationRewardD := totalReward.Mul(decimal.NewFromFloat32(config.SuccessfullValidationRewardPercent))

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

	successfullValidationRewardShare := successfullValidationRewardD.Div(decimal.NewFromFloat32(normalizedAges))

	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		switch identity.State {
		case state.Verified, state.Newbie:
			if _, ok := authors.BadAuthors[addr]; !ok {
				normalAge := normalAge(epoch - identity.Birthday)
				totalReward := successfullValidationRewardShare.Mul(decimal.NewFromFloat32(normalAge))
				reward, stake := splitReward(math.ToInt(totalReward), config)
				appState.State.AddBalance(addr, reward)
				appState.State.AddStake(addr, stake)
			}
		}
	})
}

func addFlipReward(appState *appstate.AppState, config *config.ConsensusConf, authors *types.ValidationAuthors, totalReward decimal.Decimal) {
	flipRewardD := totalReward.Mul(decimal.NewFromFloat32(config.FlipRewardPercent))

	totalFlips := float32(0)
	for _, author := range authors.GoodAuthors {
		totalFlips += float32(author.WeakFlips + author.StrongFlips)
	}
	if totalFlips == 0 {
		return
	}
	flipRewardShare := flipRewardD.Div(decimal.NewFromFloat32(totalFlips))

	for addr, author := range authors.GoodAuthors {
		totalReward := flipRewardShare.Mul(decimal.NewFromFloat32(float32(author.StrongFlips + author.WeakFlips)))
		reward, stake := splitReward(math.ToInt(totalReward), config)
		appState.State.AddBalance(addr, reward)
		appState.State.AddStake(addr, stake)
	}
}

func addInvitationReward(appState *appstate.AppState, config *config.ConsensusConf, authors *types.ValidationAuthors, totalReward decimal.Decimal) {
	invitationRewardD := totalReward.Mul(decimal.NewFromFloat32(config.ValidInvitationRewardPercent))

	totalInvites := float32(0)
	for _, author := range authors.GoodAuthors {
		totalInvites += float32(author.SuccessfulInvites)
	}
	if totalInvites == 0 {
		return
	}
	invitationRewardShare := invitationRewardD.Div(decimal.NewFromFloat32(totalInvites))

	for addr, author := range authors.GoodAuthors {
		totalReward := invitationRewardShare.Mul(decimal.NewFromFloat32(float32(author.SuccessfulInvites)))
		reward, stake := splitReward(math.ToInt(totalReward), config)
		appState.State.AddBalance(addr, reward)
		appState.State.AddStake(addr, stake)
	}
}

func addFoundationPayouts(appState *appstate.AppState, config *config.ConsensusConf, totalReward decimal.Decimal) {
	payout := totalReward.Mul(decimal.NewFromFloat32(config.FoundationPayoutsPercent))
	appState.State.AddBalance(appState.State.GodAddress(), math.ToInt(payout))
}

func addZeroWalletFund(appState *appstate.AppState, config *config.ConsensusConf, totalReward decimal.Decimal) {
	payout := totalReward.Mul(decimal.NewFromFloat32(config.ZeroWalletPercent))
	appState.State.AddBalance(common.Address{}, math.ToInt(payout))
}

func normalAge(age uint16) float32 {
	return float32(math2.Pow(float64(age)+1, float64(1)/3))
}
