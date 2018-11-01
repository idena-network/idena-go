package main

import (
	"fmt"
	"gopkg.in/urfave/cli.v1"
	"idena-go/config"
	"idena-go/log"
	"idena-go/node"
	"idena-go/p2p"
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
	}

	app.Action = func(context *cli.Context) error {
		log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

		c := config.Config{
			DataDir: context.String("datadir"),
			P2P: &p2p.Config{
				ListenAddr: fmt.Sprintf(":%d", context.Int("port")),
				MaxPeers:   25,
				NAT:        nat.Any(),
			},
			Consensus: getDefaultConsensusConfig(),
		}

		n, _ := node.NewNode(&c)
		n.Start()
		n.Wait()
		return nil
	}

	app.Run(os.Args)
}

func getDefaultConsensusConfig() *config.ConsensusConf {
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
	}
}
