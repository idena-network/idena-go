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

	poolOfAuth1 := common.Address{0x10}

	auth1 := common.Address{0x2}
	auth2 := common.Address{0x3}
	auth3 := common.Address{0x4}
	auth4 := common.Address{0x7}
	failed := common.Address{0x6}
	badAuth := common.Address{0x5}
	reporter := common.Address{0x8}

	conf := config.GetDefaultConsensusConfig()
	conf.BlockReward = big.NewInt(5)
	conf.FinalCommitteeReward = big.NewInt(5)

	memdb := db.NewMemDB()

	appState, _ := appstate.NewAppState(memdb, eventbus.New())

	appState.Initialize(0)

	appState.State.SetGlobalEpoch(5)
	appState.State.SetGodAddress(god)

	appState.State.SetState(auth1, state.Newbie)
	appState.State.SetDelegatee(auth1, poolOfAuth1)

	appState.State.SetState(auth2, state.Candidate)
	appState.State.SetState(auth3, state.Human)
	appState.State.SetState(auth4, state.Suspended)
	appState.State.SetState(badAuth, state.Newbie)
	appState.State.SetShardsNum(2)
	appState.Commit(nil)

	validationResults := map[common.ShardId]*types.ValidationResults{
		1: {
			BadAuthors: map[common.Address]types.BadAuthorReason{badAuth: types.WrongWordsBadAuthor},
			GoodAuthors: map[common.Address]*types.ValidationResult{
				auth1:  {FlipsToReward: []*types.FlipToReward{{[]byte{0x1}, types.GradeA}, {[]byte{0x1}, types.GradeB}}, NewIdentityState: uint8(state.Verified)},
				auth2:  {NewIdentityState: uint8(state.Newbie)},
				auth3:  {FlipsToReward: []*types.FlipToReward{{[]byte{0x1}, types.GradeA}, {[]byte{0x1}, types.GradeC}, {[]byte{0x1}, types.GradeD}}, NewIdentityState: uint8(state.Verified)},
				failed: {FlipsToReward: []*types.FlipToReward{{[]byte{0x1}, types.GradeA}, {[]byte{0x1}, types.GradeA}, {[]byte{0x1}, types.GradeA}}, Missed: true},
			},
			GoodInviters: map[common.Address]*types.InviterValidationResult{
				auth1:  {SuccessfulInvites: []*types.SuccessfulInvite{{2, common.Hash{}, 100}}, PayInvitationReward: true, SavedInvites: 1, NewIdentityState: uint8(state.Verified)},
				auth2:  {PayInvitationReward: true, SavedInvites: 1, NewIdentityState: uint8(state.Newbie)},
				auth3:  {PayInvitationReward: false, NewIdentityState: uint8(state.Verified)},
				auth4:  {PayInvitationReward: true, NewIdentityState: uint8(state.Verified), SuccessfulInvites: []*types.SuccessfulInvite{{3, common.Hash{}, 200}}},
				failed: {PayInvitationReward: false, SuccessfulInvites: []*types.SuccessfulInvite{{2, common.Hash{}, 0}}},
				god:    {SuccessfulInvites: []*types.SuccessfulInvite{{1, common.Hash{}, 50}, {2, common.Hash{}, 100}, {3, common.Hash{}, 200}}, PayInvitationReward: true},
			},
			ReportersToRewardByFlip: map[int]map[common.Address]*types.Candidate{
				100: {
					reporter: &types.Candidate{
						Address:          reporter,
						NewIdentityState: uint8(state.Newbie),
					},
					auth3: &types.Candidate{
						Address:          auth3,
						NewIdentityState: uint8(state.Verified),
					},
				},
			},
		},
		2: {
			ReportersToRewardByFlip: map[int]map[common.Address]*types.Candidate{
				150: {
					reporter: &types.Candidate{
						Address:          reporter,
						NewIdentityState: uint8(state.Newbie),
					},
				},
				151: {},
			},
		},
	}
	appState.State.SetState(auth1, state.Verified)
	appState.State.SetBirthday(auth1, 2)

	appState.State.SetState(auth2, state.Newbie)
	appState.State.SetBirthday(auth2, 5)

	appState.State.SetState(auth3, state.Verified)
	appState.State.SetBirthday(auth3, 4)

	appState.State.SetState(auth4, state.Verified)
	appState.State.SetBirthday(auth4, 1)

	appState.State.SetState(badAuth, state.Newbie)
	appState.State.SetBirthday(badAuth, 5)

	rewardValidIdentities(appState, conf, validationResults, []uint32{400, 200, 100}, types.Seed{1}, nil)

	appState.Commit(nil)

	validationReward := float32(200) / 5.557298 // 4^(1/3)+1^(1/3)+2^(1/3)+5^(1/3)
	flipReward := float32(350) / 23
	godPayout := float32(100)

	// sum all coefficients
	// auth1: conf.SecondInvitationRewardCoef + conf.SavedInviteWinnerRewardCoef (9 + 0)
	// auth2: conf.SavedInviteRewardCoef (0)
	// auth4: conf.ThirdInvitationRewardCoef (18)
	// god: conf.FirstInvitationRewardCoef + conf.SecondInvitationRewardCoef + conf.ThirdInvitationRewardCoef (3 + 9 + 18)
	// total: 57
	invitationReward := float32(3.1578947) // 180/57

	reportReward := float32(50) // 150 / 3

	reward, stake := splitAndSum(conf, false, validationReward*normalAge(3), flipReward*12.0, invitationReward*conf.SecondInvitationRewardCoef)

	require.Equal(t, reward.String(), appState.State.GetBalance(poolOfAuth1).String())
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth1)) == 0)

	reward, stake = splitAndSum(conf, true, validationReward*normalAge(0))
	require.True(t, reward.Cmp(appState.State.GetBalance(auth2)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth2)) == 0)

	reward, stake = splitAndSum(conf, false, validationReward*normalAge(1), flipReward*11.0, reportReward)
	require.True(t, reward.Cmp(appState.State.GetBalance(auth3)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth3)) == 0)

	reward, stake = splitAndSum(conf, false, validationReward*normalAge(4), invitationReward*conf.ThirdInvitationRewardCoef)
	require.True(t, reward.Cmp(appState.State.GetBalance(auth4)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth4)) == 0)

	reward, stake = splitAndSum(conf, false, invitationReward*conf.FirstInvitationRewardCoef, invitationReward*conf.SecondInvitationRewardCoef, invitationReward*conf.ThirdInvitationRewardCoef)
	reward.Add(reward, float32ToBigInt(godPayout))
	require.True(t, reward.Cmp(appState.State.GetBalance(god)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(god)) == 0)

	reward, stake = splitAndSum(conf, true, reportReward, reportReward)
	require.True(t, reward.Cmp(appState.State.GetBalance(reporter)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(reporter)) == 0)

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

