package main

import (
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
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
		appState, err := appstate.NewAppState(db, eventbus.New())
		if err != nil {
			return err
		}
		appState.Initialize(head.Height())

		snapshot := &models.ProtoPredefinedState{
			Block: head.Height(),
			Seed:  head.Seed().Bytes(),
		}

		globalObject := appState.State.GetOrNewGlobalObject()

		snapshot.Global = &models.ProtoPredefinedState_Global{
			LastSnapshot:                  globalObject.LastSnapshot(),
			NextValidationTime:            globalObject.NextValidationTime(),
			GodAddress:                    globalObject.GodAddress().Bytes(),
			WordsSeed:                     globalObject.FlipWordsSeed().Bytes(),
			ValidationPeriod:              uint32(globalObject.ValidationPeriod()),
			Epoch:                         uint32(globalObject.Epoch()),
			EpochBlock:                    globalObject.EpochBlock(),
			PrevEpochBlocks:               globalObject.PrevEpochBlocks(),
			FeePerGas:                     common.BigIntBytesOrNil(globalObject.FeePerGas()),
			VrfProposerThreshold:          globalObject.VrfProposerThresholdRaw(),
			EmptyBlocksBits:               common.BigIntBytesOrNil(globalObject.EmptyBlocksBits()),
			GodAddressInvites:             uint32(globalObject.GodAddressInvites()),
			BlocksCntWithoutCeremonialTxs: uint32(globalObject.BlocksCntWithoutCeremonialTxs()),
		}

		snapshot.StatusSwitch = &models.ProtoPredefinedState_StatusSwitch{
			Addresses: nil,
		}

		appState.State.IterateAccounts(func(key []byte, value []byte) bool {
			if key == nil {
				return true
			}
			addr := common.Address{}
			addr.SetBytes(key[1:])
			var data state.Account
			if err := data.FromBytes(value); err != nil {
				log.Error(err.Error())
				return false
			}
			acc := &models.ProtoPredefinedState_Account{
				Address: addr.Bytes(),
				Balance: common.BigIntBytesOrNil(data.Balance),
				Epoch:   uint32(data.Epoch),
				Nonce:   data.Nonce,
			}
			if data.Contract != nil {
				acc.ContractData = &models.ProtoPredefinedState_Account_ContractData{
					CodeHash: data.Contract.CodeHash.Bytes(),
					Stake:    data.Contract.Stake.Bytes(),
				}
			}
			snapshot.Accounts = append(snapshot.Accounts, acc)
			return false
		})

		appState.State.IterateIdentities(func(key []byte, value []byte) bool {
			if key == nil {
				return true
			}
			addr := common.Address{}
			addr.SetBytes(key[1:])

			var data state.Identity
			if err := data.FromBytes(value); err != nil {
				log.Error(err.Error())
				return false
			}

			var flips []*models.ProtoPredefinedState_Identity_Flip
			for _, f := range data.Flips {
				flips = append(flips, &models.ProtoPredefinedState_Identity_Flip{
					Cid:  f.Cid,
					Pair: uint32(f.Pair),
				})
			}

			identity := &models.ProtoPredefinedState_Identity{
				Address:          addr.Bytes(),
				State:            uint32(data.State),
				Birthday:         uint32(data.Birthday),
				Code:             data.Code,
				Generation:       data.Generation,
				Invites:          uint32(data.Invites),
				ProfileHash:      data.ProfileHash,
				PubKey:           data.PubKey,
				QualifiedFlips:   data.QualifiedFlips,
				RequiredFlips:    uint32(data.RequiredFlips),
				ShortFlipPoints:  data.ShortFlipPoints,
				Stake:            common.BigIntBytesOrNil(data.Stake),
				Flips:            flips,
				Penalty:          common.BigIntBytesOrNil(data.Penalty),
				ValidationBits:   uint32(data.ValidationTxsBits),
				ValidationStatus: uint32(data.LastValidationStatus),
				Scores:           data.Scores,
				DelegationNonce:  data.DelegationNonce,
				DelegationEpoch:  uint32(data.DelegationEpoch),
			}

			if data.Inviter != nil {
				identity.Inviter = &models.ProtoPredefinedState_Identity_Inviter{
					Hash:        data.Inviter.TxHash[:],
					Address:     data.Inviter.Address[:],
					EpochHeight: data.Inviter.EpochHeight,
				}
			}
			if data.Delegatee != nil {
				identity.Delegatee = data.Delegatee.Bytes()
			}
			for idx := range data.Invitees {
				identity.Invitees = append(identity.Invitees, &models.ProtoPredefinedState_Identity_TxAddr{
					Hash:    data.Invitees[idx].TxHash[:],
					Address: data.Invitees[idx].Address[:],
				})
			}

			snapshot.Identities = append(snapshot.Identities, identity)
			return false
		})

		appState.State.IterateContractValues(func(key []byte, value []byte) bool {
			snapshot.ContractValues = append(snapshot.ContractValues, &models.ProtoPredefinedState_ContractKeyValue{
				Key:   key,
				Value: value,
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
			if err := data.FromBytes(value); err != nil {
				log.Error(err.Error())
				return false
			}
			identity := &models.ProtoPredefinedState_ApprovedIdentity{
				Address:  addr[:],
				Approved: data.Approved,
				Online:   false,
			}
			if data.Delegatee != nil {
				identity.Delegatee = data.Delegatee.Bytes()
			}
			snapshot.ApprovedIdentities = append(snapshot.ApprovedIdentities, identity)
			return false
		})

		data, err := proto.Marshal(snapshot)
		if err != nil {
			return err
		}

		file, err := os.Create("stategen.out")
		if err != nil {
			return err
		}

		_, err = file.Write(data)
		if err != nil {
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
