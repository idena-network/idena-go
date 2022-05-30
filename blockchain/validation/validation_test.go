package validation

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/stretchr/testify/require"
	db "github.com/tendermint/tm-db"
	"math/big"
	"testing"
)

func Test_validateDelegateTx(t *testing.T) {
	SetAppConfig(&config.Config{
		Consensus: &config.ConsensusConf{
			EnableUpgrade8: true,
		},
	})

	key, _ := crypto.GenerateKey()
	sender := crypto.PubkeyToAddress(key.PublicKey)
	buildTx := func(to common.Address) *types.Transaction {
		tx := types.Transaction{
			AccountNonce: 1,
			Type:         types.DelegateTx,
			To:           &to,
			MaxFee:       big.NewInt(1000),
		}
		signedTx, _ := types.SignTx(&tx, key)
		return signedTx
	}

	initAppState := func() *appstate.AppState {
		db := db.NewMemDB()
		bus := eventbus.New()
		appState, _ := appstate.NewAppState(db, bus)
		require.NoError(t, appState.Initialize(0))
		return appState
	}

	commitAppState := func(appState *appstate.AppState) {
		appState.Precommit(true)
		require.Nil(t, appState.CommitAt(1))
		require.Nil(t, appState.Initialize(1))
	}

	{
		appState := initAppState()
		tx := buildTx(common.Address{0x1})
		err := validateDelegateTx(appState, tx, InBlockTx)
		require.NoError(t, err)
	}

	{
		appState := initAppState()
		addressWithDelegatee := common.Address{0x1}
		appState.State.SetDelegatee(addressWithDelegatee, common.Address{0x2})
		commitAppState(appState)
		tx := buildTx(addressWithDelegatee)
		err := validateDelegateTx(appState, tx, InBlockTx)
		require.Equal(t, InvalidRecipient, err)
	}

	{
		appState := initAppState()
		addressWithPendingUndelegation := common.Address{0x1}
		appState.State.SetDelegatee(addressWithPendingUndelegation, common.Address{0x2})
		appState.State.SetPendingUndelegation(addressWithPendingUndelegation)
		commitAppState(appState)
		tx := buildTx(addressWithPendingUndelegation)
		err := validateDelegateTx(appState, tx, InBlockTx)
		require.Equal(t, InvalidRecipient, err)
	}

	{
		appState := initAppState()
		appState.State.SetDelegatee(sender, common.Address{0x1})
		commitAppState(appState)
		tx := buildTx(common.Address{0x2})
		err := validateDelegateTx(appState, tx, InBlockTx)
		require.Equal(t, SenderHasDelegatee, err)
	}

	{
		appState := initAppState()
		appState.State.ToggleDelegationAddress(sender, common.Address{0x1})
		commitAppState(appState)
		tx := buildTx(common.Address{0x2})
		err := validateDelegateTx(appState, tx, InBlockTx)
		require.Equal(t, SenderHasDelegatee, err)
	}

	{
		appState := initAppState()
		appState.State.SetDelegatee(sender, common.Address{0x1})
		appState.State.ToggleDelegationAddress(sender, common.EmptyAddress)
		appState.State.ToggleDelegationAddress(sender, common.Address{0x2})
		commitAppState(appState)
		tx := buildTx(common.Address{0x3})
		err := validateDelegateTx(appState, tx, InBlockTx)
		require.Equal(t, SenderHasDelegatee, err)
	}

	{
		appState := initAppState()
		appState.State.SetDelegatee(sender, common.Address{0x1})
		appState.State.ToggleDelegationAddress(sender, common.EmptyAddress)
		commitAppState(appState)

		tx := buildTx(common.Address{0x2})
		err := validateDelegateTx(appState, tx, InBlockTx)
		require.Equal(t, InvalidRecipient, err)

		tx = buildTx(common.Address{0x1})
		err = validateDelegateTx(appState, tx, InBlockTx)
		require.NoError(t, err)
	}

	{
		appState := initAppState()
		appState.State.ToggleDelegationAddress(sender, common.Address{0x1})
		appState.State.ToggleDelegationAddress(sender, common.EmptyAddress)
		commitAppState(appState)
		tx := buildTx(common.Address{0x2})
		err := validateDelegateTx(appState, tx, InBlockTx)
		require.NoError(t, err)
	}

	{
		appState := initAppState()
		appState.State.SetDelegatee(sender, common.Address{0x1})
		appState.State.SetPendingUndelegation(sender)
		commitAppState(appState)

		tx := buildTx(common.Address{0x2})
		err := validateDelegateTx(appState, tx, InBlockTx)
		require.Equal(t, InvalidRecipient, err)

		tx = buildTx(common.Address{0x1})
		err = validateDelegateTx(appState, tx, InBlockTx)
		require.NoError(t, err)
	}
}
