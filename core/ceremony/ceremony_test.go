package ceremony

import (
	"crypto/ecdsa"
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"testing"
)

func TestValidationCeremony_getFlipsToSolve(t *testing.T) {
	require := require.New(t)

	flipsCids := [][]byte{{0x1}, {0x2}, {0x3}, {0x4}, {0x5}}

	fliptsPerCandidate := [][]int{{0, 1, 2}, {4, 2, 1}, {1, 2, 3}, {1, 2, 4}, {0, 1, 3}}

	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	result := getFlipsToSolve(addr, getParticipants(key, 0, 5), fliptsPerCandidate, flipsCids)
	shouldBe := [][]byte{{0x1}, {0x2}, {0x3}}
	require.Equal(shouldBe, result)

	result = getFlipsToSolve(addr, getParticipants(key, 3, 5), fliptsPerCandidate, flipsCids)
	shouldBe = [][]byte{{0x2}, {0x3}, {0x5}}
	require.Equal(shouldBe, result)

	result = getFlipsToSolve(addr, getParticipants(key, 4, 5), fliptsPerCandidate, flipsCids)
	shouldBe = [][]byte{{0x1}, {0x2}, {0x4}}
	require.Equal(shouldBe, result)
}

func TestValidationCeremony_getFlipsToSolve_fewFlips(t *testing.T) {
	require := require.New(t)

	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	flipsCids := [][]byte{{0x1}, {0x2}, {0x3}, {0x4}, {0x5}}

	fliptsPerCandidate := [][]int{{0, 1, 6}, {4, 2, 8}, {1, 2, 4}, {1, 2, 3}, {6, 7, 8}}

	result := getFlipsToSolve(addr, getParticipants(key, 0, 5), fliptsPerCandidate, flipsCids)
	shouldBe := [][]byte{{0x1}, {0x2}, {0x2}}
	require.Equal(shouldBe, result)

	result = getFlipsToSolve(addr, getParticipants(key, 4, 5), fliptsPerCandidate, flipsCids)
	shouldBe = [][]byte{{0x2}, {0x3}, {0x4}}
	require.Equal(shouldBe, result)
}

func getParticipants(myKey *ecdsa.PrivateKey, myIndex int, length int) []*candidate {
	participants := make([]*candidate, 0)

	for i := 0; i < length; i++ {
		if i == myIndex {
			participants = append(participants, &candidate{
				Address: crypto.PubkeyToAddress(myKey.PublicKey),
				PubKey:  crypto.FromECDSAPub(&myKey.PublicKey),
			})
		} else {
			key, _ := crypto.GenerateKey()

			participants = append(participants, &candidate{
				Address: crypto.PubkeyToAddress(key.PublicKey),
				PubKey:  crypto.FromECDSAPub(&key.PublicKey),
			})
		}
	}
	return participants
}

