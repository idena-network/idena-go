package validation

import (
	"github.com/pkg/errors"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/appstate"
)

var (
	NodeApprovedAlready = errors.New("Node is in validator set")
	InvalidSignature    = errors.New("Invalid signature")
	InvalidNonce        = errors.New("Invalid Nonce")
)

func ValidateTx(appState *appstate.AppState, tx *types.Transaction) error {
	if tx.Type == types.ApprovingTx && !validateApprovingTx(appState, tx) {
		return NodeApprovedAlready
	}
	
	if tx.Sender() == (common.Address{}) {
		return InvalidSignature
	}

	if appState.State.GetNonce(tx.Sender()) > tx.AccountNonce {
		return InvalidNonce
	}
	return nil
}

func validateApprovingTx(appState *appstate.AppState, tx *types.Transaction) bool {
	if appState.ValidatorsState.Contains(tx.Sender()) {
		return false
	}
	return true
}
