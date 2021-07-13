package common

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCalculateShardsNumber(t *testing.T) {

	cases := []struct {
		network       int
		currentShards int
		expected      int
	}{
		{
			network:       100,
			currentShards: 1,
			expected:      1,
		},
		{
			network:       3001,
			currentShards: 1,
			expected:      2,
		},
		{
			network:       3010,
			currentShards: 2,
			expected:      2,
		},
		{
			network:       24000,
			currentShards: 2,
			expected:      16,
		},
		{
			network:       24000,
			currentShards: 8,
			expected:      16,
		},
		{
			network:       30000,
			currentShards: 16,
			expected:      16,
		},
		{
			network:       22000,
			currentShards: 16,
			expected:      8,
		},
		{
			network:       1,
			currentShards: 16,
			expected:      1,
		},
		{
			network:       0,
			currentShards: 16,
			expected:      1,
		},
		{
			network: 2800,
			currentShards: 2,
			expected: 1,
		},
		{
			network: 2801,
			currentShards: 2,
			expected: 2,
		},
	}

	for idx, c := range cases {
		require.Equal(t, c.expected, CalculateShardsNumber(1400, 3000, c.network, c.currentShards), fmt.Sprintf("error in %v case", idx))
	}
}
