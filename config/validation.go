package config

import (
	"github.com/idena-network/idena-go/common"
	"time"
)

const (
	FlipLottery      = 5 * time.Minute
	ShortSession     = 2 * time.Minute
	AfterLongSession = 1 * time.Minute
)

type ValidationConfig struct {
	// Do not use directly
	ValidationInterval time.Duration
	// Do not use directly
	FlipLotteryDuration time.Duration
	// Do not use directly
	ShortSessionDuration time.Duration
	// Do not use directly
	LongSessionDuration time.Duration
}

func (cfg *ValidationConfig) GetNextValidationTime(validationTime time.Time, networkSize int, enableUpgrade12 bool) time.Time {
	if cfg.ValidationInterval > 0 {
		return validationTime.Add(cfg.ValidationInterval)
	}
	return validationTime.Add(common.NormalizedEpochDuration(validationTime, networkSize, enableUpgrade12))
}

func (cfg *ValidationConfig) GetFlipLotteryDuration() time.Duration {
	if cfg.FlipLotteryDuration > 0 {
		return cfg.FlipLotteryDuration
	}
	return FlipLottery
}

func (cfg *ValidationConfig) GetShortSessionDuration() time.Duration {
	if cfg.ShortSessionDuration > 0 {
		return cfg.ShortSessionDuration
	}
	return ShortSession
}

func (cfg *ValidationConfig) GetLongSessionDuration(networkSize int) time.Duration {
	if cfg.LongSessionDuration > 0 {
		return cfg.LongSessionDuration
	}
	return time.Minute * time.Duration(common.LongSessionFlipsCount(networkSize))
}
