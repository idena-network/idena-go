package config

import "time"

type OfflineDetectionConfig struct {
	PersistInterval             int
	MaxSelfOffline              time.Duration
	OfflineProposeInterval      time.Duration
	OfflineVoteInterval         time.Duration
	IntervalBetweenOfflineRetry time.Duration
}

func GetDefaultOfflineDetectionConfig() *OfflineDetectionConfig {
	return &OfflineDetectionConfig{
		PersistInterval:             50,
		MaxSelfOffline:              1 * time.Hour,
		OfflineProposeInterval:      1 * time.Hour,
		OfflineVoteInterval:         45 * time.Minute,
		IntervalBetweenOfflineRetry: 10 * time.Minute,
	}
}
