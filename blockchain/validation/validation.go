package validation

import (
	"github.com/pkg/errors"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/appstate"
	"idena-go/core/state"
	"idena-go/crypto"
)

const (
	MaxPayloadSize = 1024
)

var (
	NodeAlreadyActivated = errors.New("node is already in validator set")
	InvalidSignature     = errors.New("invalid signature")
	InvalidNonce         = errors.New("invalid nonce")
	InvalidEpoch         = errors.New("invalid epoch")
	InsufficientFunds    = errors.New("insufficient funds")
	InsufficientInvites  = errors.New("insufficient invites")
	RecipientRequired    = errors.New("recipient is required")
	InvitationIsMissing  = errors.New("invitation is missing")
	EmptyPayload         = errors.New("payload can't be empty")
	InvalidEpochTx       = errors.New("invalid epoch tx")
	InvalidPayload       = errors.New("invalid payload")
	InvalidRecipient     = errors.New("invalid recipient")
	LateTx               = errors.New("tx can't be accepted due to validation ceremony")
	validators           map[types.TxType]validator
)

type validator func(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error

func init() {
	validators = map[types.TxType]validator{
		types.RegularTx:           validateRegularTx,
		types.ActivationTx:        validateActivationTx,
		types.InviteTx:            validateSendInviteTx,
		types.SubmitFlipTx:        validateSubmitFlipTx,
		types.SubmitAnswersHashTx: validateSubmitAnswersHashTx,
		types.SubmitLongAnswersTx: validateSubmitLongAnswersTx,
	}
}

func ValidateTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if sender == (common.Address{}) {
		return InvalidSignature
	}

	if len(tx.Payload) > MaxPayloadSize {
		return InvalidPayload
	}

	globalEpoch := appState.State.Epoch()

	if globalEpoch > tx.Epoch {
		return InvalidEpoch
	}

	if appState.State.GetNonce(sender) >= tx.AccountNonce && appState.State.GetEpoch(sender) == globalEpoch && tx.Epoch == globalEpoch {
		return InvalidNonce
	}

	validator, ok := validators[tx.Type]
	if !ok {
		return nil
	}
	if err := validator(appState, tx, mempoolTx); err != nil {
		return err
	}

	return nil
}

// specific validation for regularTx
func validateRegularTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}

	cost := types.CalculateCost(appState.ValidatorsCache.GetCountOfValidNodes(), tx)
	if cost.Sign() > 0 && appState.State.GetBalance(sender).Cmp(cost) < 0 {
		return InsufficientFunds
	}
	return nil
}

// specific validation for approving tx
func validateActivationTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if len(tx.Payload) == 0 {
		return EmptyPayload
	}

	if addr, err := crypto.PubKeyBytesToAddress(tx.Payload); err != nil {
		return err
	} else {
		if addr != *tx.To {
			return InvalidPayload
		}
	}
	if err := validateRegularTx(appState, tx, mempoolTx); err != nil {
		return err
	}

	if appState.ValidatorsCache.Contains(*tx.To) {
		return NodeAlreadyActivated
	}

	if appState.State.GetIdentityState(sender) != state.Invite {
		return InvitationIsMissing
	}

	if appState.State.ValidationPeriod() > state.FlipLotteryPeriod {
		return LateTx
	}

	return nil
}

func validateSendInviteTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if err := validateRegularTx(appState, tx, mempoolTx); err != nil {
		return err
	}

	if appState.State.GetInvites(sender) == 0 {
		return InsufficientInvites
	}
	if appState.State.ValidationPeriod() > state.FlipLotteryPeriod {
		return LateTx
	}
	return nil
}

func validateSubmitFlipTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if err := validateRegularTx(appState, tx, mempoolTx); err != nil {
		return err
	}

	if *tx.To != sender {
		return InvalidRecipient
	}

	if appState.State.ValidationPeriod() > state.FlipLotteryPeriod {
		return LateTx
	}

	//TODO: you cannot submit more than one flip in current epoch

	return nil
}

func validateSubmitAnswersHashTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if err := validateRegularTx(appState, tx, mempoolTx); err != nil {
		return err
	}

	if len(tx.Payload) != common.HashLength {
		return InvalidPayload
	}

	if *tx.To != sender {
		return InvalidRecipient
	}
	return appState.EvidenceMap.ValidateTx(tx)
}

func validateSubmitLongAnswersTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	if mempoolTx && appState.State.ValidationPeriod() == state.LongSessionPeriod {
		return LateTx
	}

	if err := validateRegularTx(appState, tx, mempoolTx); err != nil {
		return err
	}
	sender, _ := types.Sender(tx)
	if *tx.To != sender {
		return InvalidRecipient
	}

	return nil
}

func ValidateFlipKey(appState *appstate.AppState, key *types.FlipKey) error {
	sender, _ := types.SenderFlipKey(key)
	if sender == (common.Address{}) {
		return InvalidSignature
	}
	return nil
}
