package common

import (
	math2 "github.com/idena-network/idena-go/common/math"
	"math"
)

const (
	ShortSessionFlips      = 5
	LongSessionTesters     = 10
	ShortSessionExtraFlips = 2
)

func ShortSessionExtraFlipsCount() uint {
	return ShortSessionExtraFlips
}

func ShortSessionFlipsCount() uint {
	return ShortSessionFlips
}

func LongSessionFlipsCount(networkSize int) uint {
	_, _, flipsPerIdentity := NetworkParams(networkSize)
	totalFlips := uint64(flipsPerIdentity * networkSize)
	return uint(math2.Max(5, math2.Min(totalFlips, uint64(flipsPerIdentity*LongSessionTesters))))
}

func NetworkParams(networkSize int) (epochDuration int, invites int, flips int) {
	epochDurationF := math.Round(math.Pow(float64(networkSize), 0.33))
	epochDuration = int(epochDurationF)

	if networkSize == 0 {
		return 1, 0, 0
	}

	var invitesCount float64
	if networkSize > 1 {
		invitesCount = 20 / math.Pow(math.Log10(float64(networkSize)), 2)
	} else {
		invitesCount = 1000
	}

	if invitesCount < 1 {
		flips = int(math.Round((1 + invitesCount) * 5))
	} else {
		flips = int(math.Round(epochDurationF * 2.0 / 7.0))
	}

	flips = int(math2.Max(3, uint64(flips)))
	invites = int(math2.Min(5, uint64(math.Round(invitesCount))))
	return
}
