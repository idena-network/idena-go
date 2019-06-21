package flip

import (
	"fmt"
	"github.com/deckarep/golang-set"
	"idena-go/crypto/vrf"
	"math/big"
)

const (
	a = 1103515245
	c = 12345
	m = 1 << 16
)

func GeneratePairs(seed []byte, k vrf.PrivateKey, n, pairCount int) (nums []int, proof []byte) {
	hash, proof := k.Evaluate(seed)
	rnd := generatePseudoRndSeed(hash, n)
	pairs := mapset.NewSet()
	for i := 0; i < pairCount; i++ {
		var num1, num2 int
		num1, num2, rnd = nextPair(rnd, n, pairs)
		nums = append(nums, num1, num2)
	}
	return nums, proof
}

func CheckPair(seed []byte, n, pairCount, num1, num2 int, proof []byte, pk vrf.PublicKey) bool {
	hash, err := pk.ProofToHash(seed, proof)
	if err != nil {
		return false
	}
	rnd := generatePseudoRndSeed(hash, n)
	pairs := mapset.NewSet()
	for i := 0; i < pairCount; i++ {
		var val1, val2 int
		val1, val2, rnd = nextPair(rnd, n, pairs)
		if val1 == num1 && val2 == num2 {
			return true
		}
	}
	return false
}

func generatePseudoRndSeed(hash [32]byte, n int) int {
	vrfVal := new(big.Int).SetBytes(hash[:])
	numSeed := new(big.Int)
	vrfVal.QuoRem(vrfVal, big.NewInt(int64(n)), numSeed)
	return int(numSeed.Int64())
}

func nextPair(prevRnd, n int, pairs mapset.Set) (num1, num2, rnd int) {
	num1, num2, rnd = generatePair(prevRnd, n)
	counter := 0
	for counter < 5 && !checkPair(num1, num2, pairs) {
		num1, num2, rnd = generatePair(rnd, n)
		counter++
	}

	pairs.Add(pairToString(num1, num2))
	pairs.Add(pairToString(num2, num1))
	return
}

func generatePair(prevRnd, n int) (num1, num2, rnd int) {
	rnd = nextRnd(prevRnd)
	num1 = rnd % n
	rnd = nextRnd(rnd)
	num2 = rnd % n
	return
}

func checkPair(num1, num2 int, pairs mapset.Set) bool {
	return num1 != num2 && !pairs.Contains(pairToString(num1, num2))
}

func pairToString(num1, num2 int) string {
	return fmt.Sprintf("%d;%d", num1, num2)
}

func nextRnd(prev int) int {
	return (a*prev + c) % m
}
