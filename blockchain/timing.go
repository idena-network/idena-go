package blockchain

import (
	"idena-go/config"
	"math/big"
	"time"
)

type timing struct {
	conf *config.ValidationConfig
}

func NewTiming(conf *config.ValidationConfig) *timing {
	return &timing{
		conf: conf,
	}
}

func (t *timing) isFlipLotteryStarted(nextValidation time.Time, timestamp *big.Int) bool {
	if current := timestampToTime(timestamp); nextValidation.Sub(current) < t.conf.TimeBeforeFlipLottery {
		return true
	}
	return false
}

func (t *timing) isValidationStarted(nextValidation time.Time, timestamp *big.Int) bool {
	if current := timestampToTime(timestamp); nextValidation.Sub(current) < t.conf.TimeBeforeValidation {
		return true
	}
	return false
}

func (t *timing) isValidationFinished(nextValidation time.Time, timestamp *big.Int) bool {
	if current := timestampToTime(timestamp); current.Sub(nextValidation) > t.conf.TimeoutBeforeEnd {
		return true
	}
	return false
}

func timestampToTime(timestamp *big.Int) time.Time {
	return time.Unix(timestamp.Int64(), 0)
}
