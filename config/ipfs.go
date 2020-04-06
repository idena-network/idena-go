package config

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
	BlockPinThreshold  float64
	FlipPinThreshold   float64
}

func GetDefaultIpfsConfig() *IpfsConfig {
	return &IpfsConfig{
		BlockPinThreshold: 0.3,
		FlipPinThreshold:  0.3,
	}
}
