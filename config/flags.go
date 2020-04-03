package config

import "github.com/urfave/cli"

const (
	DefaultDataDir          = "datadir"
	DefaultPort             = 40404
	DefaultRpcHost          = "localhost"
	DefaultRpcPort          = 9009
	DefaultIpfsDataDir      = "ipfs"
	DefaultIpfsPort         = 40405
	DefaultGodAddress       = "0x4d60dc6a2cba8c3ef1ba5e1eba5c12c54cee6b61"
	DefaultCeremonyTime     = int64(1567171800)
	DefaultSwarmKey         = "9ad6f96bb2b02a7308ad87938d6139a974b550cc029ce416641a60c46db2f530"
	DefaultForceFullSync    = 100
	DefaultStoreCertRange   = 2000
	DefaultMaxInboundPeers  = 12
	DefaultMaxOutboundPeers = 6
	DefaultBurntTxRange     = 180

	LowPowerMaxInboundPeers  = 6
	LowPowerMaxOutboundPeers = 3
)

var (
	DefaultIpfsBootstrapNodes = []string{
		"/ip4/206.81.23.186/tcp/40403/ipfs/QmTHDLnNMAp6K8txLmJW6EHUbwoHTGhkEUBCp4gAtpNqKY",
		"/ip4/165.227.91.202/tcp/40403/ipfs/QmZ9VnVZsokXEttRYiHbHmCUBSdzSQywjj5wM3Me96XoVD",
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
		Value: 1024 * 10,
	}
	LogColoring = cli.BoolFlag{
		Name:  "logcoloring",
		Usage: "Use log coloring",
	}
)
