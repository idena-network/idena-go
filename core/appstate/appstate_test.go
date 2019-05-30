package appstate

import (
	"github.com/stretchr/testify/require"
	db2 "github.com/tendermint/tendermint/libs/db"
	"idena-go/common"
	"idena-go/common/eventbus"
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

	appState.Commit()

	appState.State.SetNonce(addr, 2)
	appState.State.SetNonce(addr2, 1)
	appState.IdentityState.Add(addr2)

	appState.Commit()

	stateHash := appState.State.Root()
	identityHash := appState.IdentityState.Root()
	forCheck, _ := appState.ForCheckWithNewCache(1)

	forCheck.State.SetNonce(addr, 1)
	forCheck.State.SetNonce(addr2, 3)
	forCheck.IdentityState.Remove(addr)

	err := forCheck.Commit()
	require.Nil(t, err)

	appState = NewAppState(db, bus)

	appState.Initialize(2)

	require.Equal(t, stateHash, appState.State.Root())
	require.Equal(t, identityHash, appState.IdentityState.Root())
}
