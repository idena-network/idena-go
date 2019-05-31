package config

import (
	"time"
)

type ValidationConfig struct {
	ValidationInterval       time.Duration
	FlipLotteryDuration      time.Duration
	ShortSessionDuration     time.Duration
	LongSessionDuration      time.Duration
	AfterLongSessionDuration time.Duration
}

func GetDefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		ValidationInterval:       time.Minute * 35,
		FlipLotteryDuration:      time.Minute * 1,
		ShortSessionDuration:     time.Minute * 3,
		LongSessionDuration:      time.Minute * 2,
		AfterLongSessionDuration: time.Second * 30,
	}
}
