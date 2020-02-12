package main

import (
	"github.com/coreos/go-semver/semver"
	"github.com/idena-network/idena-go/common/pncw"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/node"
	"github.com/mitchellh/panicwrap"
	"gopkg.in/urfave/cli.v1"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

const (
	VersionFile = "version"
	LogDir      = "logs"
	ChainDir    = "idenachain.db"
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
		config.FastSyncFlag,
		config.ForceFullSyncFlag,
		config.ProfileFlag,
		config.IpfsPortStaticFlag,
		config.ApiKeyFlag,
		config.LogFileSizeFlag,
	}

	app.Action = func(context *cli.Context) error {
		logLvl := log.Lvl(context.Int(config.VerbosityFlag.Name))
		logFileSize := context.Int(config.LogFileSizeFlag.Name)

		var handler log.Handler
		if runtime.GOOS == "windows" {
			handler = log.LvlFilterHandler(logLvl, log.StreamHandler(os.Stdout, log.LogfmtFormat()))
		} else {
			handler = log.LvlFilterHandler(logLvl, log.StreamHandler(os.Stderr, log.TerminalFormat(true)))
		}

		log.Root().SetHandler(handler)

		cfg, err := config.MakeConfig(context)

		if err != nil {
			return err
		}

		err = dropOldDirOnFork(cfg)
		if err != nil {
			return err
		}

		fileHandler, err := getLogFileHandler(cfg, logFileSize)

		if err != nil {
			return err
		}

		log.Root().SetHandler(log.LvlFilterHandler(logLvl, log.MultiHandler(handler, fileHandler)))

		err = registerPanicWrapper(cfg)
		if err != nil {
			return err
		}

		log.Info("Idena node is starting", "version", version)

		n, err := node.NewNode(cfg, version)
		if err != nil {
			return err
		}
		n.Start()
		n.WaitForStop()
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err.Error())
	}
}

func registerPanicWrapper(cfg *config.Config) error {
	if !pncw.Enabled {
		return nil
	}
	panicHandler, err := getPanicHandler(cfg)
	if err != nil {
		return err
	}
	exitStatus, err := panicwrap.Wrap(&panicwrap.WrapConfig{
		Handler: panicHandler,

		ForwardSignals: []os.Signal{
			syscall.SIGTRAP,
			syscall.SIGILL,
			syscall.SIGHUP,
			syscall.SIGQUIT,
			syscall.SIGABRT,
			syscall.SIGBUS,
			syscall.SIGFPE,
			syscall.SIGKILL,
			syscall.SIGSEGV,
			syscall.SIGPIPE,
			syscall.SIGALRM,
			syscall.SIGTERM,
			syscall.SIGINT,
		},
	})
	if err != nil {
		panic(err)
	}

	// If exitStatus >= 0, then we're the parent process and the panicwrap
	// re-executed ourselves and completed. Just exit with the proper status.
	if exitStatus >= 0 {
		os.Exit(exitStatus)
	}
	return nil
}

func getPanicHandler(cfg *config.Config) (func(string), error) {
	path := filepath.Join(cfg.DataDir, LogDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(filepath.Join(path, "panics.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return func(s string) {
		f.WriteString(s)
		f.Close()
		os.Exit(1)
	}, nil
}

func getLogFileHandler(cfg *config.Config, logFileSize int) (log.Handler, error) {
	path := filepath.Join(cfg.DataDir, LogDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, err
		}
	}

	fileHandler, _ := log.RotatingFileHandler(filepath.Join(path, "output.log"), uint(logFileSize*1024), log.TerminalFormat(false))

	return fileHandler, nil
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
		log.Info("Network fork, removing db and logs folder...")
		err = os.RemoveAll(filepath.Join(cfg.DataDir, ChainDir))
		if err != nil {
			return err
		}
		err = os.RemoveAll(filepath.Join(cfg.DataDir, LogDir))
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
