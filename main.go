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
	"idena-go/rpc"
	"math/big"
	"os"
	"runtime"
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
		cli.IntFlag{
			Name:  "rpcport",
			Usage: "RPC listening port",
			Value: 9009,
		},
		cli.StringFlag{
			Name:  "bootnode",
			Usage: "Bootstrap node url",
			Value: "enode://86270bc08b3e4580668aa0649fff9ed1e36f8eb5cdcde5df494a2bc07a952458e96859d8a65d4e3f4a7061370d2037b5e78b6b0916c2e9bec1fba120437fab24@127.0.0.1:40405",
		},
		cli.BoolFlag{
			Name:  "automine",
			Usage: "Mine blocks alone without peers",
		},
	}

	app.Action = func(context *cli.Context) error {

		if runtime.GOOS == "windows" {
			log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stdout, log.LogfmtFormat())))
		} else {
			log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
		}
		log := log.New()
		var nodes []*enode.Node
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
			RPC:       getDefaultRPCConfig(context.Int("rpcport")),
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
		Automine:                       automine,
		BlockReward:                    big.NewInt(0).Mul(big.NewInt(1e+18), big.NewInt(15)),
		StakeRewardRate:                0.2,
		FeeBurnRate:                    0.9,
		FinalCommitteeReward:           big.NewInt(6e+18),
	}
}

func getDefaultRPCConfig(rpcPort int) *rpc.Config {
	// DefaultConfig contains reasonable default settings.
	return &rpc.Config{
		HTTPCors:         []string{"*"},
		HTTPHost:         "localhost",
		HTTPPort:         rpcPort,
		HTTPModules:      []string{"net", "dna"},
		HTTPVirtualHosts: []string{"localhost"},
		HTTPTimeouts:     rpc.DefaultHTTPTimeouts,
	}
}