func Test_determineNewIdentityState(t *testing.T) {

	type data struct {
		prev                state.IdentityState
		shortScore          float32
		longScore           float32
		totalScore          float32
		totalQualifiedFlips uint32
		missed              bool
		expected            state.IdentityState
		noQualShort         bool
		noQualLong          bool
	}

	cases := []data{
		{
			state.Killed,
			0, 0, 0, 0, true,
			state.Killed, false, false,
		},
		{
			state.Invite,
			1, 1, 1, 110, false,
			state.Killed, false, false,
		},
		{
			state.Candidate,
			common.MinShortScore, common.MinLongScore, common.MinTotalScore, 11, false,
			state.Newbie, false, false,
		},
		{
			state.Candidate,
			common.MinShortScore, common.MinLongScore, common.MinTotalScore, 11, true,
			state.Killed, false, false,
		},
		{
			state.Newbie,
			common.MinShortScore, common.MinLongScore, common.MinTotalScore, 11, false,
			state.Newbie, false, false,
		},
		{
			state.Newbie,
			common.MinShortScore, common.MinLongScore, common.MinTotalScore, 13, false,
			state.Verified, false, false,
		},
		{
			state.Newbie,
			common.MinShortScore, common.MinLongScore, common.MinTotalScore, 10, false,
			state.Newbie, false, false,
		},
		{
			state.Newbie,
			common.MinShortScore, common.MinLongScore, common.MinTotalScore, 11, true,
			state.Killed, false, false,
		},
		{
			state.Newbie,
			0.4, 0.8, 1, 11, false,
			state.Killed, false, false,
		},
		{
			state.Newbie,
			common.MinShortScore, common.MinLongScore, common.MinTotalScore, 8, false,
			state.Newbie, false, false,
		},
		{
			state.Verified,
			common.MinShortScore, common.MinLongScore, common.MinTotalScore, 10, false,
			state.Killed, false, false,
		},
		{
			state.Verified,
			0, 0, 0, 0, true,
			state.Suspended, false, false,
		},
		{
			state.Verified,
			0, 0, 0, 0, false,
			state.Killed, false, false,
		},
		{
			state.Suspended,
			common.MinShortScore, common.MinLongScore, common.MinTotalScore, 10, false,
			state.Verified, false, false,
		},
		{
			state.Suspended,
			1, 0.8, 0, 10, true,
			state.Zombie, false, false,
		},
		{
			state.Zombie,
			common.MinShortScore, 0, common.MinTotalScore, 10, false,
			state.Verified, false, false,
		},
		{
			state.Zombie,
			1, 0, 0, 10, true,
			state.Killed, false, false,
		},
		{
			state.Candidate,
			common.MinShortScore, 0, 0, 5, false,
			state.Candidate, true, false,
		},
		{
			state.Candidate,
			common.MinShortScore - 0.1, 0, 0, 5, false,
			state.Killed, false, true,
		},
		{
			state.Newbie,
			common.MinShortScore, 0, 0.1, 5, false,
			state.Newbie, true, false,
		},
		{
			state.Newbie,
			common.MinShortScore, 0, 0.1, 5, false,
			state.Newbie, false, true,
		},
		{
			state.Newbie,
			common.MinShortScore, 0, 0.1, 13, false,
			state.Killed, false, true,
		},
		{
			state.Newbie,
			common.MinShortScore - 0.1, 0, 0.1, 9, false,
			state.Killed, false, true,
		},
		{
			state.Verified,
			common.MinShortScore - 0.1, 0, 0.1, 10, false,
			state.Verified, true, false,
		},
		{
			state.Verified,
			common.MinShortScore - 0.1, 0, 1.1, 10, false,
			state.Killed, false, true,
		},
		{
			state.Suspended,
			common.MinShortScore - 0.1, 0, 0.1, 10, false,
			state.Suspended, true, false,
		},
		{
			state.Suspended,
			common.MinShortScore - 0.1, 0, 1.1, 10, false,
			state.Killed, false, true,
		},
		{
			state.Zombie,
			common.MinShortScore - 0.1, 0, 0.1, 10, false,
			state.Zombie, true, false,
		},
		{
			state.Zombie,
			common.MinShortScore, 0, 0.1, 10, false,
			state.Killed, false, true,
		},
		{
			state.Suspended,
			common.MinShortScore, common.MinLongScore, common.MinHumanTotalScore, 24, false,
			state.Human, false, false,
		},
		{
			state.Suspended,
			common.MinShortScore, common.MinLongScore, common.MinHumanTotalScore, 23, false,
			state.Verified, false, false,
		},
		{
			state.Zombie,
			common.MinShortScore, common.MinLongScore, common.MinHumanTotalScore, 24, false,
			state.Human, false, false,
		},
		{
			state.Zombie,
			common.MinShortScore, common.MinLongScore, common.MinHumanTotalScore, 23, false,
			state.Verified, false, false,
		},
		{
			state.Human,
			0.1, common.MinLongScore, common.MinHumanTotalScore, 24, false,
			state.Suspended, false, false,
		},
		{
			state.Human,
			common.MinShortScore, 0.1, common.MinHumanTotalScore, 24, false,
			state.Suspended, false, false,
		},
		{
			state.Human,
			0.1, 0.1, common.MinHumanTotalScore, 24, false,
			state.Suspended, false, true,
		},
		{
			state.Human,
			common.MinShortScore, common.MinLongScore, common.MinHumanTotalScore, 24, false,
			state.Human, false, true,
		},
		{
			state.Human,
			common.MinShortScore, common.MinLongScore, common.MinHumanTotalScore, 24, true,
			state.Suspended, false, false,
		},
		{
			state.Human,
			common.MinShortScore, common.MinLongScore, common.MinHumanTotalScore, 24, false,
			state.Human, true, true,
		},
		{
			state.Verified,
			common.MinShortScore, common.MinLongScore, common.MinHumanTotalScore, 24, false,
			state.Human, false, false,
		},
		{
			state.Human,
			0, 0, common.MinHumanTotalScore, 24, false,
			state.Suspended, false, false,
		},

		{
			state.Human,
			0, 0, 0.74, 24, false,
			state.Killed, false, false,
		},
	}

	require := require.New(t)
	for i, c := range cases {
		require.Equal(c.expected, determineNewIdentityState(state.Identity{State: c.prev}, c.shortScore, c.longScore, c.totalScore, c.totalQualifiedFlips, c.missed, c.noQualShort, c.noQualLong), "index = %v", i)
	}
}

func Test_getNotApprovedFlips(t *testing.T) {
	// given
	vc := ValidationCeremony{}
	_, app, _, _ := blockchain.NewTestBlockchain(false, make(map[common.Address]config.GenesisAllocation))
	var candidates []*candidate
	var flipsPerAuthor map[int][][]byte
	var flips [][]byte
	for i := 0; i < 3; i++ {
		key, _ := crypto.GenerateKey()
		c := candidate{
			Address: crypto.PubkeyToAddress(key.PublicKey),
		}
		candidates = append(candidates, &c)
	}
	for i := 0; i < 5; i++ {
		flips = append(flips, []byte{byte(i)})
	}
	flipsPerAuthor = make(map[int][][]byte)
	flipsPerAuthor[0] = [][]byte{
		flips[0],
		flips[1],
		flips[2],
	}
	flipsPerAuthor[1] = [][]byte{
		flips[3],
	}
	flipsPerAuthor[2] = [][]byte{
		flips[4],
	}
	addr := candidates[0].Address
	app.State.SetRequiredFlips(addr, 3)
	approvedAddr := candidates[1].Address
	app.State.SetRequiredFlips(approvedAddr, 3)

	vc.shardCandidates = map[common.ShardId]*candidatesOfShard{0: &candidatesOfShard{
		candidates:     candidates,
		flipsPerAuthor: flipsPerAuthor,
		flips:          flips,
	}}
	vc.appState = app

	approvedCandidates := mapset.NewSet()
	approvedCandidates.Add(approvedAddr)

	// when
	result := vc.getNotApprovedFlips(approvedCandidates, 0)

	// then
	r := require.New(t)
	r.Equal(3, result.Cardinality())
	r.True(result.Contains(0))
	r.True(result.Contains(1))
	r.True(result.Contains(2))
}

func Test_flipPos(t *testing.T) {
	flips := [][]byte{
		{1, 2, 3},
		{1, 2, 3, 4},
		{2, 3, 4},
	}
	r := require.New(t)
	r.Equal(-1, flipPos(flips, []byte{1, 2, 3, 4, 5}))
	r.Equal(0, flipPos(flips, []byte{1, 2, 3}))
	r.Equal(1, flipPos(flips, []byte{1, 2, 3, 4}))
	r.Equal(2, flipPos(flips, []byte{2, 3, 4}))
}

