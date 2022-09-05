package upgrade

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/stretchr/testify/require"
	db "github.com/tendermint/tm-db"
	"testing"
	"time"
)

func TestUpgrader_CanUpgrade(t *testing.T) {
	db := db.NewMemDB()
	bus := eventbus.New()
	appState, _ := appstate.NewAppState(db, bus)
	require.Nil(t, appState.Initialize(0))
	cfg := &config.Config{
		Consensus: &config.ConsensusConf{
			Version:                         TargetVersion - 1,
			UpgradeIntervalBeforeValidation: time.Minute,
		},
	}
	now := time.Now()
	config.ConsensusVersions[TargetVersion].StartActivationDate = now.Add(-time.Hour).UTC().Unix()
	config.ConsensusVersions[TargetVersion].EndActivationDate = now.Add(time.Hour).UTC().Unix()
	upgrader := NewUpgrader(cfg, appState, db)

	appState.State.SetNextValidationTime(now.Add(cfg.Consensus.UpgradeIntervalBeforeValidation * 2))

	appState.IdentityState.SetValidated(common.Address{0x1}, true)

	appState.IdentityState.SetValidated(common.Address{0x2}, true)
	appState.IdentityState.SetOnline(common.Address{0x2}, true)

	appState.IdentityState.SetValidated(common.Address{0x3}, true)
	appState.IdentityState.SetDiscriminated(common.Address{0x3}, true)

	appState.IdentityState.SetValidated(common.Address{0x4}, true)
	appState.IdentityState.SetOnline(common.Address{0x4}, true)
	appState.IdentityState.SetDiscriminated(common.Address{0x4}, true)

	appState.IdentityState.SetOnline(common.Address{0x5}, true)

	appState.IdentityState.SetValidated(common.Address{0x6}, true)
	appState.IdentityState.SetDelegatee(common.Address{0x6}, common.Address{0x5})

	appState.IdentityState.SetValidated(common.Address{0x7}, true)
	appState.IdentityState.SetDelegatee(common.Address{0x7}, common.Address{0x5})

	appState.IdentityState.SetValidated(common.Address{0x8}, true)
	appState.IdentityState.SetDelegatee(common.Address{0x8}, common.Address{0x5})
	appState.IdentityState.SetDiscriminated(common.Address{0x8}, true)

	appState.IdentityState.SetOnline(common.Address{0x9}, true)
	appState.IdentityState.SetDiscriminated(common.Address{0x9}, true)

	appState.IdentityState.SetValidated(common.Address{0xa}, true)
	appState.IdentityState.SetDelegatee(common.Address{0xa}, common.Address{0x9})

	appState.IdentityState.SetDiscriminated(common.Address{0xb}, true)

	appState.IdentityState.SetValidated(common.Address{0xc}, true)
	appState.IdentityState.SetDelegatee(common.Address{0xc}, common.Address{0xb})

	appState.IdentityState.SetValidated(common.Address{0xd}, true)
	appState.IdentityState.SetDelegatee(common.Address{0xd}, common.Address{0xb})
	appState.IdentityState.SetDiscriminated(common.Address{0xd}, true)

	appState.IdentityState.SetValidated(common.Address{0xe}, true)
	appState.IdentityState.SetOnline(common.Address{0xe}, true)

	appState.IdentityState.SetValidated(common.Address{0xf}, true)
	appState.IdentityState.SetDelegatee(common.Address{0xf}, common.Address{0xe})
	appState.IdentityState.SetDiscriminated(common.Address{0xf}, true)

	appState.IdentityState.SetValidated(common.Address{0x1, 0x1}, true)
	appState.IdentityState.SetDelegatee(common.Address{0x1, 0x1}, common.Address{0xe})
	appState.IdentityState.SetDiscriminated(common.Address{0x1, 0x1}, true)

	appState.IdentityState.SetValidated(common.Address{0x1, 0x2}, true)
	appState.IdentityState.SetOnline(common.Address{0x1, 0x2}, true)
	appState.IdentityState.SetDiscriminated(common.Address{0x1, 0x2}, true)

	appState.IdentityState.SetValidated(common.Address{0x1, 0x3}, true)
	appState.IdentityState.SetDelegatee(common.Address{0x1, 0x3}, common.Address{0x1, 0x2})
	appState.IdentityState.SetDiscriminated(common.Address{0x1, 0x3}, true)

	appState.IdentityState.SetDelegatee(common.Address{0x1, 0x5}, common.Address{0x1, 0x4})

	appState.IdentityState.SetOnline(common.Address{0x1, 0x6}, true)

	appState.IdentityState.SetValidated(common.Address{0x1, 0x7}, true)
	appState.IdentityState.SetDelegatee(common.Address{0x1, 0x7}, common.Address{0x1, 0x6})

	appState.IdentityState.SetValidated(common.Address{0x1, 0x8}, true)
	appState.IdentityState.SetDelegatee(common.Address{0x1, 0x8}, common.Address{0x1, 0x6})

	appState.IdentityState.SetValidated(common.Address{0x1, 0x9}, true)
	appState.IdentityState.SetDelegatee(common.Address{0x1, 0x9}, common.Address{0x1, 0x6})

	appState.Precommit()
	require.Nil(t, appState.CommitAt(1))
	require.Nil(t, appState.Initialize(1))

	require.False(t, upgrader.CanUpgrade())

	upgrader.votes.Add(common.Address{0x1}, uint32(TargetVersion))
	require.False(t, upgrader.CanUpgrade())

	upgrader.votes.Add(common.Address{0x2}, uint32(TargetVersion))
	require.False(t, upgrader.CanUpgrade())

	upgrader.votes.Add(common.Address{0x5}, uint32(TargetVersion))
	require.False(t, upgrader.CanUpgrade())

	upgrader.votes.Add(common.Address{0x9}, uint32(TargetVersion))
	require.False(t, upgrader.CanUpgrade())

	upgrader.votes.Add(common.Address{0x3}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x4}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x6}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x7}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x8}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0xa}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0xb}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0xc}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0xd}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0xf}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x1, 0x1}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x1, 0x2}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x1, 0x3}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x1, 0x4}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x1, 0x5}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x1, 0x7}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x1, 0x8}, uint32(TargetVersion))
	upgrader.votes.Add(common.Address{0x1, 0x9}, uint32(TargetVersion))
	require.False(t, upgrader.CanUpgrade())

	upgrader.votes.Add(common.Address{0xe}, uint32(TargetVersion))
	require.True(t, upgrader.CanUpgrade())

	upgrader.votes.Add(common.Address{0x1, 0x6}, uint32(TargetVersion))
	require.True(t, upgrader.CanUpgrade())

}
