package config

import (
	"idena-go/common"
	"time"
)

const (
	FlipLottery      = 1 * time.Minute
	ShortSession     = 2 * time.Minute
	AfterLongSession = 1 * time.Minute
)

type ValidationConfig struct {
	ValidationInterval       time.Duration
	FlipLotteryDuration      time.Duration
	ShortSessionDuration     time.Duration
	LongSessionDuration      time.Duration
	AfterLongSessionDuration time.Duration
}

func (cfg *ValidationConfig) GetEpochDuration(networkSize int) time.Duration {
	if cfg.ValidationInterval > 0 {
		return cfg.ValidationInterval
	}
	e, _, _ := common.NetworkParams(networkSize)
	return time.Hour * 24 * time.Duration(e)
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

func (cfg *ValidationConfig) GetAfterLongSessionDuration() time.Duration {
	if cfg.AfterLongSessionDuration > 0 {
		return cfg.AfterLongSessionDuration
	}
	return AfterLongSession
}
