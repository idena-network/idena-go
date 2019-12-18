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
	"github.com/jteeuwen/go-bindata"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
	"os"
	"path/filepath"
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
		snapshot.Block = head.Height()
		snapshot.Seed = head.Seed()

		globalObject := appState.State.GetOrNewGlobalObject()

		snapshot.Global = state.StateGlobal{
			LastSnapshot:         globalObject.LastSnapshot(),
			NextValidationTime:   globalObject.NextValidationTime(),
			GodAddress:           globalObject.GodAddress(),
			WordsSeed:            globalObject.FlipWordsSeed(),
			ValidationPeriod:     globalObject.ValidationPeriod(),
			Epoch:                globalObject.Epoch(),
			EpochBlock:           globalObject.EpochBlock(),
			FeePerByte:           globalObject.FeePerByte(),
			VrfProposerThreshold: globalObject.VrfProposerThresholdRaw(),
			EmptyBlocksBits:      globalObject.EmptyBlocksBits(),
		}

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
				Epoch:   data.Epoch,
				Nonce:   data.Nonce,
			})
			return false
		})

		// TODO delete the variable after fork-0.15.0 released
		inviteesToRemovePerInviter := make(map[common.Address][]common.Address)
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

			var flips []state.StateIdentityFlip
			for _, f := range data.Flips {
				flips = append(flips, state.StateIdentityFlip{
					Cid:  f.Cid,
					Pair: f.Pair,
				})
			}

			var inviter *state.TxAddr
			// TODO uncomment the line and delete next if-operator after fork-0.15.0 released
			//inviter = data.Inviter
			if data.Inviter != nil {
				if data.State == state.Invite || data.State == state.Candidate {
					inviter = data.Inviter
				} else {
					inviteesToRemovePerInviter[data.Inviter.Address] =
						append(inviteesToRemovePerInviter[data.Inviter.Address], addr)
				}
			}

			snapshot.Identities = append(snapshot.Identities, &state.StateIdentity{
				Address:         addr,
				State:           data.State,
				Birthday:        data.Birthday,
				Code:            data.Code,
				Generation:      data.Generation,
				Invites:         data.Invites,
				ProfileHash:     data.ProfileHash,
				PubKey:          data.PubKey,
				QualifiedFlips:  data.QualifiedFlips,
				RequiredFlips:   data.RequiredFlips,
				ShortFlipPoints: data.ShortFlipPoints,
				Stake:           data.Stake,
				Flips:           flips,
				Invitees:        data.Invitees,
				Inviter:         inviter,
				Penalty:         data.Penalty,
			})
			return false
		})

		// TODO delete the if-operator after fork-0.15.0 released
		if len(inviteesToRemovePerInviter) > 0 {
			for _, identity := range snapshot.Identities {
				if inviteeAddrs, ok := inviteesToRemovePerInviter[identity.Address]; ok {
					for _, inviteeAddr := range inviteeAddrs {
						for i, invitee := range identity.Invitees {
							if invitee.Address != inviteeAddr {
								continue
							}
							identity.Invitees = append(identity.Invitees[:i], identity.Invitees[i+1:]...)
							break
						}
					}
				}
			}
		}

		// TODO delete this call after fork-0.15.0 released
		validateInvites(snapshot.Identities, appState)

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
				Online:   false,
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

		err = bindata.Translate(&bindata.Config{
			Input: []bindata.InputConfig{{
				Path:      filepath.Clean("stategen.out"),
				Recursive: false,
			}},
			Package: "blockchain",
			Output:  "bindata.go",
		})

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

// TODO delete the func after fork-0.15.0 released
func validateInvites(identities []*state.StateIdentity, appState *appstate.AppState) {
	for _, identity := range identities {
		if len(identity.Invitees) == 0 {
			continue
		}
		for _, invitee := range identity.Invitees {
			if appState.State.GetIdentityState(invitee.Address) == state.Invite {
				continue
			}
			if appState.State.GetIdentityState(invitee.Address) == state.Candidate {
				continue
			}
			panic("There is a link between inviter and validated invitee")
		}
	}
}
