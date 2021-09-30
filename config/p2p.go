package config

type P2P struct {
	MaxInboundPeers  int
	MaxOutboundPeers int

	MaxInboundOwnShardPeers  int
	MaxOutboundOwnShardPeers int

	MaxDelay       int
	DisableMetrics bool
	Multishard     bool
}
