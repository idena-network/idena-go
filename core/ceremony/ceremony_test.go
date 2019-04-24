package ceremony

import (
	"github.com/stretchr/testify/require"
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

func getParticipants(myKey []byte, myIndex int, length int) []*participant {
	participants := make([]*participant, 0)

	for i := 0; i < length; i++ {
		if i == myIndex {
			participants = append(participants, &participant{
				PubKey:    myKey,
				Candidate: true,
			})
		} else {
			participants = append(participants, &participant{
				PubKey:    []byte{byte(i)},
				Candidate: true,
			})
		}
	}
	return participants
}
