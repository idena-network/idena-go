package ceremony

import (
	"encoding/binary"
	"github.com/deckarep/golang-set"
	"math/rand"
	"sort"
)

func SortFlips(candidatesLen int, flipsLen int, flipsPerAddr int, seed []byte) (flipsPerCandidate [][]int, candidatesPerFlips [][]int) {
	groupsCount := candidatesLen / flipsPerAddr

	if groupsCount == 0 {
		groupsCount = 1
	}

	flipsPerCandidate = make([][]int, candidatesLen)
	candidatesPerFlips = make([][]int, flipsLen)

	mFlipsPerCandidate := make([]mapset.Set, candidatesLen)
	mCandidatesPerFlips := make([]mapset.Set, flipsLen)

	for i := 0; i < len(mFlipsPerCandidate); i++ {
		mFlipsPerCandidate[i] = mapset.NewSet()
	}

	for i := 0; i < len(mCandidatesPerFlips); i++ {
		mCandidatesPerFlips[i] = mapset.NewSet()
	}

	if candidatesLen == 0 || flipsLen == 0 {
		return flipsPerCandidate, candidatesPerFlips
	}

	x := binary.LittleEndian.Uint64(seed)

	for i := 0; i < flipsPerAddr; i++ {

		s := x + x%21*uint64(i+1)

		perm := rand.New(rand.NewSource(int64(s))).Perm(candidatesLen)

		for g := 0; g < groupsCount; g++ {
			for k := 0; k < flipsPerAddr; k++ {
				candidateIdx := (g*flipsPerAddr + k) % len(perm)
				flipIdx := (g + i*groupsCount) % flipsLen
				mFlipsPerCandidate[perm[candidateIdx]].Add(flipIdx)
				mCandidatesPerFlips[flipIdx].Add(perm[candidateIdx])
			}
		}
		for d := 0; d < candidatesLen%flipsPerAddr; d++ {
			candidateIdx := (groupsCount*flipsPerAddr + d) % len(perm)
			flipIdx := (d%groupsCount + i*groupsCount) % flipsLen
			mFlipsPerCandidate[perm[candidateIdx]].Add(flipIdx)
			mCandidatesPerFlips[flipIdx].Add(perm[candidateIdx])
		}
	}

	for i, v := range mFlipsPerCandidate {
		var arr []int
		for _, idx := range v.ToSlice() {
			arr = append(arr, idx.(int))
		}
		sort.SliceStable(arr, func(i, j int) bool {
			return arr[i] < arr[j]
		})
		flipsPerCandidate[i] = arr
	}

	for i, v := range mCandidatesPerFlips {
		var arr []int
		for _, idx := range v.ToSlice() {
			arr = append(arr, idx.(int))
		}
		sort.SliceStable(arr, func(i, j int) bool {
			return arr[i] < arr[j]
		})
		candidatesPerFlips[i] = arr
	}

	return flipsPerCandidate, candidatesPerFlips
}
