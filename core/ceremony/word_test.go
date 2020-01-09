package ceremony

import (
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_GeneratePairs(t *testing.T) {
	secStore := &secstore.SecStore{}
	vc := &ValidationCeremony{
		secStore: secStore,
	}
	key, _ := crypto.GenerateKey()
	secStore.AddKey(crypto.FromECDSA(key))

	for _, tc := range []struct {
		dictionarySize  int
		pairCount       int
		checkUniqueness bool
	}{
		{10, 2, true},
		{3300, 9, true},
		{100, 50, true},
		{10, 20, true},
		{3, 3, true},
		{1, 1, false},
		{3, 4, false},
	} {
		nums, proof := vc.GeneratePairs([]byte("data"), tc.dictionarySize, tc.pairCount, 50)

		require.Equal(t, tc.pairCount*2, len(nums))
		require.NotNil(t, proof)

		for i := 0; i < len(nums); i++ {
			require.True(t, nums[i] < tc.dictionarySize)
		}

		if !tc.checkUniqueness {
			continue
		}

		// Check there is no pair with same values
		for i := 0; i < tc.pairCount; i++ {
			require.NotEqual(t, nums[i*2], nums[i*2+1])
		}

		// Check there is no same pairs
		pairs := mapset.NewSet()
		for i := 0; i < tc.pairCount; i++ {
			require.False(t, pairs.Contains(fmt.Sprintf("%d;%d", nums[i*2], nums[i*2+1])))
			pairs.Add(fmt.Sprintf("%d;%d", nums[i*2], nums[i*2+1]))
			pairs.Add(fmt.Sprintf("%d;%d", nums[i*2+1], nums[i*2]))
		}
	}
}

func Test_GetWords(t *testing.T) {
	require := require.New(t)
	secStore := &secstore.SecStore{}
	vc := &ValidationCeremony{
		secStore: secStore,
	}

	key, _ := crypto.GenerateKey()
	secStore.AddKey(crypto.FromECDSA(key))
	pk := secStore.GetPubKey()
	wrongKey, _ := crypto.GenerateKey()
	seed := []byte("data1")
	dictionarySize := 3300
	pairCount := 9
	nums, proof := vc.GeneratePairs(seed, dictionarySize, pairCount, 50)

	w1, w2, _ := GetWords(seed, proof, pk, dictionarySize, pairCount, 1, 50)
	require.Equal(nums[2], w1)
	require.Equal(nums[3], w2)

	w1, w2, _ = GetWords(seed, proof, pk, dictionarySize, pairCount, 8, 50)
	require.Equal(nums[16], w1)
	require.Equal(nums[17], w2)

	w1, w2, err := GetWords(seed, proof, pk, dictionarySize, pairCount, 15, 50)
	require.Error(err, "out of bounds pair index should throw error")

	_, _, err = GetWords([]byte("data2"), proof, pk, dictionarySize, pairCount, 1, 50)
	require.EqualErrorf(err, "invalid VRF proof", "invalid proof should throw error")

	_, _, err = GetWords(seed, proof, crypto.FromECDSAPub(&wrongKey.PublicKey), dictionarySize, pairCount, 1, 50)
	require.EqualErrorf(err, "invalid VRF proof", "invalid proof should throw error")
}

func Test_maxUniquePairs(t *testing.T) {
	require.Equal(t, 0, maxUniquePairs(0))
	require.Equal(t, 0, maxUniquePairs(1))
	require.Equal(t, 1, maxUniquePairs(2))
	require.Equal(t, 3, maxUniquePairs(3))
	require.Equal(t, 6, maxUniquePairs(4))
	require.Equal(t, 10, maxUniquePairs(5))
	require.Equal(t, 15, maxUniquePairs(6))
	require.Equal(t, 21, maxUniquePairs(7))
	require.Equal(t, 28, maxUniquePairs(8))
}
