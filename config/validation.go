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
		ValidationInterval:       time.Hour * 1,
		FlipLotteryDuration:      time.Minute * 5,
		ShortSessionDuration:     time.Minute,
		LongSessionDuration:      time.Minute * 5,
		AfterLongSessionDuration: time.Minute * 2,
	}
}
