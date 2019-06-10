package blockchain

import (
	"idena-go/common"
	"idena-go/config"
	"idena-go/core/validators"
	"math/big"
	"time"
)

type timing struct {
	conf       *config.ValidationConfig
	validators *validators.ValidatorsCache
}

func NewTiming(conf *config.ValidationConfig, validators *validators.ValidatorsCache) *timing {
	return &timing{
		conf:       conf,
		validators: validators,
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

func (t *timing) isAfterLongSessionStarted(nextValidation time.Time, timestamp *big.Int) bool {
	if current := common.TimestampToTime(timestamp); current.Sub(nextValidation) > t.conf.GetShortSessionDuration()+t.conf.GetLongSessionDuration(t.validators.NetworkSize()) {
		return true
	}
	return false
}

func (t *timing) isValidationFinished(nextValidation time.Time, timestamp *big.Int) bool {
	if current := common.TimestampToTime(timestamp); current.Sub(nextValidation) >
		t.conf.GetShortSessionDuration()+t.conf.GetLongSessionDuration(t.validators.NetworkSize())+t.conf.GetAfterLongSessionDuration() {
		return true
	}
	return false
}
