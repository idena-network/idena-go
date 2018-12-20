package validation

import (
	"github.com/pkg/errors"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/appstate"
)

var (
	NodeApprovedAlready = errors.New("node is already in validator set")
	InvalidSignature    = errors.New("invalid signature")
	InvalidNonce        = errors.New("invalid Nonce")
	InsufficientFunds   = errors.New("insufficient funds")
	InsufficientInvites = errors.New("insufficient invites")
	RecipientRequired   = errors.New("recipient is required")

	validators map[types.TxType]*validator
)

type validator struct {
	validate func(appState *appstate.AppState, tx *types.Transaction) error
}

func init() {
	validators = make(map[types.TxType]*validator)
	validators[types.SendTx] = &validator{
		validate: validateSendTx,
	}

	validators[types.ApprovingTx] = &validator{
		validate: validateApprovingTx,
	}

	validators[types.SendInviteTx] = &validator{
		validate: validateSendInviteTx,
	}
}

func ValidateTx(appState *appstate.AppState, tx *types.Transaction) error {
	sender, _ := types.Sender(tx)

	if sender == (common.Address{}) {
		return InvalidSignature
	}

	if appState.State.GetNonce(sender) > tx.AccountNonce {
		return InvalidNonce
	}

	validator, ok := validators[tx.Type]
	if !ok {
		return nil
	}
	if err := validator.validate(appState, tx); err != nil {
		return err
	}

	return nil
}

// specific validation for sendTx
func validateSendTx(appState *appstate.AppState, tx *types.Transaction) error {
	sender, _ := types.Sender(tx)

	if tx.To == nil {
		return RecipientRequired
	}

	cost := types.CalculateCost(appState.ValidatorsCache.GetCountOfValidNodes(), tx)
	if appState.State.GetBalance(sender).Cmp(cost) < 0 {
		return InsufficientFunds
	}
	return nil
}

// specific validation for approving tx
func validateApprovingTx(appState *appstate.AppState, tx *types.Transaction) error {
	sender, _ := types.Sender(tx)

	if appState.ValidatorsCache.Contains(sender) {
		return NodeApprovedAlready
	}

	if appState.State.GetInvites(sender) == 0 {
		return InsufficientInvites
	}

	return nil
}

func validateSendInviteTx(appState *appstate.AppState, tx *types.Transaction) error {
	sender, _ := types.Sender(tx)

	if err := validateSendTx(appState, tx); err != nil {
		return err
	}

	if appState.State.GetInvites(sender) == 0 {
		return InsufficientInvites
	}
	return nil
}
