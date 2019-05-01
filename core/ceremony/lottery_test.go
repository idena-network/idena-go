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

	flipsPerCandidate, candidatesPerFlips := SortFlips(7, 6, 3, b)

	flipsPerCandidateResult := [][]int{{1, 3, 4}, {1, 2, 4}, {1, 3, 5}, {0, 2, 4}, {0, 2, 5}, {0, 3, 4}, {0, 2, 5}}
	candidatesPerFlipsResult := [][]int{{3, 4, 5, 6}, {0, 1, 2}, {1, 3, 4, 6}, {0, 2, 5}, {0, 1, 3, 5}, {2, 4, 6}}

	require.Equal(t, flipsPerCandidateResult, flipsPerCandidate)
	require.Equal(t, candidatesPerFlipsResult, candidatesPerFlips)
}

func TestSortFlips_Big(t *testing.T) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 500)

	const flipPerAddress = 15

	flipsPerCandidate, _ := SortFlips(10000, 20, flipPerAddress, b)

	for i := 0; i < len(flipsPerCandidate); i++ {
		require.True(t, len(flipsPerCandidate[i]) <= flipPerAddress)
	}
}

func TestSortFlips_Small(t *testing.T) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 123123123)

	flipsPerCandidate, candidatesPerFlips := SortFlips(5, 4, 2, b)

	fmt.Println(flipsPerCandidate)
	fmt.Println(candidatesPerFlips)
	flipsPerCandidateResult := [][]int{{0, 3}, {1, 2}, {0, 2}, {1, 3}, {0, 2}}
	candidatesPerFlipsResult := [][]int{{0, 2, 4}, {1, 3}, {1, 2, 4}, {0, 3}}

	require.Equal(t, flipsPerCandidateResult, flipsPerCandidate)
	require.Equal(t, candidatesPerFlipsResult, candidatesPerFlips)
}

func TestSortFlips_Small2(t *testing.T) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 123123123)

	flipsPerCandidate, candidatesPerFlips := SortFlips(1, 6, 10, b)

	fmt.Println(flipsPerCandidate)
	fmt.Println(candidatesPerFlips)
	flipsPerCandidateResult := [][]int{{0, 1, 2, 3, 4, 5}}
	candidatesPerFlipsResult := [][]int{{0}, {0}, {0}, {0}, {0}, {0}}

	require.Equal(t, flipsPerCandidateResult, flipsPerCandidate)
	require.Equal(t, candidatesPerFlipsResult, candidatesPerFlips)
}
