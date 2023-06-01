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
	require.Equal(3, f)

	e, f = NetworkParams(10000000)
	require.Equal(204, e)
	require.Equal(3, f)
}

func TestLongSessionFlipsCount(t *testing.T) {
	require := require.New(t)

	require.Equal(uint(5), LongSessionFlipsCount(1))
	require.Equal(uint(6), LongSessionFlipsCount(2))
	require.Equal(uint(15), LongSessionFlipsCount(5))
	require.Equal(uint(30), LongSessionFlipsCount(10))
	require.Equal(uint(30), LongSessionFlipsCount(1000))
	require.Equal(uint(30), LongSessionFlipsCount(3000))
	require.Equal(uint(30), LongSessionFlipsCount(5000))
	require.Equal(uint(30), LongSessionFlipsCount(30000))
	require.Equal(uint(30), LongSessionFlipsCount(100000))
}

func TestNormalizedEpochDurationOld(t *testing.T) {
	const enableUpgrade12 = false
	day := time.Hour * 24
	saturday := time.Date(2020, 1, 4, 0, 0, 0, 0, time.UTC)
	require.Equal(t, day, NormalizedEpochDuration(saturday, 1, enableUpgrade12))
	require.Equal(t, day*3, NormalizedEpochDuration(saturday, 17, enableUpgrade12))
	require.Equal(t, day*4, NormalizedEpochDuration(saturday, 45, enableUpgrade12))
	require.Equal(t, day*5, NormalizedEpochDuration(saturday, 96, enableUpgrade12))
	require.Equal(t, day*5, NormalizedEpochDuration(saturday, 124, enableUpgrade12))
	require.Equal(t, day*6, NormalizedEpochDuration(saturday, 183, enableUpgrade12))
	require.Equal(t, day*6, NormalizedEpochDuration(saturday, 270, enableUpgrade12))
	require.Equal(t, day*6, NormalizedEpochDuration(saturday, 275, enableUpgrade12))
	require.Equal(t, day*8, NormalizedEpochDuration(saturday, 449, enableUpgrade12))
	require.Equal(t, day*10, NormalizedEpochDuration(saturday, 1158, enableUpgrade12))
	require.Equal(t, day*15, NormalizedEpochDuration(saturday, 3306, enableUpgrade12))
	require.Equal(t, day*21, NormalizedEpochDuration(saturday, 24284, enableUpgrade12))
	require.Equal(t, day*21, NormalizedEpochDuration(saturday, 34700, enableUpgrade12))
	require.Equal(t, day*28, NormalizedEpochDuration(saturday, 34701, enableUpgrade12))
	require.Equal(t, day*28, NormalizedEpochDuration(saturday, 50000, enableUpgrade12))
	require.Equal(t, day*28, NormalizedEpochDuration(saturday, 60000, enableUpgrade12))
	require.Equal(t, day*28, NormalizedEpochDuration(saturday, 75000, enableUpgrade12))
	require.Equal(t, day*28, NormalizedEpochDuration(saturday, 100000, enableUpgrade12))
	require.Equal(t, day*28, NormalizedEpochDuration(saturday, 500000, enableUpgrade12))
	require.Equal(t, day*28, NormalizedEpochDuration(saturday, 1000000, enableUpgrade12))
	require.Equal(t, day*28, NormalizedEpochDuration(saturday, 100000000, enableUpgrade12))

	notSaturday := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(t, day, NormalizedEpochDuration(notSaturday, 1, enableUpgrade12))
	require.Equal(t, day*3, NormalizedEpochDuration(notSaturday, 17, enableUpgrade12))
	require.Equal(t, day*4, NormalizedEpochDuration(notSaturday, 45, enableUpgrade12))
	require.Equal(t, day*5, NormalizedEpochDuration(notSaturday, 96, enableUpgrade12))
	require.Equal(t, day*5, NormalizedEpochDuration(notSaturday, 124, enableUpgrade12))
	require.Equal(t, day*6, NormalizedEpochDuration(notSaturday, 183, enableUpgrade12))
	require.Equal(t, day*6, NormalizedEpochDuration(notSaturday, 270, enableUpgrade12))
	require.Equal(t, day*6, NormalizedEpochDuration(notSaturday, 275, enableUpgrade12))
	require.Equal(t, day*8, NormalizedEpochDuration(notSaturday, 449, enableUpgrade12))
	require.Equal(t, day*10, NormalizedEpochDuration(notSaturday, 1158, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 9441, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 9442, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 24284, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 34701, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 50000, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 60000, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 75000, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 100000, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 500000, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 1000000, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 100000000, enableUpgrade12))
}

