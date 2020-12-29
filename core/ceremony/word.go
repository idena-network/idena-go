package ceremony

import (
	"encoding/binary"
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/pkg/errors"
	"math/rand"
)

func (vc *ValidationCeremony) GeneratePairsFromVrfHash(hash [32]byte, dictionarySize, pairCount int) (nums []int) {
	rnd := rand.New(rand.NewSource(int64(getWordsRnd(hash))))
	pairs := mapset.NewSet()
	for i := 0; i < pairCount; i++ {
		var num1, num2 int
		num1, num2 = nextPair(rnd, dictionarySize, pairCount, pairs)
		nums = append(nums, num1, num2)
	}
	return nums
}

func (vc *ValidationCeremony) GeneratePairs(seed []byte, dictionarySize, pairCount int) (nums []int, proof []byte) {
	hash, p := vc.secStore.VrfEvaluate(seed)
	nums = vc.GeneratePairsFromVrfHash(hash, dictionarySize, pairCount)
	return nums, p
}

func getWordsRnd(hash [32]byte) uint64 {
	return binary.LittleEndian.Uint64(hash[:])
}

func GetWords(authorRnd uint64, dictionarySize, pairCount, pairIndex int) (word1, word2 int, err error) {
	rnd := rand.New(rand.NewSource(int64(authorRnd)))
	pairs := mapset.NewSet()
	for i := 0; i < pairCount; i++ {
		var val1, val2 int
		val1, val2 = nextPair(rnd, dictionarySize, pairCount, pairs)
		if i == pairIndex {
			return val1, val2, nil
		}
	}
	return 0, 0, errors.New("index is not found")
}

func maxUniquePairs(dictionarySize int) int {
	return (dictionarySize - 1) * dictionarySize / 2
}

func nextPair(rnd *rand.Rand, dictionarySize, pairCount int, pairs mapset.Set) (num1, num2 int) {
	num1, num2 = rnd.Intn(dictionarySize), rnd.Intn(dictionarySize)
	maxPairs := maxUniquePairs(dictionarySize)
	for pairCount <= maxPairs && !checkPair(num1, num2, pairs) {
		num1, num2 = rnd.Intn(dictionarySize), rnd.Intn(dictionarySize)
	}

	pairs.Add(pairToString(num1, num2))
	pairs.Add(pairToString(num2, num1))
	return
}

func checkPair(num1, num2 int, pairs mapset.Set) bool {
	return num1 != num2 && !pairs.Contains(pairToString(num1, num2))
}

func pairToString(num1, num2 int) string {
	return fmt.Sprintf("%d;%d", num1, num2)
}
