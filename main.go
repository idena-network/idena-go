package main

import (
	"fmt"
	"gopkg.in/urfave/cli.v1"
	"idena-go/config"
	"idena-go/log"
	"idena-go/node"
	"idena-go/p2p"
	"idena-go/p2p/enode"
	"idena-go/p2p/nat"
	"os"
	"time"
)

func main() {

	app := cli.NewApp()

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "datadir",
			Value: "datadir",
			Usage: "datadir for blockchain",
		},
		cli.IntFlag{
			Name:  "port",
			Usage: "Network listening port",
			Value: 40404,
		},
		cli.StringFlag{
			Name:  "bootnode",
			Usage: "Bootstrap node url",
			Value: "enode://02f9f7c4d48fbaeb721d39129ac2480638ac8b633716a53f4cbc3cefe140bb41c34b599efa776e8baab7abed51aae0d8643eeac9ffc8c676194d3183169a3e0a@127.0.0.1:40405",
		},
		cli.BoolFlag{
			Name:  "automine",
			Usage: "Mine blocks alone without peers",
		},
	}

	app.Action = func(context *cli.Context) error {
		log.Root().SetHandler(log.LvlFilterHandler(log.LvlDebug, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
		log := log.New()
		nodes := []*enode.Node{}
		nodeParam := context.String("bootnode")
		if nodeParam != "" {
			p, err := enode.ParseV4(nodeParam)
			if err == nil {
				nodes = append(nodes, p)
			} else {
				log.Warn("Cant parse bootstrap node")
			}
		}

		c := config.Config{
			DataDir: context.String("datadir"),
			P2P: &p2p.Config{
				ListenAddr:     fmt.Sprintf(":%d", context.Int("port")),
				MaxPeers:       25,
				NAT:            nat.Any(),
				BootstrapNodes: nodes,
			},
			Consensus: getDefaultConsensusConfig(context.Bool("automine")),
		}

		n, _ := node.NewNode(&c)
		n.Start()
		n.Wait()
		return nil
	}

	app.Run(os.Args)
}

func getDefaultConsensusConfig(automine bool) *config.ConsensusConf {
	return &config.ConsensusConf{
		MaxSteps:                       150,
		CommitteePercent:               0.3,  // 30% of valid nodes will be committee members
		FinalCommitteeConsensusPercent: 0.7,  // 70% of valid nodes will be committee members
		ThesholdBa:                     0.65, // 65% of committee members should vote for block
		ProposerTheshold:               0.5,
		WaitBlockDelay:                 time.Minute,
		WaitSortitionProofDelay:        time.Second * 5,
		EstimatedBaVariance:            time.Second * 5,
		WaitForStepDelay:               time.Second * 20,
		Automine:                       false,
	}
}
