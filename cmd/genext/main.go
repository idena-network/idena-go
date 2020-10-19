package genext

import (
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	db "github.com/tendermint/tm-db"
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
		repo := database.NewRepo(db)
		genesis := repo.ReadIntermediateGenesis()
		if genesis == 0 {
			return errors.New("intermediate genesis is not found")
		}



		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err.Error())
	}
}

func OpenDatabase(datadir string, name string, cache int, handles int) (db.DB, error) {
	return db.NewGoLevelDBWithOpts(name, datadir, &opt.Options{
		OpenFilesCacheCapacity: handles,
		BlockCacheCapacity:     cache / 2 * opt.MiB,
		WriteBuffer:            cache / 4 * opt.MiB,
		Filter:                 filter.NewBloomFilter(10),
	})
}
