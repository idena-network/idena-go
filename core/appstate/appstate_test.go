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

	appState := NewAppState(db, bus)

	addr := common.Address{}
	addr2 := common.Address{}
	addr2.SetBytes([]byte{0x1})

	appState.State.SetNonce(addr, 1)
	appState.IdentityState.Add(addr)

	appState.Commit(nil)

	appState.State.SetNonce(addr, 2)
	appState.State.SetNonce(addr2, 1)
	appState.IdentityState.Add(addr2)

	appState.Commit(nil)

	stateHash := appState.State.Root()
	identityHash := appState.IdentityState.Root()
	forCheck, _ := appState.ForCheckWithOverwrite(1)

	forCheck.State.SetNonce(addr, 1)
	forCheck.State.SetNonce(addr2, 3)
	forCheck.IdentityState.Remove(addr)

	err := forCheck.Commit(nil)
	require.Nil(t, err)

	appState = NewAppState(db, bus)

	appState.Initialize(2)

	require.Equal(t, stateHash, appState.State.Root())
	require.Equal(t, identityHash, appState.IdentityState.Root())
}

func TestAppState_UseSyncTree(t *testing.T) {
	db := db2.NewMemDB()
	bus := eventbus.New()

	appState := NewAppState(db, bus)
	appState.Commit(nil)
	require.True(t, appState.defaultTree)

	require.NoError(t, appState.UseSyncTree())
	require.False(t, appState.defaultTree)

	require.NoError(t, appState.UseDefaultTree())
	require.True(t, appState.defaultTree)
}
