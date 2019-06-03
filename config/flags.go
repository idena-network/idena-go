package config

import "gopkg.in/urfave/cli.v1"

const (
	DefaultDataDir      = "datadir"
	DefaultBootNode     = "enode://45df5a0a220198ca95e2ee70dea500569e6578eafa603322c028e1e0ba9ca9e40e5810fa3308728d8d8fda565de8e66a4aa5536950128af8ad93de5ac65e8131@127.0.0.1:40404"
	DefaultPort         = 40404
	DefaultRpcHost      = "localhost"
	DefaultRpcPort      = 9009
	DefaultIpfsDataDir  = "ipfs"
	DefaultIpfsPort     = 4002
	DefaultGodAddress   = "0xf228fa1e9236343c7d44283b5ffcf9ba50df37e8"
	DefaultCeremonyTime = int64(1559225400)
	DefaultSwarmKey     = "9ad6f96bb2b02a7308ad87938d6139a974b550cc029ce416641a60c46db2f530"
)

var (
	DefaultIpfsBootstrapNodes = []string{
		"/ip4/127.0.0.1/tcp/4002/ipfs/QmcGxQehQ8YG65FNghjEhjPTc4p6MbYT5REJJ48rC4amoC",
	}

	ConfigFileFlag = cli.StringFlag{
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
