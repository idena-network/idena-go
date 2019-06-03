package config

type IpfsConfig struct {
	DataDir   string
	BootNodes []string
	IpfsPort  int
	SwarmKey  string
}
