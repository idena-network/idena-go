package blockchain

import (
	"github.com/idena-network/idena-go/config"
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

func (t *timing) isFlipLotteryStarted(nextValidation time.Time, timestamp int64) bool {
	if current := time.Unix(timestamp, 0); nextValidation.Sub(current) < t.conf.GetFlipLotteryDuration() {
		return true
	}
	return false
}

func (t *timing) isShortSessionStarted(nextValidation time.Time, timestamp int64) bool {
	return !time.Unix(timestamp, 0).Before(nextValidation)
}

func (t *timing) isLongSessionStarted(nextValidation time.Time, timestamp int64) bool {
	if current := time.Unix(timestamp, 0); current.Sub(nextValidation) > t.conf.GetShortSessionDuration() {
		return true
	}
	return false
}

func (t *timing) isAfterLongSessionStarted(nextValidation time.Time, timestamp int64, networkSize int) bool {
	if current := time.Unix(timestamp, 0); current.Sub(nextValidation) > t.conf.GetShortSessionDuration()+t.conf.GetLongSessionDuration(networkSize) {
		return true
	}
	return false
}
