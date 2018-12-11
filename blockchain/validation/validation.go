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
	InsufficientFunds   = errors.New("Insufficient funds")
)

func ValidateTx(appState *appstate.AppState, tx *types.Transaction) error {
	if tx.Type == types.ApprovingTx && !validateApprovingTx(appState, tx) {
		return NodeApprovedAlready
	}

	sender, _ := types.Sender(tx)

	if sender == (common.Address{}) {
		return InvalidSignature
	}

	if appState.State.GetNonce(sender) > tx.AccountNonce {
		return InvalidNonce
	}

	cost := types.CalculateCost(appState.ValidatorsCache.GetCountOfValidNodes(), tx)
	if appState.State.GetBalance(sender).Cmp(cost) < 0 {
		return InsufficientFunds
	}

	return nil
}

func validateApprovingTx(appState *appstate.AppState, tx *types.Transaction) bool {
	sender, _ := types.Sender(tx)

	if appState.ValidatorsCache.Contains(sender) {
		return false
	}
	return true
}
