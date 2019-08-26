package ceremony

import (
	"bytes"
	"encoding/binary"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/rlp"
	"math/rand"
	"sort"
)

const (
	GeneticRelationLength = 3
	MaxLongFlipSolvers    = 10
)

func SortFlips(flipsPerAuthor map[int][][]byte, candidates []*candidate, flips [][]byte, flipsPerAddr int, seed []byte, longSession bool, flipsToUse map[int]bool) (flipsPerCandidate [][]int) {

	candidatesLen := len(candidates)

	flipsPerCandidate = make([][]int, candidatesLen)

	if candidatesLen == 0 || len(flips) == 0 {
		return flipsPerCandidate
	}

	authorsPerFlips := make(map[common.Hash]int)

	for k, v := range flipsPerAuthor {
		for _, f := range v {
			authorsPerFlips[common.Hash(rlp.Hash(f))] = k
		}
	}

	isAuthor := func(flipIndex int, candidateIndex int) bool {
		flip := flips[flipIndex]
		for _, f := range flipsPerAuthor[candidateIndex] {
			if bytes.Compare(flip, f) == 0 {
				return true
			}
		}
		return false
	}

	randSeed := binary.LittleEndian.Uint64(seed)

	maxFlipUses := candidatesLen * flipsPerAddr / len(flips)
	if candidatesLen*flipsPerAddr%len(flips) > 0 {
		maxFlipUses++
	}
	if longSession && maxFlipUses > MaxLongFlipSolvers {
		maxFlipUses = MaxLongFlipSolvers
	}

	used := make([]int, len(flips))

	//shuffle flips
	random := rand.New(rand.NewSource(int64(randSeed)))
	flipsPermutation := random.Perm(len(flips))
	candidatesPermutation := random.Perm(candidatesLen)

	result := make(map[int]mapset.Set)

	findSuitableFlipIndex := func(currentIdx int, currentCandidateIndex int,
		hasRelationFn func(first *candidate, second *candidate, geneticOverlapLength int) bool) int {

		startFlipIndex := currentIdx
		for {
			newFlipIndex := flipsPermutation[startFlipIndex]

			// we can pick this flip if
			// 1. this is short session or this is long session and this flip was chosen during short session
			// 2. this candidate does not have this flip yet
			// 3. if number of uses is less than maximum
			// 4. this candidate is not the author of this flip
			// 5. this candidate is in relation with flip author
			if (!longSession || longSession && flipsToUse[newFlipIndex]) &&
				!result[currentCandidateIndex].Contains(newFlipIndex) &&
				used[newFlipIndex] < maxFlipUses &&
				!isAuthor(newFlipIndex, currentCandidateIndex) &&
				!hasRelationFn(candidates[currentCandidateIndex], candidates[authorsPerFlips[rlp.Hash(flips[newFlipIndex])]], GeneticRelationLength) {
				break
			}

			startFlipIndex++

			// we should check either we passed a whole circle and didnt find suitable flip
			// or we just reached the end and need to continue from 0 index.
			// the extra situation, when we started from zero index and reached the end
			// in this situation we need to stop, because we also passed whole circle
			if startFlipIndex == len(flips) {
				startFlipIndex = 0
				if currentIdx == 0 {
					break
				}
			}
			if startFlipIndex == currentIdx {
				break
			}
		}

		return startFlipIndex
	}

	currentFlipIndex := 0
	for i := 0; i < candidatesLen; i++ {
		currentCandidateIndex := candidatesPermutation[i]
		result[currentCandidateIndex] = mapset.NewSet()

		for j := 0; j < flipsPerAddr; j++ {
			if currentFlipIndex == len(flips) {
				currentFlipIndex = 0
			}

			// include genetic relation
			idx := findSuitableFlipIndex(currentFlipIndex, currentCandidateIndex, hasRelation)

			// if we didnt find suitable index, we try to search again, without genetic code
			if idx == currentFlipIndex {
				currentFlipIndex++
				if currentFlipIndex == len(flips) {
					currentFlipIndex = 0
				}
				idx = findSuitableFlipIndex(currentFlipIndex, currentCandidateIndex, func(first *candidate, second *candidate, geneticOverlapLength int) bool {
					return false
				})
			}

			foundFlipIndex := flipsPermutation[idx]
			used[foundFlipIndex]++
			result[currentCandidateIndex].Add(foundFlipIndex)
			currentFlipIndex++
		}
	}

	for i, v := range result {
		var arr []int
		for _, idx := range v.ToSlice() {
			arr = append(arr, idx.(int))
		}
		sort.SliceStable(arr, func(i, j int) bool {
			return arr[i] < arr[j]
		})
		flipsPerCandidate[i] = arr
	}

	return flipsPerCandidate
}

func hasRelation(first *candidate, second *candidate, geneticOverlapLength int) bool {

	res := func(a *candidate, b *candidate) bool {
		codeLength := len(a.Code)
		diff := b.Generation - a.Generation
		if diff > uint32(codeLength-geneticOverlapLength) {
			return false
		}

		return bytes.Compare(a.Code[diff:][:geneticOverlapLength], b.Code[:geneticOverlapLength]) == 0
	}

	if first.Generation <= second.Generation {
		return res(first, second)
	} else {
		return res(second, first)
	}
}