func Test_analyzeAuthors(t *testing.T) {
	const enableUpgrade7 = false
	vc := ValidationCeremony{}

	auth1 := common.Address{1}
	auth2 := common.Address{2}
	auth3 := common.Address{3}
	auth4 := common.Address{4}
	auth5 := common.Address{5}
	auth6 := common.Address{6}
	auth7 := common.Address{7}
	auth8 := common.Address{8}
	auth9 := common.Address{9}
	auth10 := common.Address{10}
	auth11 := common.Address{11}

	reporter1 := common.Address{12}
	reporter2 := common.Address{13}
	reporter3 := common.Address{14}

	vc.shardCandidates = map[common.ShardId]*candidatesOfShard{0: {
		flips: [][]byte{{0x0}, {0x1}, {0x2}, {0x3}, {0x4}, {0x5}, {0x6}, {0x7}, {0x8}, {0x9}, {0xa}, {0xb}, {0xc},
			{0xd}, {0xe}, {0xf}, {0x10}, {0x11}, {0x12}, {0x13}, {0x14}, {0x15}, {0x16}},
		flipAuthorMap: map[string]common.Address{
			string([]byte{0x0}): auth1,
			string([]byte{0x1}): auth1,
			string([]byte{0x2}): auth1,

			string([]byte{0x3}): auth2,
			string([]byte{0x4}): auth2,

			string([]byte{0x5}): auth3,
			string([]byte{0x6}): auth3,

			string([]byte{0x7}): auth4,
			string([]byte{0x8}): auth4,

			string([]byte{0x9}): auth5,

			string([]byte{0xa}): auth6,
			string([]byte{0xb}): auth6,
			string([]byte{0xc}): auth6,

			string([]byte{0xd}): auth7,
			string([]byte{0xe}): auth7,

			string([]byte{0xf}):  auth8,
			string([]byte{0x10}): auth8,

			string([]byte{0x11}): auth9,
			string([]byte{0x12}): auth9,

			string([]byte{0x13}): auth10,
			string([]byte{0x14}): auth10,

			string([]byte{0x15}): auth11,
			string([]byte{0x16}): auth11,
		},
	}}

	qualification := []FlipQualification{
		{status: Qualified, grade: types.GradeD},
		{status: WeaklyQualified, grade: types.GradeC},
		{status: NotQualified, grade: types.GradeD},

		{status: QualifiedByNone, grade: types.GradeD},
		{status: Qualified, grade: types.GradeD},

		{status: WeaklyQualified, grade: types.GradeReported},
		{status: Qualified, grade: types.GradeD},

		{status: NotQualified, grade: types.GradeD},
		{status: NotQualified, grade: types.GradeD},

		{status: QualifiedByNone, grade: types.GradeD},

		{status: Qualified, grade: types.GradeD},
		{status: WeaklyQualified, grade: types.GradeD},
		{status: Qualified, grade: types.GradeD},

		{status: NotQualified, grade: types.GradeReported},
		{status: Qualified, grade: types.GradeA},

		{status: Qualified, grade: types.GradeReported},
		{status: NotQualified, grade: types.GradeA},

		{status: QualifiedByNone, grade: types.GradeReported},
		{status: Qualified, grade: types.GradeA},

		{status: NotQualified, grade: types.GradeReported},
		{status: NotQualified, grade: types.GradeC},

		{status: WeaklyQualified, grade: types.GradeReported},
		{status: QualifiedByNone, grade: types.GradeA},
	}
	reporters := newReportersToReward()
	reporters.addReport(5, reporter1, enableUpgrade7)
	reporters.addReport(5, auth2, enableUpgrade7)
	reporters.addReport(13, reporter1, enableUpgrade7)
	reporters.addReport(13, reporter2, enableUpgrade7)
	reporters.addReport(15, reporter1, enableUpgrade7)
	reporters.addReport(15, reporter3, enableUpgrade7)
	reporters.addReport(15, auth2, enableUpgrade7)
	reporters.addReport(17, reporter1, enableUpgrade7)
	reporters.addReport(17, reporter2, enableUpgrade7)
	reporters.addReport(17, reporter3, enableUpgrade7)
	reporters.addReport(19, reporter2, enableUpgrade7)
	reporters.addReport(19, reporter3, enableUpgrade7)
	reporters.addReport(21, reporter1, enableUpgrade7)
	reporters.addReport(21, reporter2, enableUpgrade7)

	conf := &config.ConsensusConf{}
	bad, good, authorResults, madeFlips, reporters := vc.analyzeAuthors(qualification, reporters, 0, conf)

	require.Contains(t, bad, auth2)
	require.Contains(t, bad, auth3)
	require.Contains(t, bad, auth4)
	require.Contains(t, bad, auth5)
	require.Contains(t, bad, auth7)
	require.Contains(t, bad, auth8)
	require.Contains(t, bad, auth9)
	require.Contains(t, bad, auth10)
	require.NotContains(t, bad, auth1)
	require.NotContains(t, bad, auth6)

	require.Contains(t, good, auth1)
	require.Equal(t, 2, len(good[auth1].FlipsToReward))
	require.Equal(t, types.GradeD, good[auth1].FlipsToReward[0].Grade)
	require.Equal(t, []byte{0x0}, good[auth1].FlipsToReward[0].Cid)
	require.Equal(t, types.GradeC, good[auth1].FlipsToReward[1].Grade)
	require.Equal(t, []byte{0x1}, good[auth1].FlipsToReward[1].Cid)

	require.True(t, authorResults[auth1].HasOneNotQualifiedFlip)
	require.False(t, authorResults[auth1].AllFlipsNotQualified)
	require.False(t, authorResults[auth1].HasOneReportedFlip)

	require.False(t, authorResults[auth6].HasOneNotQualifiedFlip)
	require.False(t, authorResults[auth6].AllFlipsNotQualified)
	require.False(t, authorResults[auth6].HasOneReportedFlip)

	require.False(t, authorResults[auth3].HasOneNotQualifiedFlip)
	require.False(t, authorResults[auth3].AllFlipsNotQualified)
	require.True(t, authorResults[auth3].HasOneReportedFlip)

	require.True(t, authorResults[auth4].HasOneNotQualifiedFlip)
	require.True(t, authorResults[auth4].AllFlipsNotQualified)
	require.False(t, authorResults[auth4].HasOneReportedFlip)

	require.False(t, authorResults[auth9].HasOneNotQualifiedFlip)
	require.False(t, authorResults[auth9].AllFlipsNotQualified)
	require.True(t, authorResults[auth9].HasOneReportedFlip)

	require.Equal(t, 11, len(madeFlips))

	require.Equal(t, 2, len(reporters.reportersByFlip))
	require.Equal(t, 1, len(reporters.reportersByFlip[5]))
	require.Equal(t, reporter1, reporters.reportersByFlip[5][reporter1].Address)
	require.Equal(t, 2, len(reporters.reportersByFlip[15]))
	require.Equal(t, reporter1, reporters.reportersByFlip[15][reporter1].Address)
	require.Equal(t, reporter3, reporters.reportersByFlip[15][reporter3].Address)

	require.Equal(t, 2, len(reporters.reportersByAddr))
	require.Equal(t, reporter1, reporters.reportersByAddr[reporter1].Address)
	require.Equal(t, reporter3, reporters.reportersByAddr[reporter3].Address)

	require.Equal(t, 2, len(reporters.reportedFlipsByReporter))
	require.Equal(t, 2, len(reporters.reportedFlipsByReporter[reporter1]))
	require.Contains(t, reporters.reportedFlipsByReporter[reporter1], 5)
	require.Contains(t, reporters.reportedFlipsByReporter[reporter1], 15)
	require.Equal(t, 1, len(reporters.reportedFlipsByReporter[reporter3]))
	require.Contains(t, reporters.reportedFlipsByReporter[reporter3], 15)

}

