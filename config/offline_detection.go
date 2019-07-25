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
		MaxSelfOffline:              12 * time.Hour,
		OfflineProposeInterval:      24 * time.Hour,
		OfflineVoteInterval:         12 * time.Hour,
		IntervalBetweenOfflineRetry: 1 * time.Hour,
	}
}
