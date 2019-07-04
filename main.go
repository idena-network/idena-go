package main

import (
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/node"
	"gopkg.in/urfave/cli.v1"
	"os"
	"runtime"
)

var (
	version = "0.0.1"
)

func main() {

	app := cli.NewApp()
	app.Version = version

	app.Flags = []cli.Flag{
		config.CfgFileFlag,
		config.DataDirFlag,
		config.TcpPortFlag,
		config.RpcHostFlag,
		config.RpcPortFlag,
		config.BootNodeFlag,
		config.AutomineFlag,
		config.IpfsBootNodeFlag,
		config.IpfsPortFlag,
		config.NoDiscoveryFlag,
		config.VerbosityFlag,
		config.GodAddressFlag,
		config.CeremonyTimeFlag,
		config.MaxNetworkDelayFlag,
	}

	app.Action = func(context *cli.Context) error {

		logLvl := log.Lvl(context.Int("verbosity"))
		if runtime.GOOS == "windows" {
			log.Root().SetHandler(log.LvlFilterHandler(logLvl, log.StreamHandler(os.Stdout, log.LogfmtFormat())))
		} else {
			log.Root().SetHandler(log.LvlFilterHandler(logLvl, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
		}

		cfg := config.MakeConfig(context)

		n, _ := node.NewNode(cfg)
		n.Start()
		n.WaitForStop()
		return nil
	}

	app.Run(os.Args)
}