func Test_analyzeAuthors2(t *testing.T) {
	const enableUpgrade7 = false
	vc := ValidationCeremony{}

	auth1 := common.Address{1}
	auth2 := common.Address{2}
	auth3 := common.Address{3}
	auth4 := common.Address{4}
	auth5 := common.Address{5}
	auth6 := common.Address{6}
	auth7 := common.Address{7}
	auth8 := common.Address{8}
	auth9 := common.Address{9}
	auth10 := common.Address{10}
	auth11 := common.Address{11}

	reporter1 := common.Address{12}
	reporter2 := common.Address{13}
	reporter3 := common.Address{14}

	vc.shardCandidates = map[common.ShardId]*candidatesOfShard{0: {
		flips: [][]byte{{0x0}, {0x1}, {0x2}, {0x3}, {0x4}, {0x5}, {0x6}, {0x7}, {0x8}, {0x9}, {0xa}, {0xb}, {0xc},
			{0xd}, {0xe}, {0xf}, {0x10}, {0x11}, {0x12}, {0x13}, {0x14}, {0x15}, {0x16}},
		flipAuthorMap: map[string]common.Address{
			string([]byte{0x0}): auth1,
			string([]byte{0x1}): auth1,
			string([]byte{0x2}): auth1,

			string([]byte{0x3}): auth2,
			string([]byte{0x4}): auth2,

			string([]byte{0x5}): auth3,
			string([]byte{0x6}): auth3,

			string([]byte{0x7}): auth4,
			string([]byte{0x8}): auth4,

			string([]byte{0x9}): auth5,

			string([]byte{0xa}): auth6,
			string([]byte{0xb}): auth6,
			string([]byte{0xc}): auth6,

			string([]byte{0xd}): auth7,
			string([]byte{0xe}): auth7,

			string([]byte{0xf}):  auth8,
			string([]byte{0x10}): auth8,

			string([]byte{0x11}): auth9,
			string([]byte{0x12}): auth9,

			string([]byte{0x13}): auth10,
			string([]byte{0x14}): auth10,

			string([]byte{0x15}): auth11,
			string([]byte{0x16}): auth11,
		},
	}}

	qualification := []FlipQualification{
		{status: Qualified, grade: types.GradeD},
		{status: WeaklyQualified, grade: types.GradeC},
		{status: NotQualified, grade: types.GradeD},

		{status: QualifiedByNone, grade: types.GradeD},
		{status: Qualified, grade: types.GradeD},

		{status: WeaklyQualified, grade: types.GradeReported},
		{status: Qualified, grade: types.GradeD},

		{status: NotQualified, grade: types.GradeD},
		{status: NotQualified, grade: types.GradeD},

		{status: QualifiedByNone, grade: types.GradeD},

		{status: Qualified, grade: types.GradeD},
		{status: WeaklyQualified, grade: types.GradeD},
		{status: Qualified, grade: types.GradeD},

		{status: NotQualified, grade: types.GradeReported},
		{status: Qualified, grade: types.GradeA},

		{status: Qualified, grade: types.GradeReported},
		{status: NotQualified, grade: types.GradeA},

		{status: QualifiedByNone, grade: types.GradeReported},
		{status: Qualified, grade: types.GradeA},

		{status: NotQualified, grade: types.GradeReported},
		{status: NotQualified, grade: types.GradeC},

		{status: WeaklyQualified, grade: types.GradeReported},
		{status: QualifiedByNone, grade: types.GradeA},
	}
	reporters := newReportersToReward()
	reporters.addReport(5, reporter1, enableUpgrade7)
	reporters.addReport(5, auth2, enableUpgrade7)
	reporters.addReport(13, reporter1, enableUpgrade7)
	reporters.addReport(13, reporter2, enableUpgrade7)
	reporters.addReport(15, reporter1, enableUpgrade7)
	reporters.addReport(15, reporter3, enableUpgrade7)
	reporters.addReport(15, auth2, enableUpgrade7)
	reporters.addReport(17, reporter1, enableUpgrade7)
	reporters.addReport(17, reporter2, enableUpgrade7)
	reporters.addReport(17, reporter3, enableUpgrade7)
	reporters.addReport(19, reporter2, enableUpgrade7)
	reporters.addReport(19, reporter3, enableUpgrade7)
	reporters.addReport(21, reporter1, enableUpgrade7)
	reporters.addReport(21, reporter2, enableUpgrade7)

	conf := &config.ConsensusConf{
		ReportsRewardPercent: 0.01,
	}
	bad, good, authorResults, madeFlips, reporters := vc.analyzeAuthors(qualification, reporters, 0, conf)

	require.Contains(t, bad, auth2)
	require.Contains(t, bad, auth3)
	require.Contains(t, bad, auth4)
	require.Contains(t, bad, auth5)
	require.Contains(t, bad, auth7)
	require.Contains(t, bad, auth8)
	require.Contains(t, bad, auth9)
	require.Contains(t, bad, auth10)
	require.NotContains(t, bad, auth1)
	require.NotContains(t, bad, auth6)

	require.Contains(t, good, auth1)
	require.Equal(t, 2, len(good[auth1].FlipsToReward))
	require.Equal(t, types.GradeD, good[auth1].FlipsToReward[0].Grade)
	require.Equal(t, []byte{0x0}, good[auth1].FlipsToReward[0].Cid)
	require.Equal(t, types.GradeC, good[auth1].FlipsToReward[1].Grade)
	require.Equal(t, []byte{0x1}, good[auth1].FlipsToReward[1].Cid)

	require.True(t, authorResults[auth1].HasOneNotQualifiedFlip)
	require.False(t, authorResults[auth1].AllFlipsNotQualified)
	require.False(t, authorResults[auth1].HasOneReportedFlip)

	require.False(t, authorResults[auth6].HasOneNotQualifiedFlip)
	require.False(t, authorResults[auth6].AllFlipsNotQualified)
	require.False(t, authorResults[auth6].HasOneReportedFlip)

	require.False(t, authorResults[auth3].HasOneNotQualifiedFlip)
	require.False(t, authorResults[auth3].AllFlipsNotQualified)
	require.True(t, authorResults[auth3].HasOneReportedFlip)

	require.True(t, authorResults[auth4].HasOneNotQualifiedFlip)
	require.True(t, authorResults[auth4].AllFlipsNotQualified)
	require.False(t, authorResults[auth4].HasOneReportedFlip)

	require.False(t, authorResults[auth9].HasOneNotQualifiedFlip)
	require.False(t, authorResults[auth9].AllFlipsNotQualified)
	require.True(t, authorResults[auth9].HasOneReportedFlip)

	require.Equal(t, 11, len(madeFlips))

	require.Equal(t, 6, len(reporters.reportersByFlip))
	require.Equal(t, 1, len(reporters.reportersByFlip[5]))
	require.Equal(t, reporter1, reporters.reportersByFlip[5][reporter1].Address)
	require.Equal(t, 2, len(reporters.reportersByFlip[15]))
	require.Equal(t, reporter1, reporters.reportersByFlip[15][reporter1].Address)
	require.Equal(t, reporter3, reporters.reportersByFlip[15][reporter3].Address)

	require.Equal(t, 3, len(reporters.reportersByAddr))
	require.Equal(t, reporter1, reporters.reportersByAddr[reporter1].Address)
	require.Equal(t, reporter2, reporters.reportersByAddr[reporter2].Address)
	require.Equal(t, reporter3, reporters.reportersByAddr[reporter3].Address)

	require.Equal(t, 3, len(reporters.reportedFlipsByReporter))
	require.Equal(t, 5, len(reporters.reportedFlipsByReporter[reporter1]))
	require.Equal(t, 4, len(reporters.reportedFlipsByReporter[reporter2]))
	require.Equal(t, 3, len(reporters.reportedFlipsByReporter[reporter3]))
}

