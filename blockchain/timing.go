package blockchain

import (
	"idena-go/common"
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
	if current := common.TimestampToTime(timestamp); nextValidation.Sub(current) < t.conf.GetFlipLotteryDuration() {
		return true
	}
	return false
}

func (t *timing) isShortSessionStarted(nextValidation time.Time, timestamp *big.Int) bool {
	return !common.TimestampToTime(timestamp).Before(nextValidation)
}

func (t *timing) isLongSessionStarted(nextValidation time.Time, timestamp *big.Int) bool {
	if current := common.TimestampToTime(timestamp); current.Sub(nextValidation) > t.conf.GetShortSessionDuration() {
		return true
	}
	return false
}

func (t *timing) isAfterLongSessionStarted(nextValidation time.Time, timestamp *big.Int, networkSize int) bool {
	if current := common.TimestampToTime(timestamp); current.Sub(nextValidation) > t.conf.GetShortSessionDuration()+t.conf.GetLongSessionDuration(networkSize) {
		return true
	}
	return false
}

func (t *timing) isValidationFinished(nextValidation time.Time, timestamp *big.Int, networkSize int) bool {
	if current := common.TimestampToTime(timestamp); current.Sub(nextValidation) >
		t.conf.GetShortSessionDuration()+t.conf.GetLongSessionDuration(networkSize)+t.conf.GetAfterLongSessionDuration() {
		return true
	}
	return false
}
