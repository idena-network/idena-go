package config

import "time"

type IpfsConfig struct {
	DataDir            string
	BootNodes          []string
	IpfsPort           int
	StaticPort         bool
	SwarmKey           string
	Routing            string
	LowWater           int
	HighWater          int
	GracePeriod        string
	ReproviderInterval string
	Profile            string
	BlockPinThreshold  float32
	FlipPinThreshold   float32
	PublishPeers       bool
	Gc                 IpfsGcConfig
}

type IpfsGcConfig struct {
	Enabled                  bool
	Interval                 time.Duration
	Timeout                  time.Duration
	IntervalBeforeValidation time.Duration
	NotificationDelay        time.Duration
}

func GetDefaultIpfsConfig() *IpfsConfig {
	return &IpfsConfig{
		BlockPinThreshold: 0.3,
		FlipPinThreshold:  0.5,
		Profile:           "server",
		Gc: IpfsGcConfig{
			Enabled:                  true,
			Interval:                 time.Hour * 24,
			Timeout:                  time.Minute * 10,
			IntervalBeforeValidation: time.Hour * 12,
			NotificationDelay:        time.Minute,
		},
	}
}
