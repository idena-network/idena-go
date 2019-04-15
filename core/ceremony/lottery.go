package ceremony

import (
	"encoding/binary"
	"math/rand"
)

func SortFlips(candidatesLen int, flipsPerAddr int, seed []byte) (flipsPerCandidate [][]int, candidatesPerFlips [][]int) {
	groupsCount := candidatesLen / flipsPerAddr

	flipsPerCandidate = make([][]int, candidatesLen)
	candidatesPerFlips = make([][]int, candidatesLen)

	x := binary.LittleEndian.Uint64(seed)

	for i := 0; i < flipsPerAddr; i++ {

		s := x + x%21*uint64(i+1)

		perm := rand.New(rand.NewSource(int64(s))).Perm(candidatesLen)

		for g := 0; g < groupsCount; g++ {
			for k := 0; k < flipsPerAddr; k++ {
				flipIdx := g + i*groupsCount
				flipsPerCandidate[perm[g*flipsPerAddr+k]] = append(flipsPerCandidate[perm[g*flipsPerAddr+k]], flipIdx)
				candidatesPerFlips[flipIdx] = append(candidatesPerFlips[flipIdx], perm[g*flipsPerAddr+k])
			}
		}
		for d := 0; d < candidatesLen%flipsPerAddr; d++ {
			candidateIdx := groupsCount*flipsPerAddr + d
			flipIdx := d%groupsCount + i*groupsCount
			flipsPerCandidate[perm[candidateIdx]] = append(flipsPerCandidate[perm[candidateIdx]], flipIdx)
			candidatesPerFlips[flipIdx] = append(candidatesPerFlips[flipIdx], perm[candidateIdx])
		}
	}

	return flipsPerCandidate, candidatesPerFlips
}
