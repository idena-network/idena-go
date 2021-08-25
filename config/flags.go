package config

import "github.com/urfave/cli"

const (
	DefaultDataDir        = "datadir"
	DefaultPort           = 40404
	DefaultRpcHost        = "localhost"
	DefaultRpcPort        = 9009
	DefaultIpfsDataDir    = "ipfs"
	DefaultIpfsPort       = 40405
	DefaultGodAddress     = "0x4d60dc6a2cba8c3ef1ba5e1eba5c12c54cee6b61"
	DefaultCeremonyTime   = int64(1567171800)
	DefaultSwarmKey       = "00d6f96bb2b02a7308ad87938d6139a974b555cc029ce416641a60c46db2f531"
	DefaultForceFullSync  = 100
	DefaultStoreCertRange = 2000

	DefaultMaxInboundOwnShardPeers     = 6
	DefaultMaxOutboundOwnShardPeers    = 3
	DefaultMaxInboundNotOwnShardPeers  = 2
	DefaultMaxOutboundNotOwnShardPeers = 1

	DefaultBurntTxRange = 180


	LowPowerMaxInboundOwnShardPeers     = 3
	LowPowerMaxOutboundOwnShardPeers    = 2
	LowPowerMaxInboundNotOwnShardPeers  = 1
	LowPowerMaxOutboundNotOwnShardPeers = 1
)

var (
	DefaultIpfsBootstrapNodes = []string{
		"/ip4/64.227.41.45/tcp/40405/ipfs/QmfJktBd2jf37Jx3eCYyn1fofbW511U5XvYiMp7233mLZM",
		"/ip4/135.181.40.10/tcp/40405/ipfs/QmNYWtiwM1UfeCmHfWSdefrMuQdg6nycY5yS64HYqWCUhD",
		"/ip4/165.227.91.202/tcp/40403/ipfs/QmZ9VnVZsokXEttRYiHbHmCUBSdzSQywjj5wM3Me96XoVD",
		"/ip4/161.35.18.60/tcp/40405/ipfs/QmaBWm6dXsc6Y1MZ1x4ZTqHdjy32PGmyGmNPmHnxGGgrXF",
	}
	CfgFileFlag = cli.StringFlag{
		Name:  "config",
		Usage: "JSON configuration file",
	}
	DataDirFlag = cli.StringFlag{
		Name:  "datadir",
		Usage: "datadir for blockchain",
	}
	TcpPortFlag = cli.IntFlag{
		Name:  "port",
		Usage: "Network listening port",
	}
	RpcHostFlag = cli.StringFlag{
		Name:  "rpcaddr",
		Usage: "RPC listening address",
	}
	RpcPortFlag = cli.IntFlag{
		Name:  "rpcport",
		Usage: "RPC listening port",
	}
	BootNodeFlag = cli.StringFlag{
		Name:  "bootnode",
		Usage: "Bootstrap node url",
	}
	AutomineFlag = cli.BoolFlag{
		Name:  "automine",
		Usage: "Mine blocks alone without peers",
	}
	IpfsBootNodeFlag = cli.StringFlag{
		Name:  "ipfsbootnode",
		Usage: "Ipfs bootstrap node (overrides existing)",
	}
	IpfsPortFlag = cli.IntFlag{
		Name:  "ipfsport",
		Usage: "Ipfs port",
	}
	NoDiscoveryFlag = cli.BoolFlag{
		Name:  "nodiscovery",
		Usage: "NoDiscovery can be used to disable the peer discovery mechanism.",
	}
	VerbosityFlag = cli.IntFlag{
		Name:  "verbosity",
		Usage: "Log verbosity",
		Value: 3,
	}
	GodAddressFlag = cli.StringFlag{
		Name:  "godaddress",
		Usage: "Idena god address",
	}
	CeremonyTimeFlag = cli.Int64Flag{
		Name:  "ceremonytime",
		Usage: "First ceremony time (unix)",
	}
	MaxNetworkDelayFlag = cli.IntFlag{
		Name:  "maxnetdelay",
		Usage: "Max network delay for broadcasting",
	}
	FastSyncFlag = cli.BoolFlag{
		Name:  "fast",
		Usage: "Enable fast sync",
	}
	ForceFullSyncFlag = cli.Uint64Flag{
		Name:  "forcefullsync",
		Usage: "Force full sync on last blocks",
	}
	ProfileFlag = cli.StringFlag{
		Name:  "profile",
		Usage: "Configuration profile",
	}
	IpfsPortStaticFlag = cli.BoolFlag{
		Name:  "ipfsportstatic",
		Usage: "Enable static ipfs port",
	}
	ApiKeyFlag = cli.StringFlag{
		Name:  "apikey",
		Usage: "Set RPC api key",
	}
	LogFileSizeFlag = cli.IntFlag{
		Name:  "logfilesize",
		Usage: "Set log file size in KB",
		Value: 1024 * 100,
	}
	LogColoring = cli.BoolFlag{
		Name:  "logcoloring",
		Usage: "Use log coloring",
	}
)
