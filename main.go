package main

import (
	"gopkg.in/urfave/cli.v1"
	"idena-go/config"
	"idena-go/log"
	"idena-go/node"
	"os"
	"runtime"
)

var (
	gitCommit = ""
)

func main() {

	app := cli.NewApp()
	app.Version = "0.0.1"
	if len(gitCommit) > 0 {
		app.Version = app.Version + "-" + gitCommit
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "datadir",
			Value: "datadir",
			Usage: "datadir for blockchain",
		},
		cli.IntFlag{
			Name:  "port",
			Usage: "Network listening port",
			Value: config.DefaultPort,
		},
		cli.StringFlag{
			Name:  "rpcaddr",
			Usage: "RPC listening address",
			Value: config.DefaultRpcHost,
		},
		cli.IntFlag{
			Name:  "rpcport",
			Usage: "RPC listening port",
			Value: config.DefaultRpcPort,
		},
		cli.StringFlag{
			Name:  "bootnode",
			Usage: "Bootstrap node url",
			Value: config.DefaultBootnode,
		},
		cli.BoolFlag{
			Name:  "automine",
			Usage: "Mine blocks alone without peers",
		},
		cli.StringFlag{
			Name:  "ipfsbootnode",
			Usage: "Ipfs bootstrap node (overrides existing)",
		},
		cli.IntFlag{
			Name:  "ipfsport",
			Usage: "Ipfs port",
			Value: config.DefaultIpfsPort,
		},
		cli.BoolFlag{
			Name:  "nodiscovery",
			Usage: "NoDiscovery can be used to disable the peer discovery mechanism.",
		},
		cli.IntFlag{
			Name:  "verbosity",
			Usage: "Log verbosity",
			Value: 3,
		},
		cli.StringFlag{
			Name:  "godaddress",
			Usage: "Idena god address",
		},
		cli.Int64Flag{
			Name:  "ceremonytime",
			Usage: "First ceremony time (unix)",
		},
	}

	app.Action = func(context *cli.Context) error {

		logLvl := log.Lvl(context.Int("verbosity"))
		if runtime.GOOS == "windows" {
			log.Root().SetHandler(log.LvlFilterHandler(logLvl, log.StreamHandler(os.Stdout, log.LogfmtFormat())))
		} else {
			log.Root().SetHandler(log.LvlFilterHandler(logLvl, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
		}

		c := config.GetDefaultConfig(
			context.String("datadir"),
			context.Int("port"),
			context.Bool("automine"),
			context.String("rpcaddr"),
			context.Int("rpcport"),
			context.String("bootnode"),
			context.String("ipfsbootnode"),
			context.Int("ipfsport"),
			context.Bool("nodiscovery"),
			context.String("godaddress"),
			context.Int64("ceremonytime"))

		n, _ := node.NewNode(c)
		n.Start()
		n.WaitForStop()
		return nil
	}

	app.Run(os.Args)
}