func Test_incSuccessfulInvites_beforeUpgrade7(t *testing.T) {
	epoch := uint16(5)
	god := common.Address{0x1}
	auth1 := common.Address{0x2}
	badAuth := common.Address{0x3}

	validationResults := &types.ValidationResults{
		BadAuthors:   map[common.Address]types.BadAuthorReason{badAuth: types.WrongWordsBadAuthor},
		GoodInviters: make(map[common.Address]*types.InviterValidationResult),
	}

	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Verified,
		Inviter: &state.Inviter{
			Address: god,
		},
	}, 0, state.Newbie, epoch, map[common.Address]*types.InviterValidationResult{}, false)

	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Candidate,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 5, state.Newbie, epoch, map[common.Address]*types.InviterValidationResult{}, false)

	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Candidate,
		Inviter: &state.Inviter{
			Address: badAuth,
		},
	}, 5, state.Newbie, epoch, map[common.Address]*types.InviterValidationResult{}, false)

	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Candidate,
		Inviter: &state.Inviter{
			Address: god,
		},
	}, 5, state.Newbie, epoch, map[common.Address]*types.InviterValidationResult{}, false)

	// 4th validation (Newbie->Newbie)
	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 2, state.Newbie, epoch, map[common.Address]*types.InviterValidationResult{}, false)

	// 4th validation (Newbie->Verified)
	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 2, state.Verified, epoch, map[common.Address]*types.InviterValidationResult{}, false)

	// 3rd validation (Newbie->Newbie)
	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 3, state.Newbie, epoch, map[common.Address]*types.InviterValidationResult{}, false)

	// 2nd validation (Newbie->Newbie)
	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 4, state.Newbie, epoch, map[common.Address]*types.InviterValidationResult{}, false)

	// 3rd validation (Newbie->Verified)
	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 3, state.Verified, epoch, map[common.Address]*types.InviterValidationResult{}, false)

	require.Equal(t, len(validationResults.GoodInviters[auth1].SuccessfulInvites), 4)
	var ages []uint16
	for _, si := range validationResults.GoodInviters[auth1].SuccessfulInvites {
		ages = append(ages, si.Age)
	}
	require.Equal(t, []uint16{1, 3, 2, 3}, ages)

	require.Equal(t, len(validationResults.GoodInviters[god].SuccessfulInvites), 1)
	require.Equal(t, uint16(1), validationResults.GoodInviters[god].SuccessfulInvites[0].Age)
	require.True(t, validationResults.GoodInviters[god].PayInvitationReward)
	require.NotContains(t, validationResults.GoodInviters, badAuth)
}

