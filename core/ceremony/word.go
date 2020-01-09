package ceremony

import (
	"encoding/binary"
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/vrf/p256"
	"github.com/pkg/errors"
	"math/rand"
)

func (vc *ValidationCeremony) GeneratePairs(seed []byte, dictionarySize, pairCount int, epoch uint16) (nums []int, proof []byte) {
	if epoch <= epochToUseOldWords {
		return vc.generatePairsOld(seed, dictionarySize, pairCount)
	}
	hash, proof := vc.secStore.VrfEvaluate(seed)
	rnd := newRand(hash[:])
	pairs := mapset.NewSet()
	for i := 0; i < pairCount; i++ {
		var num1, num2 int
		num1, num2 = nextPair(rnd, dictionarySize, pairCount, pairs)
		nums = append(nums, num1, num2)
	}
	return nums, proof
}

func GetWords(seed []byte, proof []byte, pubKeyData []byte, dictionarySize, pairCount, pairIndex int, epoch uint16) (word1, word2 int, err error) {
	if epoch <= epochToUseOldWords {
		return getWordsOld(seed, proof, pubKeyData, dictionarySize, pairCount, pairIndex)
	}
	pubKey, err := crypto.UnmarshalPubkey(pubKeyData)
	if err != nil {
		return 0, 0, err
	}
	verifier, err := p256.NewVRFVerifier(pubKey)
	if err != nil {
		return 0, 0, err
	}
	hash, err := verifier.ProofToHash(seed, proof)
	if err != nil {
		return 0, 0, err
	}
	rnd := newRand(hash[:])
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

func newRand(seed []byte) *rand.Rand {
	return rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(seed))))
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
