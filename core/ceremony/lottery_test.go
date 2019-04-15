package ceremony

import (
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSortFlips(t *testing.T) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 500)

	flipsPerCandidate, candidatesPerFlips := SortFlips(7, 3, b)

	flipsPerCandidateResult := [][]int{{1, 3, 4}, {1, 2, 4}, {1, 3, 5}, {0, 2, 4}, {0, 2, 5}, {0, 3, 4}, {0, 2, 5}}
	candidatesPerFlipsResult := [][]int{{5, 3, 6, 4}, {0, 2, 1}, {3, 1, 6, 4}, {0, 2, 5}, {0, 1, 5, 3}, {6, 4, 2}, []int(nil)}

	require.Equal(t, flipsPerCandidateResult, flipsPerCandidate)
	require.Equal(t, candidatesPerFlipsResult, candidatesPerFlips)
}

func TestSortFlips_Big(t *testing.T) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 500)

	const flipPerAddress = 15

	flipsPerCandidate, _ := SortFlips(10000, flipPerAddress, b)

	for i := 0; i < len(flipsPerCandidate); i++ {
		require.Equal(t, flipPerAddress, len(flipsPerCandidate[i]))
	}
}

func TestSortFlips_Small(t *testing.T) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 123123123)

	flipsPerCandidate, candidatesPerFlips := SortFlips(5, 2, b)

	fmt.Println(flipsPerCandidate)
	fmt.Println(candidatesPerFlips)
	flipsPerCandidateResult := [][]int{{0, 3}, {1, 2}, {0, 2}, {1, 3}, {0, 2}}
	candidatesPerFlipsResult := [][]int{{4, 2, 0}, {3, 1}, {1, 4, 2}, {3, 0}, []int(nil)}

	require.Equal(t, flipsPerCandidateResult, flipsPerCandidate)
	require.Equal(t, candidatesPerFlipsResult, candidatesPerFlips)
}
