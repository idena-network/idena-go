package appstate

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/stretchr/testify/require"
	db2 "github.com/tendermint/tm-db"
	"testing"
)

func TestAppState_ForCheckWithReload(t *testing.T) {
	db := db2.NewMemDB()
	bus := eventbus.New()

	appState, _ := NewAppState(db, bus)

	addr := common.Address{}
	addr2 := common.Address{}
	addr2.SetBytes([]byte{0x1})

	appState.State.SetNonce(addr, 1)
	appState.IdentityState.SetValidated(addr, true)

	appState.Commit(nil, true)

	appState.State.SetNonce(addr, 2)
	appState.State.SetNonce(addr2, 1)
	appState.IdentityState.SetValidated(addr2, true)

	appState.Commit(nil, true)

	stateHash := appState.State.Root()
	identityHash := appState.IdentityState.Root()
	forCheck, _ := appState.ForCheckWithOverwrite(1)

	forCheck.State.SetNonce(addr, 1)
	forCheck.State.SetNonce(addr2, 3)
	forCheck.IdentityState.Remove(addr)

	err := forCheck.Commit(nil, true)
	require.Nil(t, err)

	appState, _ = NewAppState(db, bus)

	appState.Initialize(2)

	require.Equal(t, stateHash, appState.State.Root())
	require.Equal(t, identityHash, appState.IdentityState.Root())
}

func TestAppState_migrateToUpgrade8(t *testing.T) {
	db := db2.NewMemDB()
	bus := eventbus.New()
	appState, _ := NewAppState(db, bus)

	appState.IdentityState.SetValidated(common.Address{0x1}, true)
	appState.IdentityState.SetOnline(common.Address{0x1}, true)
	appState.IdentityState.SetDiscriminated(common.Address{0x1}, true)
	appState.IdentityState.SetDelegatee(common.Address{0x1}, common.Address{0x2})

	require.NoError(t, appState.Commit(nil, false))
	require.NoError(t, appState.Initialize(1))

	require.True(t, appState.IdentityState.IsOnline(common.Address{0x1}))
	require.True(t, appState.IdentityState.IsValidated(common.Address{0x1}))
	require.False(t, appState.ValidatorsCache.IsDiscriminated(common.Address{0x1}))
	require.Equal(t, common.Address{0x2}, *appState.IdentityState.Delegatee(common.Address{0x1}))

	appState.IdentityState.SetDiscriminated(common.Address{0x1}, true)
	require.NoError(t, appState.Commit(nil, true))
	require.NoError(t, appState.Initialize(2))

	require.True(t, appState.IdentityState.IsOnline(common.Address{0x1}))
	require.True(t, appState.IdentityState.IsValidated(common.Address{0x1}))
	require.True(t, appState.ValidatorsCache.IsDiscriminated(common.Address{0x1}))
	require.Equal(t, common.Address{0x2}, *appState.IdentityState.Delegatee(common.Address{0x1}))
}
