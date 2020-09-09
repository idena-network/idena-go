package ceremony

import (
	"encoding/binary"
	"github.com/google/tink/go/subtle/random"
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

func Test_GetFirstAuthorsDistribution(t *testing.T) {
	seed := make([]byte, 8)
	binary.LittleEndian.PutUint64(seed, 100)

	candidates, _, _ := makeCandidatesWithFlips(seed, 120, 100, 3)
	authors := getAuthorsIndexes(candidates)
	authorsPerCandidate, _ := getFirstAuthorsDistribution(authors, candidates, seed, 7)
	for _, item := range authorsPerCandidate {
		require.Equal(t, 7, len(item))
	}

	candidates, _, _ = makeCandidatesWithFlips(seed, 10000, 5000, 3)
	authors = getAuthorsIndexes(candidates)
	authorsPerCandidate, _ = getFirstAuthorsDistribution(authors, candidates, seed, 7)

	for _, item := range authorsPerCandidate {
		require.Equal(t, 7, len(item))
	}
}

func Test_Case1(t *testing.T) {
	seed := make([]byte, 8)
	binary.LittleEndian.PutUint64(seed, 100)

	candidates, flipsPerAuthor, flips := makeCandidatesWithFlips(seed, 150, 100, 3)

	authorsPerCandidate, _ := GetAuthorsDistribution(candidates, seed, 7)
	shortFlipsPerCandidate, longFlipsPerCandidate := GetFlipsDistribution(len(candidates), authorsPerCandidate, flipsPerAuthor, flips, seed, 7)

	for _, item := range shortFlipsPerCandidate {
		require.Equal(t, 7, len(item))
	}

	for _, item := range longFlipsPerCandidate {
		require.True(t, len(item) >= 11)
	}
}

func Test_Case2(t *testing.T) {
	seed := make([]byte, 8)
	binary.LittleEndian.PutUint64(seed, 500)

	candidates, flipsPerAuthor, flips := makeCandidatesWithFlips(seed, 300, 100, 3)

	authorsPerCandidate, _ := GetAuthorsDistribution(candidates, seed, 7)
	shortFlipsPerCandidate, longFlipsPerCandidate := GetFlipsDistribution(len(candidates), authorsPerCandidate, flipsPerAuthor, flips, seed, 7)

	for _, item := range shortFlipsPerCandidate {
		require.Equal(t, 7, len(item))
	}

	for _, item := range longFlipsPerCandidate {
		require.True(t, len(item) >= 14)
	}
}

func Test_Case3(t *testing.T) {
	seed := make([]byte, 8)
	binary.LittleEndian.PutUint64(seed, 500)

	candidates, flipsPerAuthor, flips := makeCandidatesWithFlips(seed, 5, 0, 3)

	authorsPerCandidate, _ := GetAuthorsDistribution(candidates, seed, 7)
	shortFlipsPerCandidate, longFlipsPerCandidate := GetFlipsDistribution(len(candidates), authorsPerCandidate, flipsPerAuthor, flips, seed, 7)

	require.Equal(t, 5, len(shortFlipsPerCandidate))
	require.Equal(t, 5, len(longFlipsPerCandidate))

	candidates, flipsPerAuthor, flips = makeCandidatesWithFlips(seed, 0, 0, 3)

	authorsPerCandidate, _ = GetAuthorsDistribution(candidates, seed, 7)
	shortFlipsPerCandidate, longFlipsPerCandidate = GetFlipsDistribution(len(candidates), authorsPerCandidate, flipsPerAuthor, flips, seed, 7)
	require.Equal(t, 0, len(shortFlipsPerCandidate))
	require.Equal(t, 0, len(longFlipsPerCandidate))
}

func Test_Case4(t *testing.T) {
	seed := make([]byte, 8)
	binary.LittleEndian.PutUint64(seed, 500)

	candidates, flipsPerAuthor, flips := makeCandidatesWithFlips(seed, 12, 1, 1)

	authorsPerCandidate, _ := GetAuthorsDistribution(candidates, seed, 7)
	shortFlipsPerCandidate, longFlipsPerCandidate := GetFlipsDistribution(len(candidates), authorsPerCandidate, flipsPerAuthor, flips, seed, 7)

	for _, item := range shortFlipsPerCandidate {
		require.Equal(t, 1, len(item))
	}

	for _, item := range longFlipsPerCandidate {
		require.Equal(t, 1, len(item))
	}
}

func Test_Case5(t *testing.T) {
	seed := make([]byte, 8)
	binary.LittleEndian.PutUint64(seed, 500)

	candidates, flipsPerAuthor, flips := makeCandidatesWithFlips(seed, 10, 3, 3)

	authorsPerCandidate, _ := GetAuthorsDistribution(candidates, seed, 7)
	shortFlipsPerCandidate, longFlipsPerCandidate := GetFlipsDistribution(len(candidates), authorsPerCandidate, flipsPerAuthor, flips, seed, 7)

	for _, item := range shortFlipsPerCandidate {
		require.True(t, len(item) >= 6)
	}

	for _, item := range longFlipsPerCandidate {
		require.True(t, len(item) >= 6)
	}
}

func Test_FillAuthorsQueue(t *testing.T) {
	seed := make([]byte, 8)
	binary.LittleEndian.PutUint64(seed, 100)
	candidatesCount := 20
	authorsCount := 0
	candidates := makeCandidates(candidatesCount)
	var authorIndexes []int
	for i := 0; i < candidatesCount; i += 2 {
		candidates[i].IsAuthor = true
		authorIndexes = append(authorIndexes, i)
		authorsCount++
	}
	queue := fillAuthorsQueue(seed, authorIndexes, candidates, 7)

	require.Equal(t, candidatesCount*7, queue.Len())
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

func makeCandidatesWithFlips(seed []byte, candidatesCount int, authorsCount int, flipPerAuthor int) (candidates []*candidate, flipsPerAuthor map[int][][]byte, flips [][]byte) {
	res := make([]*candidate, 0)
	randSeed := binary.LittleEndian.Uint64(seed)
	//code := random.GetRandomBytes(12)
	r := rand.New(rand.NewSource(int64(randSeed)*12 + 3))
	authors := r.Perm(candidatesCount)[:authorsCount]
	authorsMap := make(map[int]bool)
	for _, a := range authors {
		authorsMap[a] = true
	}

	flipsPerAuthor = make(map[int][][]byte)
	flips = make([][]byte, 0)

	for i := 0; i < candidatesCount; i++ {
		isAuthor := authorsMap[i]
		if isAuthor {
			for j := 0; j < flipPerAuthor; j++ {
				flip := random.GetRandomBytes(5)
				flipsPerAuthor[i] = append(flipsPerAuthor[i], flip)
				flips = append(flips, flip)
			}
		}
		res = append(res, &candidate{
			Code:       random.GetRandomBytes(12),
			Generation: 0,
			IsAuthor:   isAuthor,
		})
	}
	return res, flipsPerAuthor, flips
}