func Test_incSuccessfulInvites(t *testing.T) {
	allGoodInviters := make(map[common.Address]*types.InviterValidationResult)
	enableUpgrade7 := true

	epoch := uint16(5)
	god := common.Address{0x1}
	auth1 := common.Address{0x2}
	badAuth := common.Address{0x3}

	validationResults := &types.ValidationResults{
		BadAuthors:   map[common.Address]types.BadAuthorReason{badAuth: types.WrongWordsBadAuthor},
		GoodInviters: make(map[common.Address]*types.InviterValidationResult),
	}

	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Verified,
		Inviter: &state.Inviter{
			Address: god,
		},
	}, 0, state.Newbie, epoch, allGoodInviters, enableUpgrade7)

	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Candidate,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 5, state.Newbie, epoch, allGoodInviters, enableUpgrade7)

	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Candidate,
		Inviter: &state.Inviter{
			Address: badAuth,
		},
	}, 5, state.Newbie, epoch, allGoodInviters, enableUpgrade7)

	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Candidate,
		Inviter: &state.Inviter{
			Address: god,
		},
	}, 5, state.Newbie, epoch, allGoodInviters, enableUpgrade7)

	// 4th validation (Newbie->Newbie)
	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 2, state.Newbie, epoch, allGoodInviters, enableUpgrade7)

	// 4th validation (Newbie->Verified)
	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 2, state.Verified, epoch, allGoodInviters, enableUpgrade7)

	// 3rd validation (Newbie->Newbie)
	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 3, state.Newbie, epoch, allGoodInviters, enableUpgrade7)

	// 2nd validation (Newbie->Newbie)
	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 4, state.Newbie, epoch, allGoodInviters, enableUpgrade7)

	// 3rd validation (Newbie->Verified)
	incSuccessfulInvites(validationResults, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.Inviter{
			Address: auth1,
		},
	}, 3, state.Verified, epoch, allGoodInviters, enableUpgrade7)

	setValidationResultToGoodInviter(validationResults, auth1, state.Newbie, allGoodInviters, enableUpgrade7)
	setValidationResultToGoodInviter(validationResults, god, state.Human, allGoodInviters, enableUpgrade7)
	setValidationResultToGoodInviter(validationResults, badAuth, state.Human, allGoodInviters, enableUpgrade7)

	require.Equal(t, len(validationResults.GoodInviters[auth1].SuccessfulInvites), 4)
	var ages []uint16
	for _, si := range validationResults.GoodInviters[auth1].SuccessfulInvites {
		ages = append(ages, si.Age)
	}
	require.Equal(t, []uint16{1, 3, 2, 3}, ages)

	require.Equal(t, len(validationResults.GoodInviters[god].SuccessfulInvites), 1)
	require.Equal(t, uint16(1), validationResults.GoodInviters[god].SuccessfulInvites[0].Age)
	require.True(t, validationResults.GoodInviters[god].PayInvitationReward)
	require.NotContains(t, validationResults.GoodInviters, badAuth)
}

func Test_determineIdentityBirthday(t *testing.T) {
	identity := state.Identity{}
	identity.Birthday = 1
	identity.State = state.Newbie
	require.Equal(t, uint16(1), determineIdentityBirthday(2, identity, state.Newbie))
}

func Test_applyOnState(t *testing.T) {
	db := dbm.NewMemDB()
	appstate, _ := appstate.NewAppState(db, eventbus.New())

	addr1 := common.Address{0x1}
	delegatee := common.Address{0x2}
	appstate.State.SetState(addr1, state.Newbie)
	appstate.State.AddStake(addr1, big.NewInt(100))
	appstate.State.AddStake(addr1, big.NewInt(40))
	appstate.State.AddAddedStake(addr1, big.NewInt(40))
	appstate.State.AddBalance(addr1, big.NewInt(10))
	appstate.State.AddNewScore(addr1, common.EncodeScore(5, 6))
	appstate.State.SetDelegatee(addr1, delegatee)

	identities := applyOnState(config.ConsensusVersions[config.ConsensusV8], appstate, 0, collector.NewStatsCollector(), addr1, cacheValue{
		prevState:                state.Newbie,
		birthday:                 3,
		shortFlipPoint:           1,
		shortQualifiedFlipsCount: 2,
		state:                    state.Verified,
		delegatee:                &delegatee,
	})
	identity := appstate.State.GetIdentity(addr1)
	require.Equal(t, 1, identities)
	require.Equal(t, state.Verified, identity.State)
	require.Equal(t, uint16(3), identity.Birthday)
	require.Equal(t, []byte{common.EncodeScore(5, 6), common.EncodeScore(1, 2)}, identity.Scores)
	require.True(t, appstate.State.GetBalance(delegatee).Cmp(big.NewInt(75)) == 0)
	require.True(t, appstate.State.GetBalance(addr1).Cmp(big.NewInt(10)) == 0)
	require.True(t, appstate.State.GetStakeBalance(addr1).Cmp(big.NewInt(65)) == 0)
	require.True(t, appstate.State.GetAddedStakeBalance(addr1).Cmp(big.NewInt(40)) == 0)

	applyOnState(config.ConsensusVersions[config.ConsensusV8], appstate, 0, collector.NewStatsCollector(), addr1, cacheValue{
		shortFlipPoint:           0,
		shortQualifiedFlipsCount: 0,
		missed:                   true,
	})
	identity = appstate.State.GetIdentity(addr1)
	require.Equal(t, []byte{common.EncodeScore(5, 6), common.EncodeScore(1, 2)}, identity.Scores)

	appstate.State.SetDelegatee(delegatee, common.Address{0x3})

	applyOnState(config.ConsensusVersions[config.ConsensusV8], appstate, 0, collector.NewStatsCollector(), addr1, cacheValue{
		prevState:                state.Zombie,
		shortFlipPoint:           1,
		shortQualifiedFlipsCount: 2,
		state:                    state.Verified,
		delegatee:                &delegatee,
	})
	identity = appstate.State.GetIdentity(addr1)
	require.Nil(t, identity.Delegatee())
}

