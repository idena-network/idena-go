package common

import (
	math2 "github.com/idena-network/idena-go/common/math"
	"math"
	"time"
)

const (
	ShortSessionFlips      = 6
	LongSessionTesters     = 10
	ShortSessionExtraFlips = 2

	WordDictionarySize = 3300
	WordPairsPerFlip   = 3

	MaxFlipSize    = 1024 * 600
	MaxProfileSize = 1024 * 1024
)

func ShortSessionExtraFlipsCount() uint {
	return ShortSessionExtraFlips
}

func ShortSessionFlipsCount() uint {
	return ShortSessionFlips
}

func LongSessionFlipsCount(networkSize int) uint {
	_, flipsPerIdentity := NetworkParams(networkSize)
	totalFlips := uint64(flipsPerIdentity * networkSize)
	return uint(math2.Max(5, math2.Min(totalFlips, uint64(flipsPerIdentity*LongSessionTesters))))
}

func NetworkParams(networkSize int) (epochDuration int, flips int) {
	if networkSize == 0 {
		return 1, 0
	}
	epochDuration = int(math.Round(math.Pow(float64(networkSize), 0.33)))
	flips = 3
	return
}

func NormalizedEpochDuration(validationTime time.Time, networkSize int) time.Duration {
	const day = 24 * time.Hour
	baseEpochDays, _ := NetworkParams(networkSize)
	if baseEpochDays < 21 {
		return day * time.Duration(baseEpochDays)
	}
	if validationTime.Weekday() != time.Saturday {
		return day * 20
	}
	return day * time.Duration(math2.MinInt(28, int(math.Round(math.Pow(float64(networkSize), 0.33)/21)*21)))
}

func GodAddressInvitesCount(networkSize int) uint16 {
	return uint16(math2.MinInt(500, math2.MaxInt(50, networkSize/3)))
}
