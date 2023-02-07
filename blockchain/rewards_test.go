package blockchain

import (
	"bytes"
	"fmt"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/tests"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-db"
	math2 "math"
	"math/big"
	"sort"
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
	addr1 := common.Address{0x9}
	addr2 := common.Address{0xa}

	conf := GetDefaultConsensusConfig()
	conf.BlockReward = big.NewInt(5)
	conf.FinalCommitteeReward = big.NewInt(5)

	memdb := db.NewMemDB()

	appState, _ := appstate.NewAppState(memdb, eventbus.New())

	appState.Initialize(0)

	appState.State.SetGlobalEpoch(5)
	appState.State.SetGodAddress(god)

	appState.State.SetState(auth1, state.Newbie)
	appState.State.SetDelegatee(auth1, poolOfAuth1)
	appState.State.AddStake(auth1, big.NewInt(10))

	appState.State.SetState(auth2, state.Candidate)
	appState.State.SetState(addr1, state.Candidate)
	appState.State.SetState(addr2, state.Candidate)
	appState.State.AddStake(addr2, big.NewInt(1100))
	appState.State.SetState(auth3, state.Human)
	appState.State.AddStake(auth3, big.NewInt(95))
	appState.State.SetState(auth4, state.Suspended)
	appState.State.SetState(badAuth, state.Newbie)
	appState.State.SetShardsNum(2)
	appState.Commit(nil)

	validationResults := map[common.ShardId]*types.ValidationResults{
		1: {
			BadAuthors: map[common.Address]types.BadAuthorReason{badAuth: types.WrongWordsBadAuthor},
			GoodAuthors: map[common.Address]*types.ValidationResult{
				auth1:  {FlipsToReward: []*types.FlipToReward{{[]byte{0x1}, types.GradeA, decimal.Zero}, {[]byte{0x1}, types.GradeB, decimal.Zero}}, NewIdentityState: uint8(state.Verified)},
				auth2:  {NewIdentityState: uint8(state.Newbie)},
				auth3:  {FlipsToReward: []*types.FlipToReward{{[]byte{0x1}, types.GradeA, decimal.Zero}, {[]byte{0x1}, types.GradeC, decimal.Zero}, {[]byte{0x1}, types.GradeD, decimal.Zero}}, NewIdentityState: uint8(state.Verified)},
				failed: {FlipsToReward: []*types.FlipToReward{{[]byte{0x1}, types.GradeA, decimal.Zero}, {[]byte{0x1}, types.GradeA, decimal.Zero}, {[]byte{0x1}, types.GradeA, decimal.Zero}}, Missed: true},
			},
			GoodInviters: map[common.Address]*types.InviterValidationResult{
				auth1: {
					SuccessfulInvites: []*types.SuccessfulInvite{
						{2, common.Hash{}, 100, true, common.Address{}},
					}, PayInvitationReward: true, NewIdentityState: uint8(state.Verified)},
				auth2: {PayInvitationReward: true, NewIdentityState: uint8(state.Newbie)},
				auth3: {PayInvitationReward: false, NewIdentityState: uint8(state.Verified)},
				auth4: {PayInvitationReward: true, NewIdentityState: uint8(state.Verified),
					SuccessfulInvites: []*types.SuccessfulInvite{
						{3, common.Hash{}, 200, false, common.Address{}},
					}},
				failed: {PayInvitationReward: false,
					SuccessfulInvites: []*types.SuccessfulInvite{
						{2, common.Hash{}, 0, false, common.Address{}},
					}},
				god: {
					SuccessfulInvites: []*types.SuccessfulInvite{
						{1, common.Hash{}, 50, false, common.Address{}},
						{2, common.Hash{}, 100, false, common.Address{}},
						{3, common.Hash{}, 200, false, common.Address{}},
					}, PayInvitationReward: true},
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

	appState.State.SetState(addr1, state.Newbie)
	appState.State.SetBirthday(addr1, 5)

	appState.State.SetState(addr2, state.Newbie)
	appState.State.SetBirthday(addr2, 5)

	rewardValidIdentities(appState, conf, validationResults, []uint32{400, 200, 100}, nil, nil)

	appState.Commit(nil)

	candidateReward := float32(20) / 3
	stakingReward := float32(180) / 614.2688878 // 10^0.9 + 95^0.9 + 1100^0.9
	flipReward := float32(150) / 23
	godPayout := float32(100)

	invitationReward := float32(180)

	reportReward := float32(50) // 150 / 3

	const stake10Weight = 7.9432823 // 10^0.9
	reward, stake := splitAndSum(conf, false, stakingReward*stake10Weight, flipReward*12.0, invitationReward)

	require.Equal(t, reward.String(), appState.State.GetBalance(poolOfAuth1).String())
	require.True(t, new(big.Int).Add(big.NewInt(10), stake).Cmp(appState.State.GetStakeBalance(auth1)) == 0)

	reward, stake = splitAndSum(conf, true, candidateReward)

	require.True(t, reward.Cmp(appState.State.GetBalance(auth2)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth2)) == 0)

	require.True(t, reward.Cmp(appState.State.GetBalance(addr1)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(addr1)) == 0)

	const stake1100Weight = 546.076411 // 1100^0.9
	reward, stake = splitAndSum(conf, true, stakingReward*stake1100Weight, candidateReward)
	require.True(t, reward.Cmp(appState.State.GetBalance(addr2)) == 0)
	require.True(t, new(big.Int).Add(big.NewInt(1100), stake).Cmp(appState.State.GetStakeBalance(addr2)) == 0)

	const stake95Weight = 60.2491944 // 95^0.9
	reward, stake = splitAndSum(conf, false, stakingReward*stake95Weight, flipReward*11.0, reportReward)
	require.True(t, reward.Cmp(appState.State.GetBalance(auth3)) == 0)
	require.True(t, new(big.Int).Add(big.NewInt(95), stake).Cmp(appState.State.GetStakeBalance(auth3)) == 0)

	reward, stake = splitAndSum(conf, false, 0)

	require.True(t, reward.Cmp(appState.State.GetBalance(auth4)) == 0)
	require.True(t, stake.Cmp(appState.State.GetStakeBalance(auth4)) == 0)

	reward, stake = splitAndSum(conf, false, 0)
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
	reward, stake := splitReward(big.NewInt(100), false, GetDefaultConsensusConfig())

	require.True(t, big.NewInt(80).Cmp(reward) == 0)
	require.True(t, big.NewInt(20).Cmp(stake) == 0)

	reward, stake = splitReward(big.NewInt(100), true, GetDefaultConsensusConfig())

	require.True(t, big.NewInt(20).Cmp(reward) == 0)
	require.True(t, big.NewInt(80).Cmp(stake) == 0)
}

func Test_getInvitationRewardCoef(t *testing.T) {
	consensusConf := GetDefaultConsensusConfig()

	var inviter, invitee float32

	inviter, invitee = getInvitationRewardCoef(1, 0, false, 0, []uint32{}, consensusConf)
	require.Zero(t, invitee)
	require.Zero(t, inviter)
	inviter, invitee = getInvitationRewardCoef(1, 0, true, 0, []uint32{}, consensusConf)
	require.Zero(t, invitee)
	require.Zero(t, inviter)

	inviter, invitee = getInvitationRewardCoef(2.1, 1, false, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(2.1), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(2.1, 1, true, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(0.42), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(3.21, 2, false, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(3.21), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(3.21, 2, true, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(1.605), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(4.321, 3, false, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(4.321), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(4.321, 3, true, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(3.4568002), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(5.4321, 4, false, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(0.0), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(5.4321, 4, true, 0, []uint32{}, consensusConf)
	require.Equal(t, float32(0.0), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(6.54321, 1, false, 0, []uint32{90}, consensusConf)
	require.Equal(t, float32(6.54321), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(6.54321, 1, true, 0, []uint32{90}, consensusConf)
	require.Equal(t, float32(1.308642), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(6.54321, 1, false, 90, []uint32{90}, consensusConf)
	require.Equal(t, float32(3.271605), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(6.54321, 1, true, 90, []uint32{90}, consensusConf)
	require.Equal(t, float32(0.654321), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(7.654321, 2, false, 90, []uint32{90}, consensusConf)
	require.Equal(t, float32(7.654321), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(7.654321, 2, true, 90, []uint32{90}, consensusConf)
	require.Equal(t, float32(3.8271605), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(8.7654321, 2, false, 70, []uint32{100, 90}, consensusConf)
	require.Equal(t, float32(7.713142), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(8.7654321, 2, true, 70, []uint32{100, 90}, consensusConf)
	require.Equal(t, float32(3.856571), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(9.87654321, 3, false, 192, []uint32{100, 90}, consensusConf)
	require.Equal(t, float32(9.87654321), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(9.87654321, 3, true, 192, []uint32{100, 90}, consensusConf)
	require.Equal(t, float32(7.9012346), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(10.987654321, 3, false, 192, []uint32{200, 100, 90}, consensusConf)
	require.Equal(t, float32(6.3214917), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(10.987654321, 3, true, 192, []uint32{200, 100, 90}, consensusConf)
	require.Equal(t, float32(5.0571933), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(10.987654321, 3, false, 200, []uint32{200, 100, 90}, consensusConf)
	require.Equal(t, float32(5.4938273), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(10.987654321, 3, true, 200, []uint32{200, 100, 90}, consensusConf)
	require.Equal(t, float32(4.395062), inviter)
	require.Zero(t, invitee)

	inviter, invitee = getInvitationRewardCoef(10.987654321, 3, false, 1000, []uint32{200, 100, 90}, consensusConf)
	require.Equal(t, float32(5.4938273), inviter+invitee)
	inviter, invitee = getInvitationRewardCoef(10.987654321, 3, true, 1000, []uint32{200, 100, 90}, consensusConf)
	require.Equal(t, float32(4.395062), inviter)
	require.Zero(t, invitee)
}

func Test_addSuccessfulValidationReward1(t *testing.T) {
	memdb := db.NewMemDB()
	appState, _ := appstate.NewAppState(memdb, eventbus.New())
	_ = appState.Initialize(0)

	conf := GetDefaultConsensusConfig()

	appState.State.SetGlobalEpoch(1)

	addrZeroStake := common.Address{0x1}
	appState.State.SetState(addrZeroStake, state.Verified)

	addrNotValidated := common.Address{0x2}
	appState.State.AddStake(addrNotValidated, ConvertToInt(decimal.RequireFromString("111.111")))

	addrPenalized := common.Address{0x3}
	appState.State.SetState(addrPenalized, state.Verified)
	appState.State.AddStake(addrPenalized, ConvertToInt(decimal.RequireFromString("222.222")))

	addr1 := common.Address{0x4}
	appState.State.SetState(addr1, state.Verified)
	appState.State.AddStake(addr1, ConvertToInt(decimal.RequireFromString("0.0000000000000001")))

	addr2 := common.Address{0x5}
	appState.State.SetState(addr2, state.Verified)
	appState.State.AddStake(addr2, ConvertToInt(decimal.RequireFromString("2.013650178560176501")))

	addr3 := common.Address{0x6}
	appState.State.SetState(addr3, state.Verified)
	appState.State.AddStake(addr3, ConvertToInt(decimal.RequireFromString("915.160012350876913691")))

	addr4 := common.Address{0x7}
	appState.State.SetState(addr4, state.Verified)
	appState.State.AddStake(addr4, ConvertToInt(decimal.RequireFromString("9125849.019823751067178698")))

	addr5 := common.Address{0x8}
	appState.State.SetState(addr5, state.Verified)
	appState.State.AddStake(addr5, ConvertToInt(decimal.RequireFromString("9136892363.230579078150897132")))

	addr6 := common.Address{0x9}
	appState.State.SetState(addr6, state.Verified)
	appState.State.AddStake(addr6, ConvertToInt(decimal.RequireFromString("0.0000000000000005")))

	_ = appState.Commit(nil)
	validationResults := map[common.ShardId]*types.ValidationResults{
		1: {
			BadAuthors: map[common.Address]types.BadAuthorReason{addrPenalized: types.WrongWordsBadAuthor},
		},
	}

	totalReward := decimal.RequireFromString("545000149673614247952282")

	stakeWeights := addSuccessfulValidationReward(appState, conf, validationResults, totalReward, nil)
	_ = appState.Commit(nil)

	require.Zero(t, appState.State.GetBalance(addrZeroStake).Sign())
	require.Zero(t, appState.State.GetBalance(addrPenalized).Sign())
	require.Zero(t, appState.State.GetBalance(addrNotValidated).Sign())
	require.Zero(t, appState.State.GetBalance(addr1).Sign())
	require.Equal(t, "0.000159500166643957", ConvertToFloat(appState.State.GetBalance(addr2)).String())
	require.Equal(t, "0.039311794902290124", ConvertToFloat(appState.State.GetBalance(addr3)).String())
	require.Equal(t, "156.106680689089783251", ConvertToFloat(appState.State.GetBalance(addr4)).String())
	require.Equal(t, "78323.87772688500601495", ConvertToFloat(appState.State.GetBalance(addr5)).String())
	require.Equal(t, "0.000000000000000001", ConvertToFloat(appState.State.GetBalance(addr6)).String())

	require.Len(t, stakeWeights, 8)
	weight, ok := stakeWeights[addrZeroStake]
	require.True(t, ok)
	require.Zero(t, weight)
	weight, ok = stakeWeights[appState.State.GodAddress()]
	require.True(t, ok)
	require.Zero(t, weight)
	require.Equal(t, calculateWeight("0.0000000000000001"), stakeWeights[addr1])
	require.Equal(t, calculateWeight("2.013650178560176501"), stakeWeights[addr2])
	require.Equal(t, calculateWeight("915.160012350876913691"), stakeWeights[addr3])
	require.Equal(t, calculateWeight("9125849.019823751067178698"), stakeWeights[addr4])
	require.Equal(t, calculateWeight("9136892363.230579078150897132"), stakeWeights[addr5])
	require.Equal(t, calculateWeight("0.0000000000000005"), stakeWeights[addr6])
}

func Test_addSuccessfulValidationReward2(t *testing.T) {
	memdb := db.NewMemDB()
	appState, _ := appstate.NewAppState(memdb, eventbus.New())
	_ = appState.Initialize(0)

	conf := config.GetDefaultConsensusConfig()

	appState.State.SetGlobalEpoch(1)
	appState.State.SetGodAddress(common.Address{})

	type Data struct {
		initialStake, expectedBalance, expectedStake string
	}
	data := []Data{
		{"23.901251933403362891", "1.744160086320680416", "24.337291954983532995"},
		{"25.030759479121613193", "1.81816966969033754", "25.485301896544197578"},
		{"98.299319143329667938", "6.227349615579322676", "99.856156547224498607"},
		{"61.455376194675076080", "4.080484016756367336", "62.475497198864167914"},
		{"51.121278767526378358", "3.457397517133370383", "51.985628146809720953"},
		{"156.233602195720497997", "9.449412013012337612", "158.595955198973582399"},
		{"74.543022175331808255", "4.854830532193606389", "75.756729808380209852"},
		{"120.612559598967739968", "7.486193891175377375", "122.484108071761584311"},
		{"157.500564367818495446", "9.518350026994330551", "159.880151874567078083"},
		{"54.127919825522390643", "3.639879190812445165", "55.037889623225501934"},
		{"121.088924081043980791", "7.512798983239044355", "122.967123826853741879"},
		{"174.206379657465592313", "10.422344270243071328", "176.811965725026360144"},
		{"4.4459456702478145389", "0.383862341914930379", "4.541911255726547132"},
		{"8.8672564359573240457", "0.714526161559155794", "9.045887976347112993"},
		{"3.6416131161381747353", "0.320754085282201624", "3.721801637458725141"},
		{"52.6356419927491460169", "3.549439007342764074", "53.523001744584837034"},
		{"9.4958940134037782705", "0.759958795410504958", "9.685883712256404509"},
		{"64.365317468597527652", "4.253970766593101256", "65.428810160245802965"},
		{"143.486033647725247153", "8.75258740274776271", "145.67418049841218783"},
		{"331.253594722354378435", "18.584561078304970168", "335.899734991930620977"},
		{"63.917471517456865638", "4.227322850662469677", "64.974302230122483057"},
		{"47.202582327438713366", "3.217932570762616637", "48.007065470129367525"},
		{"74.324912167463461354", "4.842044166701078953", "75.535423209138731092"},
		{"184.544365845332718624", "10.977374468716401097", "187.288709462511818898"},
		{"45.651514209219230018", "3.122607789967285574", "46.432166156711051411"},
		{"30.204964927921225969", "2.153169883809672395", "30.743257398873644067"},
		{"78.449401861221527367", "5.083214802987270543", "79.720205561968345002"},
		{"41.169665279198971657", "2.845295530564770365", "41.880989161840164248"},
	}
	addrs := make([]common.Address, 0, len(data))
	for i := 0; i < len(data); i++ {
		addrs = append(addrs, tests.GetRandAddr())
	}
	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i].Bytes(), addrs[j].Bytes()) == -1
	})

	for i, addr := range addrs {
		appState.State.SetState(addr, state.Verified)
		appState.State.AddStake(addr, ConvertToInt(decimal.RequireFromString(data[i].initialStake)))
	}

	_ = appState.Commit(nil)
	validationResults := map[common.ShardId]*types.ValidationResults{
		1: {},
	}

	totalReward := decimal.RequireFromString("1000000000000000000000")

	stakeWeights := addSuccessfulValidationReward(appState, conf, validationResults, totalReward, nil)
	_ = appState.Commit(nil)

	for i, addr := range addrs {
		require.Equal(t, data[i].expectedBalance, ConvertToFloat(appState.State.GetBalance(addr)).String(), fmt.Sprintf("wrong balance of address with index %v", i))
		require.Equal(t, data[i].expectedStake, ConvertToFloat(appState.State.GetStakeBalance(addr)).String(), fmt.Sprintf("wrong stake of address with index %v", i))
	}

	require.Len(t, stakeWeights, len(addrs)+1)
	for i, addr := range addrs {
		weight, ok := stakeWeights[addr]
		require.True(t, ok)
		require.Equal(t, calculateWeight(data[i].initialStake), weight)
	}
	weight, ok := stakeWeights[appState.State.GodAddress()]
	require.True(t, ok)
	require.Zero(t, weight)
}

func calculateWeight(stakeS string) float32 {
	stake, _ := decimal.RequireFromString(stakeS).Float64()
	return float32(math2.Pow(stake, 0.9))
}

func Test_addInvitationReward(t *testing.T) {
	memdb := db.NewMemDB()

	appState, _ := appstate.NewAppState(memdb, eventbus.New())
	require.NoError(t, appState.Initialize(0))

	cfg := GetDefaultConsensusConfig()

	var addrs []common.Address
	for i := byte(1); i <= byte(13); i++ {
		addrs = append(addrs, common.Address{i})
	}

	stakeWeights := make(map[common.Address]float32)
	appState.State.SetGodAddress(addrs[0])

	stakeWeights[addrs[0]] = calculateWeight("48.972831666678647531")
	stakeWeights[addrs[1]] = calculateWeight("1518501.612011012002755181")
	stakeWeights[addrs[2]] = calculateWeight("91206.147842382102804845")
	stakeWeights[addrs[3]] = calculateWeight("43904.383871607742557663")
	stakeWeights[addrs[4]] = calculateWeight("19852.173267418088262289")
	stakeWeights[addrs[5]] = calculateWeight("9604.202734533158562331")
	stakeWeights[addrs[6]] = calculateWeight("2804.507900290687139989")
	stakeWeights[addrs[7]] = calculateWeight("54.088262289871399891")
	stakeWeights[addrs[8]] = calculateWeight("99999")
	stakeWeights[addrs[9]] = calculateWeight("0.007900290687139989")
	stakeWeights[addrs[10]] = calculateWeight("99999")
	stakeWeights[addrs[11]] = calculateWeight("0.000000000000000989")

	pool := common.Address{0xff}
	appState.State.SetDelegatee(addrs[4], pool)
	appState.State.SetDelegatee(addrs[5], pool)
	appState.State.SetDelegatee(addrs[6], pool)

	validationResults := map[common.ShardId]*types.ValidationResults{
		1: {
			GoodInviters: map[common.Address]*types.InviterValidationResult{
				addrs[0]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Undefined), SuccessfulInvites: []*types.SuccessfulInvite{
						{1, common.Hash{}, 0, true, common.Address{0xa1}},
						{2, common.Hash{}, 999999, false, common.Address{0xa2}},
					},
				},
				addrs[1]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Verified), SuccessfulInvites: []*types.SuccessfulInvite{
						{2, common.Hash{}, 3000, true, common.Address{0xa3}},
						{3, common.Hash{}, 70000, true, common.Address{0xa4}},
					},
				},
				addrs[2]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Newbie), SuccessfulInvites: []*types.SuccessfulInvite{
						{3, common.Hash{}, 10000, true, common.Address{0xa5}},
						{1, common.Hash{}, 30000, false, common.Address{0xa6}},
					},
				},
				addrs[3]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Human), SuccessfulInvites: []*types.SuccessfulInvite{
						{2, common.Hash{}, 15000, false, common.Address{0xa7}},
						{2, common.Hash{}, 25000, false, common.Address{0xa8}},
					},
				},
				addrs[4]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Verified), SuccessfulInvites: []*types.SuccessfulInvite{
						{2, common.Hash{}, 3000, true, common.Address{0xa9}},
						{3, common.Hash{}, 70000, true, common.Address{0xaa}},
					},
				},
				addrs[5]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Newbie), SuccessfulInvites: []*types.SuccessfulInvite{
						{3, common.Hash{}, 10000, true, common.Address{0xab}},
						{1, common.Hash{}, 30000, false, common.Address{0xac}},
					},
				},
				addrs[6]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Human), SuccessfulInvites: []*types.SuccessfulInvite{
						{2, common.Hash{}, 15000, false, common.Address{0xad}},
						{2, common.Hash{}, 25000, false, common.Address{0xae}},
					},
				},
				addrs[7]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Verified), SuccessfulInvites: []*types.SuccessfulInvite{
						{1, common.Hash{}, 33333, true, common.Address{0xaf}},
					},
				},
				addrs[8]: {
					PayInvitationReward: false, NewIdentityState: uint8(state.Newbie), SuccessfulInvites: []*types.SuccessfulInvite{
						{2, common.Hash{}, 33333, false, common.Address{0xb0}},
					},
				},
				addrs[9]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Human), SuccessfulInvites: []*types.SuccessfulInvite{
						{3, common.Hash{}, 33333, false, common.Address{0xb1}},
					},
				},
				addrs[10]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Verified), SuccessfulInvites: []*types.SuccessfulInvite{},
				},
				addrs[11]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Newbie), SuccessfulInvites: []*types.SuccessfulInvite{
						{2, common.Hash{}, 33333, false, common.Address{0xb3}},
					},
				},
				addrs[12]: {
					PayInvitationReward: true, NewIdentityState: uint8(state.Human), SuccessfulInvites: []*types.SuccessfulInvite{
						{3, common.Hash{}, 33333, false, common.Address{0xb4}},
					},
				},
			},
		},
	}

	totalReward := new(big.Int).Mul(new(big.Int).SetInt64(554454), common.DnaBase)
	totalRewardD := decimal.NewFromBigInt(totalReward, 0)

	epochDurations := []uint32{72644, 92409, 86329}

	addInvitationReward(appState, cfg, validationResults, totalRewardD, epochDurations, stakeWeights, nil)

	require.Equal(t, "2.660363363372324272", ConvertToFloat(appState.State.GetBalance(addrs[0])).String())
	require.Equal(t, "0.665090840843081067", ConvertToFloat(appState.State.GetStakeBalance(addrs[0])).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xa1}).Sign())
	require.Zero(t, appState.State.GetStakeBalance(common.Address{0xa1}).Sign())
	require.Zero(t, appState.State.GetBalance(common.Address{0xa2}).Sign())
	require.Equal(t, "1.847474595010569582", ConvertToFloat(appState.State.GetStakeBalance(common.Address{0xa2})).String())

	require.Equal(t, "62245.168908884489584627", ConvertToFloat(appState.State.GetBalance(addrs[1])).String())
	require.Equal(t, "15561.292227221122396156", ConvertToFloat(appState.State.GetStakeBalance(addrs[1])).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xa3}).Sign())
	require.Zero(t, appState.State.GetStakeBalance(common.Address{0xa3}).Sign())
	require.Zero(t, appState.State.GetBalance(common.Address{0xa4}).Sign())
	require.Zero(t, appState.State.GetStakeBalance(common.Address{0xa4}).Sign())

	require.Equal(t, "1294.297600637698061009", ConvertToFloat(appState.State.GetBalance(addrs[2])).String())
	require.Equal(t, "5177.190402550792244031", ConvertToFloat(appState.State.GetStakeBalance(addrs[2])).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xa5}).Sign())
	require.Zero(t, appState.State.GetStakeBalance(common.Address{0xa5}).Sign())
	require.Zero(t, appState.State.GetBalance(common.Address{0xa6}).Sign())
	require.Equal(t, "5147.686539282495524245", ConvertToFloat(appState.State.GetStakeBalance(common.Address{0xa6})).String())

	require.Equal(t, "2681.44154436890331253", ConvertToFloat(appState.State.GetBalance(addrs[3])).String())
	require.Equal(t, "670.360386092225828132", ConvertToFloat(appState.State.GetStakeBalance(addrs[3])).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xa7}).Sign())
	require.Equal(t, "1677.857398311075623942", ConvertToFloat(appState.State.GetStakeBalance(common.Address{0xa7})).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xa8}).Sign())
	require.Equal(t, "1673.94453215005351672", ConvertToFloat(appState.State.GetStakeBalance(common.Address{0xa8})).String())

	require.Equal(t, "1651.83896803968129109", ConvertToFloat(appState.State.GetBalance(common.Address{0xff})).String())
	require.Zero(t, appState.State.GetStakeBalance(common.Address{0xff}).Sign())

	require.Zero(t, appState.State.GetBalance(addrs[4]).Sign())
	require.Equal(t, "313.905685117772372841", ConvertToFloat(appState.State.GetStakeBalance(addrs[4])).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xa9}).Sign())
	require.Zero(t, appState.State.GetStakeBalance(common.Address{0xa9}).Sign())
	require.Zero(t, appState.State.GetBalance(common.Address{0xaa}).Sign())
	require.Zero(t, appState.State.GetStakeBalance(common.Address{0xaa}).Sign())

	require.Zero(t, appState.State.GetBalance(addrs[5]).Sign())
	require.Equal(t, "682.790776141615700033", ConvertToFloat(appState.State.GetStakeBalance(addrs[5])).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xab}).Sign())
	require.Zero(t, appState.State.GetStakeBalance(common.Address{0xab}).Sign())
	require.Zero(t, appState.State.GetBalance(common.Address{0xac}).Sign())
	require.Equal(t, "678.899670146385092848", ConvertToFloat(appState.State.GetStakeBalance(common.Address{0xac})).String())

	require.Zero(t, appState.State.GetBalance(addrs[6]).Sign())
	require.Equal(t, "56.379633383296968678", ConvertToFloat(appState.State.GetStakeBalance(addrs[6])).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xad}).Sign())
	require.Equal(t, "141.11363524819448702", ConvertToFloat(appState.State.GetStakeBalance(common.Address{0xad})).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xae}).Sign())
	require.Equal(t, "140.784531668290356371", ConvertToFloat(appState.State.GetStakeBalance(common.Address{0xae})).String())

	require.Equal(t, "1.278609777010080814", ConvertToFloat(appState.State.GetBalance(addrs[7])).String())
	require.Equal(t, "0.319652444252520203", ConvertToFloat(appState.State.GetStakeBalance(addrs[7])).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xaf}).Sign())
	require.Zero(t, appState.State.GetStakeBalance(common.Address{0xaf}).Sign())

	require.Equal(t, "0.001786500169903872", ConvertToFloat(appState.State.GetBalance(addrs[9])).String())
	require.Equal(t, "0.000446625042475968", ConvertToFloat(appState.State.GetStakeBalance(addrs[9])).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xb1}).Sign())
	require.Equal(t, "0.000558281147219847", ConvertToFloat(appState.State.GetStakeBalance(common.Address{0xb1})).String())

	require.Equal(t, "0.000000000000000692", ConvertToFloat(appState.State.GetBalance(addrs[11])).String())
	require.Equal(t, "0.000000000000002764", ConvertToFloat(appState.State.GetStakeBalance(addrs[11])).String())
	require.Zero(t, appState.State.GetBalance(common.Address{0xb3}).Sign())
	require.Equal(t, "0.000000000000003456", ConvertToFloat(appState.State.GetStakeBalance(common.Address{0xb3})).String())
}

func Test_splitFlipsToReward(t *testing.T) {
	src := []*types.FlipToReward{
		{
			GradeScore: decimal.NewFromInt32(2),
			Cid:        []byte{0x1},
		},
		{
			GradeScore: decimal.NewFromInt32(2),
			Cid:        []byte{0x2},
		},
		{
			GradeScore: decimal.NewFromInt32(8),
			Cid:        []byte{0x3},
		},
		{
			GradeScore: decimal.NewFromInt32(6),
			Cid:        []byte{0x4},
		},
	}

	base, extra := splitFlipsToReward(src, true)
	require.Len(t, base, 3)
	require.Len(t, extra, 1)

	require.Equal(t, decimal.NewFromInt32(8), base[0].GradeScore)
	require.Equal(t, decimal.NewFromInt32(6), base[1].GradeScore)
	require.Equal(t, decimal.NewFromInt32(2), base[2].GradeScore)
	require.Equal(t, []byte{0x1}, base[2].Cid)

	require.Equal(t, decimal.NewFromInt32(2), extra[0].GradeScore)
	require.Equal(t, []byte{0x2}, extra[0].Cid)
}
