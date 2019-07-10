package main

import (
	"github.com/coreos/go-semver/semver"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/node"
	"gopkg.in/urfave/cli.v1"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
)

const (
	VersionFile = "version"
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

		cfg, err := config.MakeConfig(context)

		if err != nil {
			return err
		}

		err = dropOldDirOnFork(cfg)
		if err != nil {
			return err
		}

		n, _ := node.NewNode(cfg)
		n.Start()
		n.WaitForStop()
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err.Error())
	}
}

func dropOldDirOnFork(cfg *config.Config) error {
	path := filepath.Join(cfg.DataDir, VersionFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return writeVersion(cfg)
	}
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	current := semver.New(version)
	old := semver.New(string(b))

	if old.Major < current.Major || old.Minor < current.Minor {
		log.Info("Network fork, removing db folder...")
		err = os.RemoveAll(filepath.Join(cfg.DataDir, "idenachain.db"))
		if err != nil {
			return err
		}
	}

	if old.LessThan(*current) {
		return writeVersion(cfg)
	}
	return nil
}

func writeVersion(cfg *config.Config) error {
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(cfg.DataDir, VersionFile), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(version)
	return err
}
