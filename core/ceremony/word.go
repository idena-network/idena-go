package ceremony

import (
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/vrf/p256"
	"math/big"
)

const (
	a = 1103515245
	c = 12345
	m = 1 << 16
)

func (vc *ValidationCeremony) GeneratePairs(seed []byte, dictionarySize, pairCount int) (nums []int, proof []byte) {
	hash, proof := vc.secStore.VrfEvaluate(seed)
	rnd := generatePseudoRndSeed(hash, dictionarySize)
	pairs := mapset.NewSet()
	for i := 0; i < pairCount; i++ {
		var num1, num2 int
		num1, num2, rnd = nextPair(rnd, dictionarySize, pairCount, pairs)
		nums = append(nums, num1, num2)
	}
	return nums, proof
}

func CheckPair(seed []byte, proof []byte, pubKeyData []byte, dictionarySize, pairCount, num1, num2 int) bool {
	pubKey, err := crypto.UnmarshalPubkey(pubKeyData)
	if err != nil {
		return false
	}
	verifier, err := p256.NewVRFVerifier(pubKey)
	if err != nil {
		return false
	}
	hash, err := verifier.ProofToHash(seed, proof)
	if err != nil {
		return false
	}
	rnd := generatePseudoRndSeed(hash, dictionarySize)
	pairs := mapset.NewSet()
	for i := 0; i < pairCount; i++ {
		var val1, val2 int
		val1, val2, rnd = nextPair(rnd, dictionarySize, pairCount, pairs)
		if val1 == num1 && val2 == num2 {
			return true
		}
	}
	return false
}

func maxUniquePairs(dictionarySize int) int {
	return (dictionarySize - 1) * dictionarySize / 2
}

func generatePseudoRndSeed(hash [32]byte, dictionarySize int) int {
	vrfVal := new(big.Int).SetBytes(hash[:])
	numSeed := new(big.Int)
	vrfVal.QuoRem(vrfVal, big.NewInt(int64(dictionarySize)), numSeed)
	return int(numSeed.Int64())
}

func nextPair(prevRnd, dictionarySize, pairCount int, pairs mapset.Set) (num1, num2, rnd int) {
	num1, num2, rnd = generatePair(prevRnd, dictionarySize)
	maxPairs := maxUniquePairs(dictionarySize)
	for pairCount <= maxPairs && !checkPair(num1, num2, pairs) {
		num1, num2, rnd = generatePair(rnd, dictionarySize)
	}

	pairs.Add(pairToString(num1, num2))
	pairs.Add(pairToString(num2, num1))
	return
}

func generatePair(prevRnd, dictionarySize int) (num1, num2, rnd int) {
	rnd = nextRnd(prevRnd)
	num1 = rnd % dictionarySize
	rnd = nextRnd(rnd)
	num2 = rnd % dictionarySize
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
