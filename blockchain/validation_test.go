package blockchain

import (
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/crypto"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func Test_ValidateDeleteFlipTx(t *testing.T) {
	chain, appState, _, key := NewTestBlockchain(true, nil)
	defer chain.SecStore().Destroy()
	minFeePerGas := big.NewInt(1)

	buildTx := func(cid []byte) *types.Transaction {
		tx := types.Transaction{
			AccountNonce: 1,
			Type:         types.DeleteFlipTx,
			Payload:      attachments.CreateDeleteFlipAttachment(cid),
			MaxFee:       big.NewInt(700_000_0),
		}
		signedTx, _ := types.SignTx(&tx, key)
		return signedTx
	}

	tx := buildTx(nil)
	err := validation.ValidateTx(appState, tx, minFeePerGas, validation.InBlockTx)
	require.Equal(t, validation.InvalidPayload, err)

	tx = buildTx([]byte{0x1, 0x2, 0x3})
	err = validation.ValidateTx(appState, tx, minFeePerGas, validation.InBlockTx)
	require.Equal(t, validation.FlipIsMissing, err)

	addr := crypto.PubkeyToAddress(key.PublicKey)
	appState.State.AddFlip(addr, []byte{0x1, 0x2, 0x3}, 0)
	err = validation.ValidateTx(appState, tx, minFeePerGas, validation.InBlockTx)
	require.Equal(t, nil, err)
}
