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
	"github.com/idena-network/idena-go/rlp"
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
			MinShortScore, MinLongScore, MinTotalScore, 11, false,
			state.Newbie, false, false,
		},
		{
			state.Candidate,
			MinShortScore, MinLongScore, MinTotalScore, 11, true,
			state.Killed, false, false,
		},
		{
			state.Newbie,
			MinShortScore, MinLongScore, MinTotalScore, 11, false,
			state.Newbie, false, false,
		},
		{
			state.Newbie,
			MinShortScore, MinLongScore, MinTotalScore, 13, false,
			state.Verified, false, false,
		},
		{
			state.Newbie,
			MinShortScore, MinLongScore, MinTotalScore, 10, false,
			state.Newbie, false, false,
		},
		{
			state.Newbie,
			MinShortScore, MinLongScore, MinTotalScore, 11, true,
			state.Killed, false, false,
		},
		{
			state.Newbie,
			0.4, 0.8, 1, 11, false,
			state.Killed, false, false,
		},
		{
			state.Newbie,
			MinShortScore, MinLongScore, MinTotalScore, 8, false,
			state.Newbie, false, false,
		},
		{
			state.Verified,
			MinShortScore, MinLongScore, MinTotalScore, 10, false,
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
			MinShortScore, MinLongScore, MinTotalScore, 10, false,
			state.Verified, false, false,
		},
		{
			state.Suspended,
			1, 0.8, 0, 10, true,
			state.Zombie, false, false,
		},
		{
			state.Zombie,
			MinShortScore, 0, MinTotalScore, 10, false,
			state.Verified, false, false,
		},
		{
			state.Zombie,
			1, 0, 0, 10, true,
			state.Killed, false, false,
		},
		{
			state.Candidate,
			MinShortScore, 0, 0, 5, false,
			state.Candidate, true, false,
		},
		{
			state.Candidate,
			MinShortScore - 0.1, 0, 0, 5, false,
			state.Killed, false, true,
		},
		{
			state.Newbie,
			MinShortScore, 0, 0.1, 5, false,
			state.Newbie, true, false,
		},
		{
			state.Newbie,
			MinShortScore, 0, 0.1, 5, false,
			state.Newbie, false, true,
		},
		{
			state.Newbie,
			MinShortScore, 0, 0.1, 13, false,
			state.Killed, false, true,
		},
		{
			state.Newbie,
			MinShortScore - 0.1, 0, 0.1, 9, false,
			state.Killed, false, true,
		},
		{
			state.Verified,
			MinShortScore - 0.1, 0, 0.1, 10, false,
			state.Verified, true, false,
		},
		{
			state.Verified,
			MinShortScore - 0.1, 0, 1.1, 10, false,
			state.Killed, false, true,
		},
		{
			state.Suspended,
			MinShortScore - 0.1, 0, 0.1, 10, false,
			state.Suspended, true, false,
		},
		{
			state.Suspended,
			MinShortScore - 0.1, 0, 1.1, 10, false,
			state.Killed, false, true,
		},
		{
			state.Zombie,
			MinShortScore - 0.1, 0, 0.1, 10, false,
			state.Zombie, true, false,
		},
		{
			state.Zombie,
			MinShortScore, 0, 0.1, 10, false,
			state.Killed, false, true,
		},
		{
			state.Suspended,
			MinShortScore, MinLongScore, MinHumanTotalScore, 24, false,
			state.Human, false, false,
		},
		{
			state.Suspended,
			MinShortScore, MinLongScore, MinHumanTotalScore, 23, false,
			state.Verified, false, false,
		},
		{
			state.Zombie,
			MinShortScore, MinLongScore, MinHumanTotalScore, 24, false,
			state.Human, false, false,
		},
		{
			state.Zombie,
			MinShortScore, MinLongScore, MinHumanTotalScore, 23, false,
			state.Verified, false, false,
		},
		{
			state.Human,
			0.1, MinLongScore, MinHumanTotalScore, 24, false,
			state.Suspended, false, false,
		},
		{
			state.Human,
			MinShortScore, 0.1, MinHumanTotalScore, 24, false,
			state.Killed, false, false,
		},
		{
			state.Human,
			0.1, 0.1, MinHumanTotalScore, 24, false,
			state.Suspended, false, true,
		},
		{
			state.Human,
			MinShortScore, MinLongScore, MinHumanTotalScore, 24, false,
			state.Human, false, true,
		},
		{
			state.Human,
			MinShortScore, MinLongScore, MinHumanTotalScore, 24, true,
			state.Suspended, false, false,
		},
		{
			state.Human,
			MinShortScore, MinLongScore, MinHumanTotalScore, 24, false,
			state.Human, true, true,
		},
		{
			state.Verified,
			MinShortScore, MinLongScore, MinHumanTotalScore, 24, false,
			state.Human, false, false,
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

	vc.candidates = candidates
	vc.flips = flips
	vc.flipsPerAuthor = flipsPerAuthor
	vc.appState = app

	approvedCandidates := mapset.NewSet()
	approvedCandidates.Add(approvedAddr)

	// when
	result := vc.getNotApprovedFlips(approvedCandidates)

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
	vc := ValidationCeremony{}

	auth1 := common.Address{1}
	auth2 := common.Address{2}
	auth3 := common.Address{3}
	auth4 := common.Address{4}
	auth5 := common.Address{5}
	auth6 := common.Address{6}

	vc.flips = [][]byte{{0x0}, {0x1}, {0x2}, {0x3}, {0x4}, {0x5}, {0x6}, {0x7}, {0x8}, {0x9}, {0xa}, {0xb}, {0xc}}
	vc.flipAuthorMap = map[common.Hash]common.Address{
		rlp.Hash([]byte{0x0}): auth1,
		rlp.Hash([]byte{0x1}): auth1,
		rlp.Hash([]byte{0x2}): auth1,

		rlp.Hash([]byte{0x3}): auth2,
		rlp.Hash([]byte{0x4}): auth2,

		rlp.Hash([]byte{0x5}): auth3,
		rlp.Hash([]byte{0x6}): auth3,

		rlp.Hash([]byte{0x7}): auth4,
		rlp.Hash([]byte{0x8}): auth4,

		rlp.Hash([]byte{0x9}): auth5,

		rlp.Hash([]byte{0xa}): auth6,
		rlp.Hash([]byte{0xb}): auth6,
		rlp.Hash([]byte{0xc}): auth6,
	}

	qualification := []FlipQualification{
		{status: Qualified},
		{status: WeaklyQualified},
		{status: NotQualified},

		{status: Qualified, answer: types.Inappropriate},
		{status: Qualified},

		{status: WeaklyQualified, wrongWords: true},
		{status: Qualified},

		{status: NotQualified},
		{status: NotQualified},

		{status: QualifiedByNone},

		{status: Qualified},
		{status: WeaklyQualified},
		{status: Qualified},
	}

	bad, good, authorResults := vc.analyzeAuthors(qualification)

	require.Contains(t, bad, auth2)
	require.Contains(t, bad, auth3)
	require.Contains(t, bad, auth4)
	require.Contains(t, bad, auth5)
	require.NotContains(t, bad, auth1)
	require.NotContains(t, bad, auth6)

	require.Contains(t, good, auth1)
	require.Equal(t, 1, len(good[auth1].WeakFlipCids))
	require.Equal(t, 1, len(good[auth1].StrongFlipCids))

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
}