func Test_applyOnState_saveStake(t *testing.T) {
	db := dbm.NewMemDB()
	appState, _ := appstate.NewAppState(db, eventbus.New())

	addr := common.Address{0x1}
	appState.State.AddStake(addr, big.NewInt(100))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Newbie,
			birthday:     14,
			state:        state.Killed,
			participated: true,
		})
	require.Equal(t, big.NewInt(0), appState.State.GetBalance(addr))
	require.Equal(t, big.NewInt(100), appState.State.GetStakeBalance(addr))

	addr = common.Address{0x1, 0x1}
	appState.State.AddStake(addr, big.NewInt(100))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			participated: false,
			prevState:    state.Newbie,
			birthday:     14,
			state:        state.Killed,
		})
	require.Equal(t, big.NewInt(0), appState.State.GetBalance(addr))
	require.Equal(t, big.NewInt(100), appState.State.GetStakeBalance(addr))

	addr = common.Address{0x2}
	appState.State.AddStake(addr, big.NewInt(100))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Verified,
			birthday:     2,
			state:        state.Killed,
			participated: true,
		})
	require.Equal(t, big.NewInt(0), appState.State.GetBalance(addr))
	require.Equal(t, big.NewInt(100), appState.State.GetStakeBalance(addr))

	addr = common.Address{0x2, 0x2}
	appState.State.AddStake(addr, big.NewInt(100))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			participated: false,
			prevState:    state.Verified,
			birthday:     2,
			state:        state.Killed,
		})
	require.Equal(t, big.NewInt(0), appState.State.GetBalance(addr))
	require.Equal(t, big.NewInt(100), appState.State.GetStakeBalance(addr))

	addr = common.Address{0x3}
	appState.State.AddStake(addr, big.NewInt(100))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Human,
			birthday:     2,
			state:        state.Killed,
			participated: true,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(100)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(0)))

	addr = common.Address{0x3, 0x3}
	appState.State.AddStake(addr, big.NewInt(100))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			participated: false,
			prevState:    state.Human,
			birthday:     2,
			state:        state.Killed,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(0)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(100)))

	addr = common.Address{0x4}
	appState.State.AddStake(addr, big.NewInt(100))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Suspended,
			birthday:     2,
			state:        state.Killed,
			participated: true,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(100)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(0)))

	addr = common.Address{0x4, 0x4}
	appState.State.AddStake(addr, big.NewInt(100))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Zombie,
			birthday:     2,
			state:        state.Killed,
			participated: true,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(100)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(0)))

	addr = common.Address{0x4, 0x4, 0x4}
	appState.State.AddStake(addr, big.NewInt(100))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			participated: false,
			prevState:    state.Zombie,
			birthday:     2,
			state:        state.Killed,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(0)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(100)))

	addr = common.Address{0x5}
	appState.State.AddStake(addr, big.NewInt(234))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Suspended,
			birthday:     6,
			state:        state.Killed,
			participated: true,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(232)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(2)))

	addr = common.Address{0x5, 0x5}
	appState.State.AddStake(addr, big.NewInt(234))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Zombie,
			birthday:     6,
			state:        state.Killed,
			participated: true,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(232)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(2)))

	addr = common.Address{0x5, 0x5, 0x5}
	appState.State.AddStake(addr, big.NewInt(234))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			participated: false,
			prevState:    state.Suspended,
			birthday:     6,
			state:        state.Killed,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(0)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(234)))

	addr = common.Address{0x6}
	appState.State.AddStake(addr, big.NewInt(234))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Suspended,
			birthday:     7,
			state:        state.Killed,
			participated: true,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(230)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(4)))

	addr = common.Address{0x7}
	appState.State.AddStake(addr, big.NewInt(234))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Suspended,
			birthday:     8,
			state:        state.Killed,
			participated: true,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(227)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(7)))

	addr = common.Address{0x8}
	appState.State.AddStake(addr, big.NewInt(234))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Suspended,
			birthday:     9,
			state:        state.Killed,
			participated: true,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(225)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(9)))

	addr = common.Address{0x9}
	appState.State.AddStake(addr, big.NewInt(234))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Suspended,
			birthday:     10,
			state:        state.Killed,
			participated: true,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(223)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(11)))

	addr = common.Address{0x9, 0x9}
	appState.State.AddStake(addr, big.NewInt(234))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			participated: false,
			prevState:    state.Suspended,
			birthday:     11,
			state:        state.Killed,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(0)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(234)))

	addr = common.Address{0xa}
	appState.State.AddStake(addr, big.NewInt(234))
	applyOnState(config.ConsensusVersions[config.ConsensusV8],
		appState, 15, collector.NewStatsCollector(), addr, cacheValue{
			prevState:    state.Suspended,
			birthday:     12,
			state:        state.Killed,
			participated: true,
		})
	require.Zero(t, appState.State.GetBalance(addr).Cmp(big.NewInt(0)))
	require.Zero(t, appState.State.GetStakeBalance(addr).Cmp(big.NewInt(234)))
}

