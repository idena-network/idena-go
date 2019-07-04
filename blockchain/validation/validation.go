package validation

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/pkg/errors"
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
	NotIdentity          = errors.New("user is not identity")
	InsufficientFlips    = errors.New("insufficient flips")
	IsAlreadyOnline      = errors.New("identity is already online")
	IsAlreadyOffline     = errors.New("identity is already offline")
	validators           map[types.TxType]validator
)

type validator func(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error

func init() {
	validators = map[types.TxType]validator{
		types.SendTx:               validateSendTx,
		types.ActivationTx:         validateActivationTx,
		types.InviteTx:             validateSendInviteTx,
		types.KillTx:               validateKillIdentityTx,
		types.SubmitFlipTx:         validateSubmitFlipTx,
		types.SubmitAnswersHashTx:  validateSubmitAnswersHashTx,
		types.SubmitShortAnswersTx: validateSubmitShortAnswersTx,
		types.SubmitLongAnswersTx:  validateSubmitLongAnswersTx,
		types.EvidenceTx:           validateEvidenceTx,
		types.OnlineStatusTx:       validateOnlineStatusTx,
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

	nonce, epoch := appState.State.GetNonce(sender), appState.State.GetEpoch(sender)

	if nonce >= tx.AccountNonce && epoch == globalEpoch && tx.Epoch == globalEpoch {
		return errors.Errorf("invalid nonce, state nonce: %v, state epoch: %v, tx nonce: %v, tx epoch: %v", nonce, epoch, tx.AccountNonce, tx.Epoch)
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

func validateTotalCost(sender common.Address, appState *appstate.AppState, tx *types.Transaction) error {
	cost := types.CalculateCost(appState.ValidatorsCache.NetworkSize(), tx)
	if cost.Sign() > 0 && appState.State.GetBalance(sender).Cmp(cost) < 0 {
		return InsufficientFunds
	}
	return nil
}

// specific validation for sendTx
func validateSendTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}

	if err := validateTotalCost(sender, appState, tx); err != nil {
		return err
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

	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}
	if err := validateTotalCost(sender, appState, tx); err != nil {
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

	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}
	if err := validateTotalCost(sender, appState, tx); err != nil {
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

	if tx.To != nil {
		return InvalidRecipient
	}
	if err := validateTotalCost(sender, appState, tx); err != nil {
		return err
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	if appState.State.GetIdentityState(sender) < state.Candidate {
		return NotCandidate
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

	if tx.To != nil {
		return InvalidRecipient
	}
	if err := validateTotalCost(sender, appState, tx); err != nil {
		return err
	}
	if len(tx.Payload) != common.HashLength {
		return InvalidPayload
	}
	if appState.State.ValidationPeriod() < state.ShortSessionPeriod && !mempoolTx {
		return EarlyTx
	}
	if !state.IsCeremonyCandidate(appState.State.GetIdentity(sender)) {
		return NotCandidate
	}

	return nil
}

func validateSubmitShortAnswersTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if mempoolTx && appState.State.ValidationPeriod() == state.AfterLongSessionPeriod {
		return LateTx
	}
	if err := validateTotalCost(sender, appState, tx); err != nil {
		return err
	}
	if appState.State.ValidationPeriod() < state.LongSessionPeriod && !mempoolTx {
		return EarlyTx
	}
	if !state.IsCeremonyCandidate(appState.State.GetIdentity(sender)) {
		return NotCandidate
	}
	return nil
}

func validateSubmitLongAnswersTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if mempoolTx && appState.State.ValidationPeriod() == state.AfterLongSessionPeriod {
		return LateTx
	}
	if err := validateTotalCost(sender, appState, tx); err != nil {
		return err
	}
	if appState.State.ValidationPeriod() < state.ShortSessionPeriod && !mempoolTx {
		return EarlyTx
	}
	if !state.IsCeremonyCandidate(appState.State.GetIdentity(sender)) {
		return NotCandidate
	}

	return nil
}

func validateEvidenceTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if err := validateTotalCost(sender, appState, tx); err != nil {
		return err
	}
	if appState.State.ValidationPeriod() < state.LongSessionPeriod && !mempoolTx {
		return EarlyTx
	}
	if !state.IsCeremonyCandidate(appState.State.GetIdentity(sender)) {
		return NotCandidate
	}

	return nil
}

func validateOnlineStatusTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if err := validateTotalCost(sender, appState, tx); err != nil {
		return err
	}
	if !appState.ValidatorsCache.Contains(sender) {
		return InvalidRecipient
	}

	shouldBecomeOnline := len(tx.Payload) > 0 && tx.Payload[0] != 0

	if shouldBecomeOnline && appState.ValidatorsCache.IsOnlineIdentity(sender) {
		return IsAlreadyOnline
	}
	if !shouldBecomeOnline && !appState.ValidatorsCache.IsOnlineIdentity(sender) {
		return IsAlreadyOffline
	}

	return nil
}

func validateKillIdentityTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}
	fee := types.CalculateFee(appState.ValidatorsCache.NetworkSize(), tx)
	if fee.Sign() > 0 && appState.State.GetStakeBalance(sender).Cmp(fee) < 0 {
		return InsufficientFunds
	}
	if appState.State.GetBalance(sender).Cmp(tx.AmountOrZero()) < 0 {
		return InsufficientFunds
	}
	if appState.State.GetIdentityState(sender) <= state.Candidate {
		return NotIdentity
	}

	return nil
}
