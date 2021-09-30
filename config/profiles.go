package config

func applyLowPowerProfile(cfg *Config) {
	cfg.P2P.MaxInboundPeers = LowPowerMaxInboundNotOwnShardPeers
	cfg.P2P.MaxOutboundPeers = LowPowerMaxOutboundNotOwnShardPeers
	cfg.P2P.MaxInboundOwnShardPeers = LowPowerMaxInboundOwnShardPeers
	cfg.P2P.MaxOutboundOwnShardPeers = LowPowerMaxOutboundOwnShardPeers
	cfg.IpfsConf.LowWater = 8
	cfg.IpfsConf.HighWater = 10
	cfg.IpfsConf.GracePeriod = "30s"
	cfg.IpfsConf.ReproviderInterval = "0"
	cfg.IpfsConf.Routing = "dhtclient"
}

func applySharedNodeProfile(cfg *Config) {
	cfg.P2P.Multishard = true
	cfg.Sync.LoadAllFlips = true
	cfg.IpfsConf.LowWater = 50
	cfg.IpfsConf.HighWater = 100
}

func applyDefaultProfile(cfg *Config) {
	cfg.P2P.MaxInboundPeers = DefaultMaxInboundNotOwnShardPeers
	cfg.P2P.MaxOutboundPeers = DefaultMaxOutboundNotOwnShardPeers
	cfg.P2P.MaxInboundOwnShardPeers = DefaultMaxInboundOwnShardPeers
	cfg.P2P.MaxOutboundOwnShardPeers = DefaultMaxOutboundOwnShardPeers
	cfg.IpfsConf.LowWater = 30
	cfg.IpfsConf.HighWater = 50
	cfg.P2P.Multishard = false
	cfg.Sync.LoadAllFlips = false
}
