package config

import "gopkg.in/urfave/cli.v1"

const (
	DefaultDataDir      = "datadir"
	DefaultBootNode     = "enode://baeceea089e3e9f5382940ef38168fff4c5155692f5a3e0f618ca586ea73a62f8fc858995471741a50724c651af37db84c14254ad80ecbe8e1f979ce8a9ec7b0@111.90.140.21:40404"
	DefaultPort         = 40404
	DefaultRpcHost      = "localhost"
	DefaultRpcPort      = 9009
	DefaultIpfsDataDir  = "ipfs"
	DefaultIpfsPort     = 4002
	DefaultGodAddress   = "0x4d60dc6a2cba8c3ef1ba5e1eba5c12c54cee6b61"
	DefaultCeremonyTime = int64(1563197400)
	DefaultSwarmKey     = "9ad6f96bb2b02a7308ad87938d6139a974b550cc029ce416641a60c46db2f530"
)

var (
	DefaultIpfsBootstrapNodes = []string{
		"/ip4/111.90.140.21/tcp/4002/ipfs/QmZV7cwSgVTSnMUE2zTK3f5nepCuT5yYnTyMpRgGhxJoTG",
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
)
