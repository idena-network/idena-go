package appstate

import (
	db2 "github.com/tendermint/tendermint/libs/db"
	"gx/ipfs/QmPVkJMTeRC6iBByPWdrRkD3BE5UXsj5HPzb4kPqL186mS/testify/require"
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
	forCheck := appState.ForCheckWithReload(1)

	forCheck.State.SetNonce(addr, 1)
	forCheck.State.SetNonce(addr2, 3)
	forCheck.IdentityState.Remove(addr)

	forCheck.Commit()

	appState = NewAppState(db, bus)

	appState.Initialize(2)

	require.Equal(t, stateHash, appState.State.Root())
	require.Equal(t, identityHash, appState.IdentityState.Root())
}
