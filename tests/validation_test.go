package tests

import (
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/crypto"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func Test_BigFeeTx(t *testing.T) {
	_, appState, _ := blockchain.NewTestBlockchain(true, nil)

	key, _ := crypto.GenerateKey()
	sender := crypto.PubkeyToAddress(key.PublicKey)
	receiver := GetRandAddr()
	appState.State.SetBalance(sender, big.NewInt(10000))

	tx := &types.Transaction{
		Type:         types.SendTx,
		Amount:       big.NewInt(1000),
		MaxFee:       big.NewInt(190),
		AccountNonce: 1,
		To:           &receiver,
	}

	signedTx, _ := types.SignTx(tx, key) // tx size 97

	appState.State.SetFeePerByte(big.NewInt(2))
	require.Nil(t, validation.ValidateTx(appState, signedTx, true))
	require.Equal(t, validation.BigFee, validation.ValidateTx(appState, signedTx, false))

	appState.State.SetFeePerByte(big.NewInt(1))
	require.Nil(t, validation.ValidateTx(appState, signedTx, true))
	require.Nil(t, validation.ValidateTx(appState, signedTx, false))
}