func TestNormalizedEpochDuration(t *testing.T) {
	const enableUpgrade12 = true
	day := time.Hour * 24
	saturday := time.Date(2020, 1, 4, 0, 0, 0, 0, time.UTC)
	require.Equal(t, day, NormalizedEpochDuration(saturday, 1, enableUpgrade12))
	require.Equal(t, day*2, NormalizedEpochDuration(saturday, 16, enableUpgrade12))
	require.Equal(t, day*3, NormalizedEpochDuration(saturday, 17, enableUpgrade12))
	require.Equal(t, day*3, NormalizedEpochDuration(saturday, 44, enableUpgrade12))
	require.Equal(t, day*4, NormalizedEpochDuration(saturday, 45, enableUpgrade12))
	require.Equal(t, day*4, NormalizedEpochDuration(saturday, 95, enableUpgrade12))
	require.Equal(t, day*5, NormalizedEpochDuration(saturday, 96, enableUpgrade12))
	require.Equal(t, day*5, NormalizedEpochDuration(saturday, 175, enableUpgrade12))
	require.Equal(t, day*6, NormalizedEpochDuration(saturday, 176, enableUpgrade12))
	require.Equal(t, day*6, NormalizedEpochDuration(saturday, 290, enableUpgrade12))
	require.Equal(t, day*14, NormalizedEpochDuration(saturday, 291, enableUpgrade12))
	require.Equal(t, day*14, NormalizedEpochDuration(saturday, 5844, enableUpgrade12))
	require.Equal(t, day*21, NormalizedEpochDuration(saturday, 5845, enableUpgrade12))
	require.Equal(t, day*21, NormalizedEpochDuration(saturday, 16202, enableUpgrade12))
	require.Equal(t, day*28, NormalizedEpochDuration(saturday, 16203, enableUpgrade12))
	require.Equal(t, day*28, NormalizedEpochDuration(saturday, 9999999, enableUpgrade12))

	notSaturday := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(t, day, NormalizedEpochDuration(notSaturday, 1, enableUpgrade12))
	require.Equal(t, day*2, NormalizedEpochDuration(notSaturday, 16, enableUpgrade12))
	require.Equal(t, day*3, NormalizedEpochDuration(notSaturday, 17, enableUpgrade12))
	require.Equal(t, day*3, NormalizedEpochDuration(notSaturday, 44, enableUpgrade12))
	require.Equal(t, day*4, NormalizedEpochDuration(notSaturday, 45, enableUpgrade12))
	require.Equal(t, day*4, NormalizedEpochDuration(notSaturday, 95, enableUpgrade12))
	require.Equal(t, day*5, NormalizedEpochDuration(notSaturday, 96, enableUpgrade12))
	require.Equal(t, day*5, NormalizedEpochDuration(notSaturday, 175, enableUpgrade12))
	require.Equal(t, day*6, NormalizedEpochDuration(notSaturday, 176, enableUpgrade12))
	require.Equal(t, day*6, NormalizedEpochDuration(notSaturday, 290, enableUpgrade12))
	require.Equal(t, day*13, NormalizedEpochDuration(notSaturday, 291, enableUpgrade12))
	require.Equal(t, day*13, NormalizedEpochDuration(notSaturday, 5844, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 5845, enableUpgrade12))
	require.Equal(t, day*20, NormalizedEpochDuration(notSaturday, 16202, enableUpgrade12))
	require.Equal(t, day*27, NormalizedEpochDuration(notSaturday, 16203, enableUpgrade12))
	require.Equal(t, day*27, NormalizedEpochDuration(notSaturday, 9999999, enableUpgrade12))
}

func Test_Scores(t *testing.T) {
	cases := [][]interface{}{
		{float32(0), uint32(4)},
		{float32(1.5), uint32(5)},
		{float32(3.5), uint32(6)},
		{float32(4), uint32(4)},
		{float32(5.5), uint32(6)},
		{float32(6), uint32(6)},
	}

	for _, item := range cases {
		a, b := item[0].(float32), item[1].(uint32)
		res := EncodeScore(a, b)
		x, y := DecodeScore(res)
		require.Equal(t, a, x)
		require.Equal(t, b, y)
	}
}
