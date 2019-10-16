package validation

import (
	"bytes"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/vrf/p256"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"math/big"
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
	DuplicatedFlip       = errors.New("duplicated flip")
	DuplicatedFlipPair   = errors.New("flip with these words already exists")
	BigFee               = errors.New("current fee is greater than tx max fee")
	InvalidMaxFee        = errors.New("invalid max fee")
	InvalidSender        = errors.New("invalid sender")
	validators           map[types.TxType]validator
)

type validator func(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error

func init() {
	validators = map[types.TxType]validator{
		types.SendTx:               validateSendTx,
		types.ActivationTx:         validateActivationTx,
		types.InviteTx:             validateSendInviteTx,
		types.KillTx:               validateKillIdentityTx,
		types.KillInviteeTx:        validateKillInviteeTx,
		types.SubmitFlipTx:         validateSubmitFlipTx,
		types.SubmitAnswersHashTx:  validateSubmitAnswersHashTx,
		types.SubmitShortAnswersTx: validateSubmitShortAnswersTx,
		types.SubmitLongAnswersTx:  validateSubmitLongAnswersTx,
		types.EvidenceTx:           validateEvidenceTx,
		types.OnlineStatusTx:       validateOnlineStatusTx,
		types.ChangeGodAddressTx:   validateChangeGodAddressTx,
	}
}

func ValidateTx(appState *appstate.AppState, tx *types.Transaction, minFeePerByte *big.Int, mempoolTx bool) error {
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

	minFee := fee.CalculateFee(appState.ValidatorsCache.NetworkSize(), minFeePerByte, tx)
	if minFee.Cmp(tx.MaxFeeOrZero()) == 1 {
		return InvalidMaxFee
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

func validateTotalCost(sender common.Address, appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	var cost *big.Int
	if mempoolTx {
		cost = calculateMaxCost(tx)
	} else {
		cost = fee.CalculateCost(appState.ValidatorsCache.NetworkSize(), appState.State.FeePerByte(), tx)
	}
	if cost.Sign() > 0 && appState.State.GetBalance(sender).Cmp(cost) < 0 {
		return InsufficientFunds
	}
	return nil
}

func ValidateFee(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	if mempoolTx {
		return nil
	}
	fee := fee.CalculateFee(appState.ValidatorsCache.NetworkSize(), appState.State.FeePerByte(), tx)
	if fee.Cmp(tx.MaxFeeOrZero()) == 1 {
		return BigFee
	}
	return nil
}

func calculateMaxCost(tx *types.Transaction) *big.Int {
	result := big.NewInt(0)
	result.Add(result, tx.AmountOrZero())
	result.Add(result, tx.TipsOrZero())
	result.Add(result, tx.MaxFeeOrZero())
	return result
}

// specific validation for sendTx
func validateSendTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}

	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}

	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
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
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
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
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
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
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
		return err
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	if appState.State.GetIdentityState(sender) < state.Candidate {
		return NotCandidate
	}

	identity := appState.State.GetIdentity(sender)
	god := appState.State.GodAddress()
	noFlips := identity.RequiredFlips == uint8(len(identity.Flips))
	if noFlips && sender != god ||
		noFlips && sender == god && appState.ValidatorsCache.NetworkSize() > GodValidUntilNetworkSize {
		return InsufficientFlips
	}

	attachment := attachments.ParseFlipSubmitAttachment(tx)
	if attachment == nil {
		return InvalidPayload
	}
	_, err := cid.Parse(attachment.Cid)
	if err != nil {
		return InvalidPayload
	}

	// pair index should be less than total words count
	if identity.GetTotalWordPairsCount() <= int(attachment.Pair) && sender != god {
		return InvalidPayload
	}

	for _, flip := range identity.Flips {
		if bytes.Compare(flip.Cid, attachment.Cid) == 0 {
			return DuplicatedFlip
		}
		// TODO: enable when UI will be ready (fork)
		//if flip.Pair == attachment.Pair {
		//	return DuplicatedFlipPair
		//}
	}

	return nil
}

func validateSubmitAnswersHashTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
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
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
		return err
	}
	if appState.State.ValidationPeriod() < state.LongSessionPeriod && !mempoolTx {
		return EarlyTx
	}

	identity := appState.State.GetIdentity(sender)
	if !state.IsCeremonyCandidate(identity) {
		return NotCandidate
	}

	// we do not check VRF proof until first validation
	if appState.State.Epoch() == 0 {
		return nil
	}

	attachment := attachments.ParseShortAnswerAttachment(tx)
	seed := appState.State.FlipWordsSeed()
	rawPubKey, _ := types.SenderPubKey(tx)
	pubKey, err := crypto.UnmarshalPubkey(rawPubKey)
	if err != nil {
		return err
	}
	verifier, err := p256.NewVRFVerifier(pubKey)
	if err != nil {
		return err
	}
	_, err = verifier.ProofToHash(seed[:], attachment.Proof)
	if err != nil {
		return err
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
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
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
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
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
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
		return err
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	if !appState.ValidatorsCache.Contains(sender) {
		return InvalidRecipient
	}

	attachment := attachments.ParseOnlineStatusAttachment(tx)

	if attachment == nil {
		return InvalidPayload
	}

	if attachment.Online && appState.ValidatorsCache.IsOnlineIdentity(sender) {
		return IsAlreadyOnline
	}
	if !attachment.Online && !appState.ValidatorsCache.IsOnlineIdentity(sender) {
		return IsAlreadyOffline
	}

	return nil
}

func validateKillIdentityTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)

	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	var txFee *big.Int
	if mempoolTx {
		txFee = tx.MaxFeeOrZero()
	} else {
		txFee = fee.CalculateFee(appState.ValidatorsCache.NetworkSize(), appState.State.FeePerByte(), tx)
	}
	if txFee.Sign() > 0 && appState.State.GetStakeBalance(sender).Cmp(txFee) < 0 {
		return InsufficientFunds
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	cost := new(big.Int).Add(tx.AmountOrZero(), tx.TipsOrZero())
	if appState.State.GetBalance(sender).Cmp(cost) < 0 {
		return InsufficientFunds
	}

	return nil
}

func validateKillInviteeTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)
	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
		return err
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	inviter := appState.State.GetInviter(*tx.To)
	if inviter == nil || inviter.Address != sender {
		return InvalidRecipient
	}
	return nil
}

func validateChangeGodAddressTx(appState *appstate.AppState, tx *types.Transaction, mempoolTx bool) error {
	sender, _ := types.Sender(tx)
	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}
	if sender != appState.State.GodAddress() {
		return InvalidSender
	}
	if err := ValidateFee(appState, tx, mempoolTx); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, mempoolTx); err != nil {
		return err
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}

	return nil
}
