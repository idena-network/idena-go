package main

import (
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	dbm "github.com/tendermint/tm-db"
	"github.com/urfave/cli"
	"os"
	"runtime"
)

func main() {
	app := cli.NewApp()

	app.Flags = []cli.Flag{
		config.DataDirFlag,
		config.VerbosityFlag,
	}

	app.Action = func(context *cli.Context) error {
		logLvl := log.Lvl(context.Int("verbosity"))
		var handler log.Handler
		if runtime.GOOS == "windows" {
			handler = log.LvlFilterHandler(logLvl, log.StreamHandler(os.Stdout, log.LogfmtFormat()))
		} else {
			handler = log.LvlFilterHandler(logLvl, log.StreamHandler(os.Stderr, log.TerminalFormat(true)))
		}
		log.Root().SetHandler(handler)
		if !context.IsSet(config.DataDirFlag.Name) {
			return errors.New("datadir option is required")
		}

		db, err := OpenDatabase(context.String(config.DataDirFlag.Name), "idenachain", 16, 16)
		if err != nil {
			return err
		}
		defer db.Close()
		repo := database.NewRepo(db)
		genesis := repo.ReadIntermediateGenesis()
		if genesis == 0 {
			return errors.New("intermediate genesis is not found")
		}

		genesisBlockHash := repo.ReadCanonicalHash(genesis)

		genesisBlock := repo.ReadBlockHeader(genesisBlockHash)

		appState, err := appstate.NewAppState(db, eventbus.New())
		if err != nil {
			return err
		}
		if err := appState.Initialize(genesis); err != nil {
			return err
		}

		prefix, err := state.StateDbKeys.LoadDbPrefix(db)
		if err != nil {
			return err
		}
		stateDb := dbm.NewPrefixDB(db, prefix)
		prefix, err = state.IdentityStateDbKeys.LoadDbPrefix(db, false)
		if err != nil {
			return err
		}
		identityStateDb := dbm.NewPrefixDB(db, prefix)

		if err := os.Mkdir("bindata", 0777); err != nil {
			return err
		}

		file, err := os.Create("bindata/statedb.tar")
		if err != nil {
			return err
		}

		stateRoot, err := state.WriteTreeTo(stateDb, genesis, file)
		file.Close()
		if err != nil {
			return err
		}

		file, err = os.Create("bindata/identitystatedb.tar")
		if err != nil {
			return err
		}

		identityRoot, err := state.WriteTreeTo(identityStateDb, genesis, file)
		file.Close()
		if err != nil {
			return err
		}

		if genesisBlock.ProposedHeader.Root != stateRoot || genesisBlock.ProposedHeader.IdentityRoot != identityRoot {
			return errors.New("written tree is incompatible with block header")
		}

		file, err = os.Create("bindata/header.tar")
		if err != nil {
			return err
		}

		data, err := genesisBlock.ToBytes()
		if err != nil {
			return err
		}
		if _, err := file.Write(data); err != nil {
			return err
		}
		file.Close()
		log.Info("Genesis block generated", "height", genesis, "hash", genesisBlock.Hash())
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err.Error())
	}
}

func OpenDatabase(datadir string, name string, cache int, handles int) (dbm.DB, error) {
	return dbm.NewGoLevelDBWithOpts(name, datadir, &opt.Options{
		OpenFilesCacheCapacity: handles,
		BlockCacheCapacity:     cache / 2 * opt.MiB,
		WriteBuffer:            cache / 4 * opt.MiB,
		Filter:                 filter.NewBloomFilter(10),
	})
}
