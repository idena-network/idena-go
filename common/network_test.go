package common

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestNetworkParams(t *testing.T) {
	require := require.New(t)

	e, f := NetworkParams(1)
	require.Equal(1, e)
	require.Equal(3, f)

	e, f = NetworkParams(5)
	require.Equal(2, e)
	require.Equal(3, f)

	e, f = NetworkParams(100)
	require.Equal(5, e)
	require.Equal(3, f)

	e, f = NetworkParams(1000)
	require.Equal(10, e)
	require.Equal(3, f)

	e, f = NetworkParams(50000)
	require.Equal(36, e)
	require.Equal(10, f)

	e, f = NetworkParams(10000000)
	require.Equal(204, e)
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

func TestNormalizedEpochDuration(t *testing.T) {
	day := time.Hour * 24
	saturday := time.Date(2020, 1, 4, 0, 0, 0, 0, time.UTC)
	require.Equal(t, day, NormalizedEpochDuration(saturday, 1))
	require.Equal(t, day*3, NormalizedEpochDuration(saturday, 17))
	require.Equal(t, day*4, NormalizedEpochDuration(saturday, 45))
	require.Equal(t, day*5, NormalizedEpochDuration(saturday, 96))
	require.Equal(t, day*5, NormalizedEpochDuration(saturday, 124))
	require.Equal(t, day*6, NormalizedEpochDuration(saturday, 183))
	require.Equal(t, day*6, NormalizedEpochDuration(saturday, 270))
	require.Equal(t, day*6, NormalizedEpochDuration(saturday, 275))
	require.Equal(t, day*8, NormalizedEpochDuration(saturday, 449))
	require.Equal(t, day*10, NormalizedEpochDuration(saturday, 1158))
	require.Equal(t, day*21, NormalizedEpochDuration(saturday, 9441))
	require.Equal(t, day*21, NormalizedEpochDuration(saturday, 24284))
	require.Equal(t, day*21, NormalizedEpochDuration(saturday, 34700))
	require.Equal(t, day*42, NormalizedEpochDuration(saturday, 34701))
	require.Equal(t, day*42, NormalizedEpochDuration(saturday, 50000))
	require.Equal(t, day*42, NormalizedEpochDuration(saturday, 60000))
	require.Equal(t, day*42, NormalizedEpochDuration(saturday, 75000))
	require.Equal(t, day*42, NormalizedEpochDuration(saturday, 100000))
	require.Equal(t, day*84, NormalizedEpochDuration(saturday, 500000))
	require.Equal(t, day*91, NormalizedEpochDuration(saturday, 1000000))
	require.Equal(t, day*91, NormalizedEpochDuration(saturday, 100000000))

	notSaturday := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(t, day, NormalizedEpochDuration(notSaturday, 1))
	require.Equal(t, day*3, NormalizedEpochDuration(notSaturday, 17))
	require.Equal(t, day*4, NormalizedEpochDuration(notSaturday, 45))
	require.Equal(t, day*5, NormalizedEpochDuration(notSaturday, 96))
	require.Equal(t, day*5, NormalizedEpochDuration(notSaturday, 124))
	require.Equal(t, day*6, NormalizedEpochDuration(notSaturday, 183))
	require.Equal(t, day*6, NormalizedEpochDuration(notSaturday, 270))
	require.Equal(t, day*6, NormalizedEpochDuration(notSaturday, 275))
	require.Equal(t, day*8, NormalizedEpochDuration(notSaturday, 449))
	require.Equal(t, day*10, NormalizedEpochDuration(notSaturday, 1158))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 9441))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 9442))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 24284))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 34701))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 50000))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 60000))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 75000))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 100000))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 500000))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 1000000))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 100000000))
}
