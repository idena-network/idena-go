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
		MaxSelfOffline:              90 * time.Minute,
		OfflineProposeInterval:      90 * time.Minute,
		OfflineVoteInterval:         45 * time.Minute,
		IntervalBetweenOfflineRetry: 5 * time.Minute,
	}
}
