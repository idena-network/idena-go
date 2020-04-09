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
	BlockPinThreshold  float32
	FlipPinThreshold   float32
}

func GetDefaultIpfsConfig() *IpfsConfig {
	return &IpfsConfig{
		BlockPinThreshold: 0.3,
		FlipPinThreshold:  0.5,
	}
}