func Test_getInvitationRewardCoef(t *testing.T) {
	consensusConf := &config.ConsensusConf{}
	consensusConf.FirstInvitationRewardCoef = 1.0
	consensusConf.SecondInvitationRewardCoef = 2.0
	consensusConf.ThirdInvitationRewardCoef = 4.0

	var coef float32

	coef = getInvitationRewardCoef(0, 0, []uint32{}, consensusConf)
	require.Zero(t, coef)

	coef = getInvitationRewardCoef(1, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(1.0), coef)

	coef = getInvitationRewardCoef(2, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(2.0), coef)

	coef = getInvitationRewardCoef(3, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(4.0), coef)

	coef = getInvitationRewardCoef(4, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(0.0), coef)

	coef = getInvitationRewardCoef(1, 0, []uint32{90}, consensusConf)
	require.Equal(t, float32(1.0), coef)

	coef = getInvitationRewardCoef(1, 90, []uint32{90}, consensusConf)
	require.Equal(t, float32(0.5), coef)

	coef = getInvitationRewardCoef(2, 90, []uint32{90}, consensusConf)
	require.Equal(t, float32(2.0), coef)

	coef = getInvitationRewardCoef(2, 70, []uint32{100, 90}, consensusConf)
	require.Equal(t, float32(1.7599), coef)

	coef = getInvitationRewardCoef(3, 192, []uint32{100, 90}, consensusConf)
	require.Equal(t, float32(4.0), coef)

	coef = getInvitationRewardCoef(3, 192, []uint32{200, 100, 90}, consensusConf)
	require.Equal(t, float32(2.301307), coef)

	coef = getInvitationRewardCoef(3, 200, []uint32{200, 100, 90}, consensusConf)
	require.Equal(t, float32(2.0), coef)

	coef = getInvitationRewardCoef(3, 1000, []uint32{200, 100, 90}, consensusConf)
	require.Equal(t, float32(2.0), coef)
}