func Test_calculateNewTotalScore(t *testing.T) {
	var a float32
	var b uint32

	a, b = calculateNewTotalScore([]byte{}, 4, 6, 0, 0)
	require.Equal(t, float32(4)/6, a)
	require.Equal(t, uint32(6), b)

	a, b = calculateNewTotalScore([]byte{}, 4, 6, 140, 145)
	require.Equal(t, float32(130)/137, a)
	require.Equal(t, uint32(137), b)

	a, b = calculateNewTotalScore([]byte{common.EncodeScore(3, 5)}, 5, 6, 150, 163)
	require.Equal(t, float32(128)/141, a)
	require.Equal(t, uint32(141), b)

	a, b = calculateNewTotalScore([]byte{common.EncodeScore(4, 6)}, 3.5, 6, 237.5, 255)
	require.Equal(t, float32(197.5)/216, a)
	require.Equal(t, uint32(216), b)

	a, b = calculateNewTotalScore([]byte{
		common.EncodeScore(4, 6),
		common.EncodeScore(3.5, 6),
		common.EncodeScore(5, 6),
		common.EncodeScore(6, 6),
	}, 4, 5, 237.5, 255)
	require.Equal(t, float32(141.25)/157, a)
	require.Equal(t, uint32(157), b)

	a, b = calculateNewTotalScore([]byte{
		common.EncodeScore(4, 6),
		common.EncodeScore(3.5, 6),
		common.EncodeScore(5, 6),
		common.EncodeScore(6, 6),
		common.EncodeScore(4, 5),
		common.EncodeScore(6, 6),
		common.EncodeScore(5, 6),
		common.EncodeScore(4, 5),
	}, 4, 6, 237.5, 255)
	require.Equal(t, float32(65.25)/78, a)
	require.Equal(t, uint32(78), b)

	a, b = calculateNewTotalScore([]byte{
		common.EncodeScore(4, 6),
		common.EncodeScore(3.5, 6),
		common.EncodeScore(5, 6),
		common.EncodeScore(6, 6),
		common.EncodeScore(4, 5),
		common.EncodeScore(6, 6),
		common.EncodeScore(5, 6),
		common.EncodeScore(4, 5),
		common.EncodeScore(4, 6),
	}, 6, 6, 237.5, 255)
	require.Equal(t, float32(47.5)/58, a)
	require.Equal(t, uint32(58), b)

	a, b = calculateNewTotalScore([]byte{
		common.EncodeScore(4, 6),
		common.EncodeScore(3.5, 6),
		common.EncodeScore(5, 6),
		common.EncodeScore(6, 6),
		common.EncodeScore(4, 5),
		common.EncodeScore(6, 6),
		common.EncodeScore(5, 6),
		common.EncodeScore(4, 5),
		common.EncodeScore(4, 6),
		common.EncodeScore(6, 6),
	}, 5, 5, 237.5, 255)
	require.Equal(t, float32(48.5)/57, a)
	require.Equal(t, uint32(57), b)
}

func Test_determineStakeShareToBurn(t *testing.T) {
	require.Equal(t, 100, determineStakeShareToBurn(state.Candidate, 1, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Candidate, 10, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Candidate, 14, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Newbie, 1, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Newbie, 10, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Newbie, 14, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Verified, 1, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Verified, 10, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Verified, 14, 14))
	require.Equal(t, 0, determineStakeShareToBurn(state.Human, 1, 14))
	require.Equal(t, 0, determineStakeShareToBurn(state.Human, 10, 14))
	require.Equal(t, 0, determineStakeShareToBurn(state.Suspended, 1, 14))
	require.Equal(t, 0, determineStakeShareToBurn(state.Zombie, 1, 14))
	require.Equal(t, 0, determineStakeShareToBurn(state.Suspended, 2, 14))
	require.Equal(t, 0, determineStakeShareToBurn(state.Zombie, 2, 14))
	require.Equal(t, 0, determineStakeShareToBurn(state.Suspended, 3, 14))
	require.Equal(t, 0, determineStakeShareToBurn(state.Zombie, 3, 14))
	require.Equal(t, 0, determineStakeShareToBurn(state.Suspended, 4, 14))
	require.Equal(t, 0, determineStakeShareToBurn(state.Zombie, 4, 14))
	require.Equal(t, 1, determineStakeShareToBurn(state.Suspended, 5, 14))
	require.Equal(t, 1, determineStakeShareToBurn(state.Zombie, 5, 14))
	require.Equal(t, 2, determineStakeShareToBurn(state.Suspended, 6, 14))
	require.Equal(t, 2, determineStakeShareToBurn(state.Zombie, 6, 14))
	require.Equal(t, 3, determineStakeShareToBurn(state.Suspended, 7, 14))
	require.Equal(t, 3, determineStakeShareToBurn(state.Zombie, 7, 14))
	require.Equal(t, 4, determineStakeShareToBurn(state.Suspended, 8, 14))
	require.Equal(t, 4, determineStakeShareToBurn(state.Zombie, 8, 14))
	require.Equal(t, 5, determineStakeShareToBurn(state.Suspended, 9, 14))
	require.Equal(t, 5, determineStakeShareToBurn(state.Zombie, 9, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Suspended, 10, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Zombie, 10, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Suspended, 11, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Zombie, 11, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Suspended, 12, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Zombie, 12, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Suspended, 13, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Zombie, 13, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Suspended, 14, 14))
	require.Equal(t, 100, determineStakeShareToBurn(state.Zombie, 14, 14))
}
