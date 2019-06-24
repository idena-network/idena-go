package flip

import (
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/stretchr/testify/require"
	"idena-go/crypto"
	"idena-go/secstore"
	"testing"
)

func Test_GeneratePairs(t *testing.T) {
	secStore := &secstore.SecStore{}
	flipper := &Flipper{
		secStore: secStore,
	}
	key, _ := crypto.GenerateKey()
	secStore.AddKey(crypto.FromECDSA(key))

	for _, tc := range []struct {
		dictionarySize int
		pairCount      int
	}{
		{10, 2},
		{3300, 9},
		{100, 50},
		{10, 20},
	} {
		nums, proof := flipper.GeneratePairs([]byte("data"), tc.dictionarySize, tc.pairCount)

		require.Equal(t, tc.pairCount*2, len(nums))
		require.NotNil(t, proof)

		for i := 0; i < len(nums); i++ {
			require.True(t, nums[i] >= 0 && nums[i] < tc.dictionarySize)
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

func Test_CheckPair(t *testing.T) {
	secStore := &secstore.SecStore{}
	flipper := &Flipper{
		secStore: secStore,
	}

	key, _ := crypto.GenerateKey()
	secStore.AddKey(crypto.FromECDSA(key))
	pk := secStore.GetPubKey()
	wrongKey, _ := crypto.GenerateKey()
	seed := []byte("data1")
	dictionarySize := 3300
	pairCount := 9
	nums, proof := flipper.GeneratePairs(seed, dictionarySize, pairCount)

	require.True(t, CheckPair(seed, proof, pk, dictionarySize, pairCount, nums[0], nums[1]))
	require.True(t, CheckPair(seed, proof, pk, dictionarySize, pairCount, nums[2], nums[3]))

	require.False(t, CheckPair([]byte("data2"), proof, pk, dictionarySize, 9, nums[0], nums[1]))
	require.False(t, CheckPair(seed, proof, crypto.FromECDSAPub(&wrongKey.PublicKey), dictionarySize, pairCount, nums[0], nums[1]))
	require.False(t, CheckPair(seed, proof, pk, dictionarySize, pairCount, dictionarySize+100, nums[3]))
}
