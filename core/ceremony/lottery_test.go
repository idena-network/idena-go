package ceremony

import (
	"encoding/binary"
	"github.com/google/tink/go/subtle/random"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSortFlips(t *testing.T) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 1000)

	flipsPerAuthor, flips := makeFlips(7, 3)

	flipsPerCandidateShort := SortFlips(flipsPerAuthor, makeCandidates(7), flips, 3, b, false, nil)

	chosenFlips := make(map[int]bool)
	for _, a := range flipsPerCandidateShort {
		for _, f := range a {
			chosenFlips[f] = true
		}
	}

	flipsPerCandidateLong := SortFlips(flipsPerAuthor, makeCandidates(7), flips, 10, b, true, chosenFlips)

	flipsPerCandidateShortResult := [][]int{{9, 14, 15}, {11, 17, 19}, {4, 13, 16}, {1, 7, 18}, {0, 2, 8}, {6, 10, 20}, {3, 5, 12}}
	flipsPerCandidateLongResult := [][]int{{4, 6, 9, 10, 12, 13, 15, 16, 18, 20}, {0, 1, 6, 9, 10, 13, 15, 16, 18, 20}, {2, 4, 5, 10, 11, 12, 13, 16, 18, 20}, {0, 2, 3, 7, 8, 12, 14, 17, 18, 19}, {0, 1, 2, 3, 4, 7, 8, 11, 17, 19}, {0, 1, 3, 7, 8, 9, 11, 14, 19, 20}, {3, 4, 5, 6, 10, 11, 12, 13, 15, 16}}

	require.Equal(t, flipsPerCandidateShortResult, flipsPerCandidateShort)
	require.Equal(t, flipsPerCandidateLongResult, flipsPerCandidateLong)
}

func TestHasRelation(t *testing.T) {
	a := &candidate{
		Generation: 4,
		Code:       []byte{0x5, 0x6, 0x7, 0x8, 0x4, 0xf, 0x3},
	}
	b := &candidate{
		Generation: 8,
		Code:       []byte{0x4, 0xf, 0x3, 0xc, 0xf, 0x3, 0x4},
	}

	require.True(t, hasRelation(a, b, 3))

	a = &candidate{
		Generation: 2,
		Code:       []byte{0x5, 0x6, 0x7, 0x8, 0x4, 0xf, 0x3},
	}
	b = &candidate{
		Generation: 8,
		Code:       []byte{0x3, 0xf, 0x3, 0xc, 0xf, 0x3, 0x4},
	}

	require.True(t, hasRelation(a, b, 1))

	a = &candidate{
		Generation: 0,
		Code:       []byte{0x5, 0x6, 0x7, 0x8, 0x4, 0xf, 0x3},
	}
	b = &candidate{
		Generation: 8,
		Code:       []byte{0x3, 0xf, 0x3, 0xc, 0xf, 0x3, 0x4},
	}

	require.False(t, hasRelation(a, b, 5))

	a = &candidate{
		Generation: 4,
		Code:       []byte{0x5, 0x6, 0x7, 0x8, 0x4, 0xf, 0x4},
	}
	b = &candidate{
		Generation: 8,
		Code:       []byte{0x4, 0xf, 0x3, 0xc, 0xf, 0x3, 0x4},
	}

	require.False(t, hasRelation(a, b, 5))
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

func makeCandidates(authors int) []*candidate {
	res := make([]*candidate, 0)
	b := random.GetRandomBytes(12)
	for i := 0; i < authors; i++ {
		res = append(res, &candidate{
			Code:       b,
			Generation: 0,
		})
	}
	return res
}
