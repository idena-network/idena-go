package common

import (
	"math/big"
	"time"
)

func TimestampToTime(timestamp *big.Int) time.Time {
	return time.Unix(timestamp.Int64(), 0)
}
