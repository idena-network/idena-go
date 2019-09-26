package config

type IpfsConfig struct {
	DataDir            string
	BootNodes          []string
	IpfsPort           int
	SwarmKey           string
	Routing            string
	LowWater           int
	HighWater          int
	GracePeriod        string
	ReproviderInterval string
}
