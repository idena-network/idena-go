package ceremony

import (
	"bytes"
	"encoding/binary"
	"github.com/deckarep/golang-set"
	"math/rand"
	"sort"
)

func SortFlips(flipsPerAuthor map[int][][]byte, candidatesLen int, flips [][]byte, flipsPerAddr int, seed []byte, longSession bool, flipsToUse map[int]bool) (flipsPerCandidate [][]int) {

	isAuthor := func(flipIndex int, candidateIndex int) bool {
		flip := flips[flipIndex]
		for _, f := range flipsPerAuthor[candidateIndex] {
			if bytes.Compare(flip, f) == 0 {
				return true
			}
		}
		return false
	}

	flipsPerCandidate = make([][]int, candidatesLen)

	if candidatesLen == 0 || len(flips) == 0 {
		return flipsPerCandidate
	}

	randSeed := binary.LittleEndian.Uint64(seed)

	maxFlipUses := candidatesLen * flipsPerAddr / len(flips)
	if candidatesLen*flipsPerAddr%len(flips) > 0 {
		maxFlipUses++
	}

	used := make([]int, len(flips))

	//shuffle flips
	random := rand.New(rand.NewSource(int64(randSeed)))
	flipsPermutation := random.Perm(len(flips))
	candidatesPermutation := random.Perm(candidatesLen)

	result := make(map[int]mapset.Set)

	currentFlipIndex := 0
	for i := 0; i < candidatesLen; i++ {
		currentCandidateIndex := candidatesPermutation[i]
		result[currentCandidateIndex] = mapset.NewSet()

		for j := 0; j < flipsPerAddr; j++ {
			if currentFlipIndex == len(flips) {
				currentFlipIndex = 0
			}

			startFlipIndex := currentFlipIndex

			for {
				newFlipIndex := flipsPermutation[startFlipIndex]

				// we can pick this flip if
				// 1. this is short session or this is long session and this flip was chosen during short session
				// 2. this candidate does not have this flip yet
				// 3. if number of uses is less than maximum
				// 4. this candidate is not the author of this flip
				if (!longSession || longSession && flipsToUse[newFlipIndex]) &&
					!result[currentCandidateIndex].Contains(newFlipIndex) &&
					used[newFlipIndex] < maxFlipUses &&
					!isAuthor(newFlipIndex, currentCandidateIndex) {
					break
				}

				startFlipIndex++

				// we should check either we passed a whole circle and didnt find suitable flip
				// or we just reached the end and need to continue from 0 index.
				// the extra situation, when we started from zero index and reached the end
				// in this situation we need to stop, because we also passed whole circle
				if startFlipIndex == len(flips) {
					startFlipIndex = 0
					if currentFlipIndex == 0 {
						break
					}
				}
				if startFlipIndex == currentFlipIndex {
					break
				}
			}

			foundFlipIndex := flipsPermutation[startFlipIndex]
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
