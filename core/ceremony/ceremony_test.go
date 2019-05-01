package ceremony

import (
	"github.com/stretchr/testify/require"
	"idena-go/core/state"
	"testing"
)

func TestValidationCeremony_getFlipsToSolve(t *testing.T) {
	require := require.New(t)

	myKey := []byte{0x1, 0x2, 0x3}

	flipsCids := [][]byte{{0x1}, {0x2}, {0x3}, {0x4}, {0x5}}

	fliptsPerCandidate := [][]int{{0, 1, 2}, {4, 2, 1}, {1, 2, 3}, {1, 2, 4}, {0, 1, 3}}

	result := getFlipsToSolve(myKey, getParticipants(myKey, 0, 5), fliptsPerCandidate, flipsCids)
	shouldBe := [][]byte{{0x1}, {0x2}, {0x3}}
	require.Equal(shouldBe, result)

	result = getFlipsToSolve(myKey, getParticipants(myKey, 3, 5), fliptsPerCandidate, flipsCids)
	shouldBe = [][]byte{{0x2}, {0x3}, {0x5}}
	require.Equal(shouldBe, result)

	result = getFlipsToSolve(myKey, getParticipants(myKey, 4, 5), fliptsPerCandidate, flipsCids)
	shouldBe = [][]byte{{0x1}, {0x2}, {0x4}}
	require.Equal(shouldBe, result)
}

func TestValidationCeremony_getFlipsToSolve_fewFlips(t *testing.T) {
	require := require.New(t)

	myKey := []byte{0x1, 0x2, 0x3}

	flipsCids := [][]byte{{0x1}, {0x2}, {0x3}, {0x4}, {0x5}}

	fliptsPerCandidate := [][]int{{0, 1, 6}, {4, 2, 8}, {1, 2, 4}, {1, 2, 3}, {6, 7, 8}}

	result := getFlipsToSolve(myKey, getParticipants(myKey, 0, 5), fliptsPerCandidate, flipsCids)
	shouldBe := [][]byte{{0x1}, {0x2}, {0x2}}
	require.Equal(shouldBe, result)

	result = getFlipsToSolve(myKey, getParticipants(myKey, 4, 5), fliptsPerCandidate, flipsCids)
	shouldBe = [][]byte{{0x2}, {0x3}, {0x4}}
	require.Equal(shouldBe, result)
}

func getParticipants(myKey []byte, myIndex int, length int) []*candidate {
	participants := make([]*candidate, 0)

	for i := 0; i < length; i++ {
		if i == myIndex {
			participants = append(participants, &candidate{
				PubKey: myKey,
			})
		} else {
			participants = append(participants, &candidate{
				PubKey: []byte{byte(i)},
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
	}

	cases := []data{
		{
			state.Killed,
			0, 0, 0, 0, true,
			state.Killed,
		},
		{
			state.Invite,
			1, 1, 1, 110, false,
			state.Undefined,
		},
		{
			state.Candidate,
			MinShortScore, MinLongScore, MinTotalScore, 11, false,
			state.Newbie,
		},
		{
			state.Newbie,
			MinShortScore, MinLongScore, MinTotalScore, 11, false,
			state.Verified,
		},
		{
			state.Newbie,
			0.4, 0.8, 1, 11, false,
			state.Killed,
		},
		{
			state.Newbie,
			MinShortScore, MinLongScore, MinTotalScore, 8, false,
			state.Newbie,
		},
		{
			state.Verified,
			MinShortScore, MinLongScore, MinTotalScore, 10, false,
			state.Verified,
		},
		{
			state.Verified,
			0, 0, 0, 0, true,
			state.Suspended,
		},
		{
			state.Verified,
			0, 0, 0, 0, false,
			state.Killed,
		},
		{
			state.Suspended,
			MinShortScore, MinLongScore, MinTotalScore, 10, false,
			state.Verified,
		},
		{
			state.Suspended,
			1, 0.8, 0, 10, true,
			state.Zombie,
		},
		{
			state.Zombie,
			MinShortScore, 0, MinTotalScore, 10, false,
			state.Verified,
		},
		{
			state.Zombie,
			1, 0, 0, 10, true,
			state.Killed,
		},
	}

	require := require.New(t)

	for _, c := range cases {
		require.Equal(c.expected, determineNewIdentityState(c.prev, c.shortScore, c.longScore, c.totalScore, c.totalQualifiedFlips, c.missed))
	}
}
