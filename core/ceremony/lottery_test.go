package ceremony

import (
	"encoding/binary"
	"github.com/google/tink/go/subtle/random"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSortFlips(t *testing.T) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 500)

	flipsPerAuthor, flips := makeFlips(7, 3)

	flipsPerCandidateShort := SortFlips(flipsPerAuthor, 7, flips, 3, b, false, nil)

	chosenFlips := make(map[int]bool)
	for _, a := range flipsPerCandidateShort {
		for _, f := range a {
			chosenFlips[f] = true
		}
	}

	flipsPerCandidateLong := SortFlips(flipsPerAuthor, 7, flips, 10, b, true, chosenFlips)

	flipsPerCandidateShortResult := [][]int{{7, 11, 13}, {8, 9, 18}, {4, 12, 16}, {1, 19, 20}, {0, 2, 10}, {3, 5}, {6, 14, 17}}
	flipsPerCandidateLongResult := [][]int{{3, 4, 5, 8, 12, 14, 16, 18, 19, 20}, {1, 8, 9, 12, 14, 16, 17, 18, 19, 20}, {0, 2, 3, 5, 9, 10, 11, 13, 14, 17}, {0, 1, 3, 4, 8, 12, 16, 18, 19, 20}, {0, 2, 5, 6, 7, 9, 10, 11, 17, 18}, {2, 3, 5, 6, 7, 8, 10, 11, 12, 13}, {0, 2, 6, 7, 9, 10, 11, 13, 14, 17}}

	require.Equal(t, flipsPerCandidateShortResult, flipsPerCandidateShort)
	require.Equal(t, flipsPerCandidateLongResult, flipsPerCandidateLong)
}

func makeFlips(authors int, flipNum int) (flipsPerAuthor map[int][][]byte, flips [][]byte) {
	flipsPerAuthor = make(map[int][][]byte)
	flips = make([][]byte, 0)
	for i := 0; i < authors; i++ {
		for j := 0; j < flipNum; j++ {
			flip := random.GetRandomBytes(5)
			flipsPerAuthor[i] = append(flipsPerAuthor[i], flip)
			flips = append(flips, flip)
		}
	}
	return flipsPerAuthor, flips
}
