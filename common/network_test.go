package common

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNetworkParams(t *testing.T) {
	require := require.New(t)

	e, i, f := NetworkParams(1)
	require.Equal(1, e)
	require.Equal(1, i)
	require.Equal(3, f)

	e, i, f = NetworkParams(5)
	require.Equal(2, e)
	require.Equal(1, i)
	require.Equal(3, f)

	e, i, f = NetworkParams(100)
	require.Equal(5, e)
	require.Equal(1, i)
	require.Equal(3, f)

	e, i, f = NetworkParams(1000)
	require.Equal(10, e)
	require.Equal(1, i)
	require.Equal(3, f)

	e, i, f = NetworkParams(50000)
	require.Equal(36, e)
	require.Equal(1, i)
	require.Equal(10, f)

	e, i, f = NetworkParams(10000000)
	require.Equal(204, e)
	require.Equal(0, i)
	require.Equal(7, f)
}

func TestLongSessionFlipsCount(t *testing.T) {
	require := require.New(t)

	require.Equal(uint(5), LongSessionFlipsCount(1))
	require.Equal(uint(6), LongSessionFlipsCount(2))
	require.Equal(uint(15), LongSessionFlipsCount(5))
	require.Equal(uint(30), LongSessionFlipsCount(10))
	require.Equal(uint(30), LongSessionFlipsCount(1000))
	require.Equal(uint(40), LongSessionFlipsCount(3000))
	require.Equal(uint(50), LongSessionFlipsCount(5000))
	require.Equal(uint(100), LongSessionFlipsCount(30000))
	require.Equal(uint(90), LongSessionFlipsCount(100000))
}
