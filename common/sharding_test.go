package common

import (
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
			network:       3000,
			currentShards: 1,
			expected:      2,
		},
		{
			network:       3000,
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
	}

	for _, c := range cases {
		require.Equal(t, c.expected, CalculateShardsNumber(c.network, c.currentShards))
	}
}
