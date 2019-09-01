package main

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
	"os"
	"runtime"

	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/tendermint/tm-db"
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

		head := repo.ReadHead()
		if head == nil {
			return errors.New("head is not found")
		}
		appState := appstate.NewAppState(db, eventbus.New())
		appState.Initialize(head.Height())

		snapshot := state.PredefinedState{}

		appState.State.IterateAccounts(func(key []byte, value []byte) bool {
			if key == nil {
				return true
			}
			addr := common.Address{}
			addr.SetBytes(key[1:])
			var data state.Account
			if err := rlp.DecodeBytes(value, &data); err != nil {
				log.Error(err.Error())
				return false
			}

			snapshot.Accounts = append(snapshot.Accounts, &state.StateAccount{
				Address: addr,
				Balance: data.Balance,
			})
			return false
		})

		appState.State.IterateIdentities(func(key []byte, value []byte) bool {
			if key == nil {
				return true
			}
			addr := common.Address{}
			addr.SetBytes(key[1:])

			var data state.Identity
			if err := rlp.DecodeBytes(value, &data); err != nil {
				log.Error(err.Error())
				return false
			}
			snapshot.Identities = append(snapshot.Identities, &state.StateIdentity{
				Address:         addr,
				State:           data.State,
				Birthday:        data.Birthday,
				Code:            data.Code,
				Generation:      data.Generation,
				Invites:         data.Invites,
				Nickname:        data.Nickname,
				PubKey:          data.PubKey,
				QualifiedFlips:  data.QualifiedFlips,
				RequiredFlips:   data.RequiredFlips,
				ShortFlipPoints: data.ShortFlipPoints,
				Stake:           data.Stake,
			})
			return false
		})

		appState.IdentityState.IterateIdentities(func(key []byte, value []byte) bool {
			if key == nil {
				return true
			}
			addr := common.Address{}
			addr.SetBytes(key[1:])

			var data state.ApprovedIdentity
			if err := rlp.DecodeBytes(value, &data); err != nil {
				log.Error(err.Error())
				return false
			}
			snapshot.ApprovedIdentities = append(snapshot.ApprovedIdentities, &state.StateApprovedIdentity{
				Address:  addr,
				Approved: data.Approved,
				Online:   data.Online,
			})
			return false
		})
		file, err := os.Create("stategen.out")
		if err != nil {
			return err
		}

		if err := rlp.Encode(file, snapshot); err != nil {
			return err
		}
		file.Close()

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