func Test_incSuccessfulInvites(t *testing.T) {
	epoch := uint16(5)
	god := common.Address{0x1}
	auth1 := common.Address{0x2}
	badAuth := common.Address{0x3}

	authors := &types.ValidationAuthors{
		BadAuthors: map[common.Address]types.BadAuthorReason{badAuth: types.WrongWordsBadAuthor},
		GoodAuthors: map[common.Address]*types.ValidationResult{
			auth1: {StrongFlipCids: [][]byte{{0x1}}, WeakFlipCids: [][]byte{{0x1}}},
		},
	}

	incSuccessfulInvites(authors, god, state.Identity{
		State: state.Verified,
		Inviter: &state.TxAddr{
			Address: god,
		},
	}, 0, state.Newbie, epoch)

	incSuccessfulInvites(authors, god, state.Identity{
		State: state.Candidate,
		Inviter: &state.TxAddr{
			Address: auth1,
		},
	}, 5, state.Newbie, epoch)

	incSuccessfulInvites(authors, god, state.Identity{
		State: state.Candidate,
		Inviter: &state.TxAddr{
			Address: badAuth,
		},
	}, 5, state.Newbie, epoch)

	incSuccessfulInvites(authors, god, state.Identity{
		State: state.Candidate,
		Inviter: &state.TxAddr{
			Address: god,
		},
	}, 5, state.Newbie, epoch)

	// 4th validation (Newbie->Newbie)
	incSuccessfulInvites(authors, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.TxAddr{
			Address: auth1,
		},
	}, 2, state.Newbie, epoch)

	// 4th validation (Newbie->Verified)
	incSuccessfulInvites(authors, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.TxAddr{
			Address: auth1,
		},
	}, 2, state.Verified, epoch)

	// 3rd validation (Newbie->Newbie)
	incSuccessfulInvites(authors, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.TxAddr{
			Address: auth1,
		},
	}, 3, state.Newbie, epoch)

	// 2nd validation (Newbie->Newbie)
	incSuccessfulInvites(authors, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.TxAddr{
			Address: auth1,
		},
	}, 4, state.Newbie, epoch)

	// 3rd validation (Newbie->Verified)
	incSuccessfulInvites(authors, god, state.Identity{
		State: state.Newbie,
		Inviter: &state.TxAddr{
			Address: auth1,
		},
	}, 3, state.Verified, epoch)

	require.Equal(t, len(authors.GoodAuthors[auth1].SuccessfulInvites), 4)
	var ages []uint16
	for _, si := range authors.GoodAuthors[auth1].SuccessfulInvites {
		ages = append(ages, si.Age)
	}
	require.Equal(t, []uint16{1, 3, 2, 3}, ages)

	require.Equal(t, len(authors.GoodAuthors[god].SuccessfulInvites), 1)
	require.Equal(t, uint16(1), authors.GoodAuthors[god].SuccessfulInvites[0].Age)
	require.True(t, authors.GoodAuthors[god].PayInvitationReward)
	require.False(t, authors.GoodAuthors[god].Missed)

	require.NotContains(t, authors.GoodAuthors, badAuth)
}

