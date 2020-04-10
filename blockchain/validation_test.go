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
	_, appState, _, key := NewTestBlockchain(true, nil)
	minFeePerByte := big.NewInt(1)

	buildTx := func(cid []byte) *types.Transaction {
		tx := types.Transaction{
			AccountNonce: 1,
			Type:         types.DeleteFlipTx,
			Payload:      attachments.CreateDeleteFlipAttachment(cid),
			MaxFee:       big.NewInt(700_000),
		}
		signedTx, _ := types.SignTx(&tx, key)
		return signedTx
	}

	tx := buildTx(nil)
	err := validation.ValidateTx(appState, tx, minFeePerByte, validation.InBlockTx)
	require.Equal(t, validation.FlipIsMissing, err)

	tx = buildTx([]byte{0x1, 0x2, 0x3})
	err = validation.ValidateTx(appState, tx, minFeePerByte, validation.InBlockTx)
	require.Equal(t, validation.FlipIsMissing, err)

	addr := crypto.PubkeyToAddress(key.PublicKey)
	appState.State.AddFlip(addr, []byte{0x1, 0x2, 0x3}, 0)
	err = validation.ValidateTx(appState, tx, minFeePerByte, validation.InBlockTx)
	require.Equal(t, nil, err)
}
