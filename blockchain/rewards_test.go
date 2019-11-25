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

	conf := GetDefaultConsensusConfig(false)

	conf.BlockReward = big.NewInt(5)
	conf.FinalCommitteeReward = big.NewInt(5)

	memdb := db.NewMemDB()

	appState := appstate.NewAppState(memdb, eventbus.New())

	appState.Initialize(0)

	appState.State.SetGlobalEpoch(5)
	appState.State.SetGodAddress(god)

	appState.State.SetState(auth1, state.Newbie)
	appState.State.SetState(auth2, state.Candidate)
	appState.State.SetState(auth3, state.Verified)
	appState.State.SetState(badAuth, state.Newbie)
	appState.Commit(nil)

	authors := types.ValidationAuthors{
		BadAuthors: map[common.Address]struct{}{badAuth: {}},
		GoodAuthors: map[common.Address]*types.ValidationResult{
			auth1:  {StrongFlips: 1, WeakFlips: 1, SuccessfulInviteAges: []uint16{2}, Validated: true},
			auth2:  {StrongFlips: 0, WeakFlips: 0, Validated: true},
			auth3:  {StrongFlips: 2, WeakFlips: 1, Validated: false, Missed: false},
			failed: {StrongFlips: 2, WeakFlips: 1, Validated: false, Missed: true, SuccessfulInviteAges: []uint16{2}},
			god:    {SuccessfulInviteAges: []uint16{1, 2, 3}, Validated: true},
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

	rewardValidIdentities(appState, conf, &authors, 100, nil)

	appState.Commit(nil)

	validationReward := float32(240) / 3.847322
	flipReward := float32(320) / 5
	invitationReward := float32(320) / 13
	godPayout := float32(100)

	reward, stake := splitAndSum(conf, validationReward*normalAge(3), flipReward*2, invitationReward*3)
	require.True(t, reward.Cmp(appState.State.GetBalance(auth1)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth1)) == 0)

	reward, stake = splitAndSum(conf, validationReward*normalAge(0))
	require.True(t, reward.Cmp(appState.State.GetBalance(auth2)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth2)) == 0)

	reward, stake = splitAndSum(conf, validationReward*normalAge(1), flipReward*3)
	require.True(t, reward.Cmp(appState.State.GetBalance(auth3)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth3)) == 0)

	reward, stake = splitAndSum(conf, invitationReward, invitationReward*3, invitationReward*6)
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

func splitAndSum(conf *config.ConsensusConf, nums ...float32) (*big.Int, *big.Int) {
	sumReward := big.NewInt(0)
	sumStake := big.NewInt(0)
	for _, n := range nums {
		reward, stake := splitReward(float32ToBigInt(n), conf)
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
	reward, stake := splitReward(big.NewInt(100), GetDefaultConsensusConfig(false))

	require.True(t, big.NewInt(80).Cmp(reward) == 0)
	require.True(t, big.NewInt(20).Cmp(stake) == 0)
}
