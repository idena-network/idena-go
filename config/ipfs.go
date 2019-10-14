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
}
