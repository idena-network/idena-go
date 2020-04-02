package blockchain

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-db"
	"math/big"
	"testing"
)

func Test_rewardValidIdentities(t *testing.T) {

	god := common.Address{0x1}
	auth1 := common.Address{0x2}
	auth2 := common.Address{0x3}
	auth3 := common.Address{0x4}
	failed := common.Address{0x6}
	badAuth := common.Address{0x5}

	conf := config.GetDefaultConsensusConfig()
	conf.BlockReward = big.NewInt(5)
	conf.FinalCommitteeReward = big.NewInt(5)

	memdb := db.NewMemDB()

	appState := appstate.NewAppState(memdb, eventbus.New())

	appState.Initialize(0)

	appState.State.SetGlobalEpoch(5)
	appState.State.SetGodAddress(god)

	appState.State.SetState(auth1, state.Newbie)
	appState.State.SetState(auth2, state.Candidate)
	appState.State.SetState(auth3, state.Human)
	appState.State.SetState(badAuth, state.Newbie)
	appState.Commit(nil)

	authors := types.ValidationAuthors{
		BadAuthors: map[common.Address]types.BadAuthorReason{badAuth: types.WrongWordsBadAuthor},
		GoodAuthors: map[common.Address]*types.ValidationResult{
			auth1:  {StrongFlipCids: [][]byte{{0x1}}, WeakFlipCids: [][]byte{{0x1}}, SuccessfulInvites: []*types.SuccessfulInvite{{2, common.Hash{}}}, PayInvitationReward: true, SavedInvites: 1, NewIdentityState: uint8(state.Verified)},
			auth2:  {StrongFlipCids: nil, WeakFlipCids: nil, PayInvitationReward: true, SavedInvites: 1, NewIdentityState: uint8(state.Newbie)},
			auth3:  {StrongFlipCids: [][]byte{{0x1}, {0x1}}, WeakFlipCids: [][]byte{{0x1}}, PayInvitationReward: false, Missed: false, NewIdentityState: uint8(state.Verified)},
			failed: {StrongFlipCids: [][]byte{{0x1}, {0x1}}, WeakFlipCids: [][]byte{{0x1}}, PayInvitationReward: false, Missed: true, SuccessfulInvites: []*types.SuccessfulInvite{{2, common.Hash{}}}},
			god:    {SuccessfulInvites: []*types.SuccessfulInvite{{1, common.Hash{}}, {2, common.Hash{}}, {3, common.Hash{}}}, PayInvitationReward: true},
		},
	}
	appState.State.SetState(auth1, state.Verified)
	appState.State.SetBirthday(auth1, 2)

	appState.State.SetState(auth2, state.Newbie)
	appState.State.SetBirthday(auth2, 5)

	appState.State.SetState(auth3, state.Verified)
	appState.State.SetBirthday(auth3, 4)

	appState.State.SetState(badAuth, state.Newbie)
	appState.State.SetBirthday(badAuth, 5)

	rewardValidIdentities(appState, conf, &authors, 100, types.Seed{1}, nil)

	appState.Commit(nil)

	validationReward := float32(240) / 3.847322
	flipReward := float32(320) / 5
	godPayout := float32(100)

	// sum all coefficients
	// auth1: conf.SecondInvitationRewardCoef + conf.SavedInviteRewardCoef (9 + 1)
	// auth2: conf.SavedInviteWinnerRewardCoef (2)
	// god: conf.FirstInvitationRewardCoef + conf.SecondInvitationRewardCoef + conf.ThirdInvitationRewardCoef (3 + 9 + 18)
	// total: 42
	invitationReward := float32(320) / 42

	reward, stake := splitAndSum(conf, false, validationReward*normalAge(3), flipReward*2, invitationReward*conf.SecondInvitationRewardCoef, invitationReward*conf.SavedInviteRewardCoef)
	require.True(t, reward.Cmp(appState.State.GetBalance(auth1)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth1)) == 0)

	reward, stake = splitAndSum(conf, true, validationReward*normalAge(0), invitationReward*conf.SavedInviteWinnerRewardCoef)
	require.True(t, reward.Cmp(appState.State.GetBalance(auth2)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth2)) == 0)

	reward, stake = splitAndSum(conf, false, validationReward*normalAge(1), flipReward*3)
	require.True(t, reward.Cmp(appState.State.GetBalance(auth3)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth3)) == 0)

	reward, stake = splitAndSum(conf, false, invitationReward*conf.FirstInvitationRewardCoef, invitationReward*conf.SecondInvitationRewardCoef, invitationReward*conf.ThirdInvitationRewardCoef)
	reward.Add(reward, float32ToBigInt(godPayout))
	require.True(t, reward.Cmp(appState.State.GetBalance(god)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(god)) == 0)

	require.True(t, appState.State.GetBalance(badAuth).Sign() == 0)
	require.True(t, appState.State.GetStakeBalance(badAuth).Sign() == 0)
	require.True(t, appState.State.GetBalance(failed).Sign() == 0)
	require.True(t, appState.State.GetStakeBalance(failed).Sign() == 0)

	require.True(t, big.NewInt(20).Cmp(appState.State.GetBalance(common.Address{})) == 0)
}

func float32ToBigInt(f float32) *big.Int {
	return math.ToInt(decimal.NewFromFloat32(f))
}

func splitAndSum(conf *config.ConsensusConf, isNewbie bool, nums ...float32) (*big.Int, *big.Int) {
	sumReward := big.NewInt(0)
	sumStake := big.NewInt(0)
	for _, n := range nums {
		reward, stake := splitReward(float32ToBigInt(n), isNewbie, conf)
		sumReward.Add(sumReward, reward)
		sumStake.Add(sumStake, stake)
	}
	return sumReward, sumStake
}

func Test_normalAge(t *testing.T) {

	require.Equal(t, float32(1.587401), normalAge(3))
	require.Equal(t, float32(2), normalAge(7))
	require.Equal(t, float32(3), normalAge(26))
}

func Test_splitReward(t *testing.T) {
	reward, stake := splitReward(big.NewInt(100), false, config.GetDefaultConsensusConfig())

	require.True(t, big.NewInt(80).Cmp(reward) == 0)
	require.True(t, big.NewInt(20).Cmp(stake) == 0)

	reward, stake = splitReward(big.NewInt(100), true, config.GetDefaultConsensusConfig())

	require.True(t, big.NewInt(20).Cmp(reward) == 0)
	require.True(t, big.NewInt(80).Cmp(stake) == 0)
}