func Test_determineIdentityBirthday(t *testing.T) {
	identity := state.Identity{}
	identity.Birthday = 1
	identity.State = state.Newbie
	require.Equal(t, uint16(1), determineIdentityBirthday(2, identity, state.Newbie))
}

func Test_applyOnState(t *testing.T) {
	db := dbm.NewMemDB()
	appstate := appstate.NewAppState(db, eventbus.New())

	addr1 := common.Address{0x1}

	appstate.State.SetState(addr1, state.Newbie)
	appstate.State.AddStake(addr1, big.NewInt(100))
	appstate.State.AddBalance(addr1, big.NewInt(10))

	identities := applyOnState(appstate, collector.NewStatsCollector(), addr1, cacheValue{
		prevState:                state.Newbie,
		birthday:                 3,
		shortFlipPoint:           1,
		shortQualifiedFlipsCount: 2,
		state:                    state.Verified,
	})
	identity := appstate.State.GetIdentity(addr1)
	require.Equal(t, 1, identities)
	require.Equal(t, state.Verified, identity.State)
	require.Equal(t, uint16(3), identity.Birthday)
	require.Equal(t, float32(1), identity.GetShortFlipPoints())
	require.Equal(t, uint32(2), identity.QualifiedFlips)
	require.True(t, appstate.State.GetBalance(addr1).Cmp(big.NewInt(85)) == 0)
	require.True(t, appstate.State.GetStakeBalance(addr1).Cmp(big.NewInt(25)) == 0)
}
