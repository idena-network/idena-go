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
	MaxPayloadSize           = 1024
	GodValidUntilNetworkSize = 10
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
	EarlyTx              = errors.New("tx can't be accepted due to wrong period")
	LateTx               = errors.New("tx can't be accepted due to validation ceremony")
	NotCandidate         = errors.New("user is not a candidate")
	InsufficientFlips    = errors.New("insufficient flips")
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
		types.EvidenceTx:          validateEvidenceTx,
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

	cost := types.CalculateCost(appState.ValidatorsCache.NetworkSize(), tx)
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

	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}

	recipientState := appState.State.GetIdentityState(*tx.To)
	if recipientState != state.Invite && recipientState != state.Undefined {
		return InvalidRecipient
	}

	return nil
}

func validateSendInviteTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if err := validateRegularTx(appState, tx, mempoolTx); err != nil {
		return err
	}

	if appState.State.GetInvites(sender) == 0 && sender != appState.State.GodAddress() {
		return InsufficientInvites
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	if appState.State.GetIdentityState(*tx.To) != state.Undefined {
		return InvalidRecipient
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

	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}

	god := appState.State.GodAddress()
	noFlips := appState.State.GetRequiredFlips(sender) == appState.State.GetMadeFlips(sender)
	if noFlips && sender != god ||
		noFlips && sender == god && appState.ValidatorsCache.NetworkSize() > GodValidUntilNetworkSize {
		return InsufficientFlips
	}

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

	if appState.State.ValidationPeriod() < state.ShortSessionPeriod {
		return EarlyTx
	}

	if !state.IsCeremonyCandidate(appState.State.GetIdentity(sender)) {
		return NotCandidate
	}

	return nil
}

func validateSubmitLongAnswersTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	if mempoolTx && appState.State.ValidationPeriod() == state.AfterLongSessionPeriod {
		return LateTx
	}

	if err := validateRegularTx(appState, tx, mempoolTx); err != nil {
		return err
	}
	sender, _ := types.Sender(tx)
	if *tx.To != sender {
		return InvalidRecipient
	}

	if appState.State.ValidationPeriod() < state.ShortSessionPeriod {
		return EarlyTx
	}

	if !state.IsCeremonyCandidate(appState.State.GetIdentity(sender)) {
		return NotCandidate
	}

	return nil
}

func validateEvidenceTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	if err := validateRegularTx(appState, tx, mempoolTx); err != nil {
		return err
	}
	sender, _ := types.Sender(tx)
	if *tx.To != sender {
		return InvalidRecipient
	}

	if appState.State.ValidationPeriod() < state.LongSessionPeriod {
		return EarlyTx
	}

	if !state.IsCeremonyCandidate(appState.State.GetIdentity(sender)) {
		return NotCandidate
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
