package config

type P2P struct {
	MaxInboundPeers  int
	MaxOutboundPeers int
	MaxDelay         int
	DisableMetrics   bool
}
