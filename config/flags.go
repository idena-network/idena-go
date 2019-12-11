package config

import "gopkg.in/urfave/cli.v1"

const (
	DefaultDataDir        = "datadir"
	DefaultPort           = 40404
	DefaultRpcHost        = "localhost"
	DefaultRpcPort        = 9009
	DefaultIpfsDataDir    = "ipfs"
	DefaultIpfsPort       = 40405
	DefaultGodAddress     = "0x4d60dc6a2cba8c3ef1ba5e1eba5c12c54cee6b61"
	DefaultCeremonyTime   = int64(1567171800)
	DefaultSwarmKey       = "9ad6f96bb2b02a7308ad87938d6139a974b550cc029ce416641a60c46db2f530"
	DefaultForceFullSync  = 100
	DefaultStoreCertRange = 2000
	DefaultMaxPeers       = 25
	DefaultBurntTxRange   = 180

	LowPowerMaxPeers = 8
)

var (
	DefaultBootstrapNodes = []string{
		"enode://56b1f1fb9ebf3c83d049083b6ceba87a37036176c250beabec6f93f746ff4e70cd7470e0c9431ea793dea9d1f5ad25039c94a6600ff609e85b5713f6163ab326@206.81.23.186:40404",
		"enode://cec6377ad9bbf60794e7c38f673312c22b4baf831603f17e8a2fe40bb81824b281d20c39f3cb948ec908be6eb1c662366dfd538941382d2d50f29d84bc089b69@165.227.91.202:40404",
	}
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
)
