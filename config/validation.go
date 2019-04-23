package config

import (
	"time"
)

type ValidationConfig struct {
	ValidationInterval    time.Duration
	TimeBeforeFlipLottery time.Duration
	TimeBeforeValidation  time.Duration
	TimeoutBeforeEnd      time.Duration
}

func GetDefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		ValidationInterval:    time.Minute * 30,
		TimeBeforeFlipLottery: time.Minute * 5,
		TimeBeforeValidation:  time.Minute * 2,
		TimeoutBeforeEnd:      time.Minute * 2,
	}
}
