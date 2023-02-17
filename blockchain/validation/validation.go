package validation

import (
	"bytes"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/vrf/p256"
	"github.com/idena-network/idena-go/vm/embedded"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"math/big"
)

const (
	MaxPayloadSize          = 3 * 1024
	MaxPayloadSizeUpgrade11 = 3 * 1024 * 1024

	GodValidUntilNetworkSize = 10
)

type TxType = int

const (
	InBlockTx = TxType(1)
	MempoolTx = TxType(2)
	InboundTx = TxType(3)
)

var (
	NodeAlreadyActivated = errors.New("node is already in validator set")
	InvalidSignature     = errors.New("invalid signature")
	InvalidNonce         = errors.New("invalid nonce")
	InvalidEpoch         = errors.New("invalid epoch")
	InvalidAmount        = errors.New("invalid amount")
	InsufficientFunds    = errors.New("insufficient funds")
	InsufficientInvites  = errors.New("insufficient invites")
	RecipientRequired    = errors.New("recipient is required")
	InvitationIsMissing  = errors.New("invitation is missing")
	EmptyPayload         = errors.New("payload can't be empty")
	InvalidPayload       = errors.New("invalid payload")
	InvalidRecipient     = errors.New("invalid recipient")
	EarlyTx              = errors.New("tx can't be accepted due to wrong period")
	LateTx               = errors.New("tx can't be accepted due to validation ceremony")
	NotCandidate         = errors.New("user is not a candidate")
	InsufficientFlips    = errors.New("insufficient flips")
	IsAlreadyOnline      = errors.New("identity is already online or has pending online status")
	IsAlreadyOffline     = errors.New("identity is already offline or has pending offline status")
	DuplicatedFlip       = errors.New("duplicated flip")
	DuplicatedFlipPair   = errors.New("flip with these words already exists")
	BigFee               = errors.New("current fee is greater than tx max fee")
	InvalidMaxFee        = errors.New("invalid max fee")
	TooHighMaxFee        = errors.New("too high max fee")
	InvalidSender        = errors.New("invalid sender")
	FlipIsMissing        = errors.New("flip is missing")
	DuplicatedTx         = errors.New("duplicated tx")
	NegativeValue        = errors.New("value must be non-negative")
	SenderHasDelegatee   = errors.New("sender has delegatee already")
	SenderHasNoDelegatee = errors.New("sender has no delegatee")
	WrongEpoch           = errors.New("wrong epoch")
	InvalidDeployAmount  = errors.New("insufficient amount to create contract")
	SenderHasPenalty     = errors.New("sender has penalty")

	validators map[types.TxType]validator
)

var (
	CeremonialTxs = map[types.TxType]bool{
		types.SubmitAnswersHashTx:  true,
		types.SubmitShortAnswersTx: true,
		types.SubmitLongAnswersTx:  true,
		types.EvidenceTx:           true,
	}

	contractTxs = map[types.TxType]struct{}{
		types.CallContractTx:      {},
		types.DeployContractTx:    {},
		types.TerminateContractTx: {},
	}
)

type validator func(appState *appstate.AppState, tx *types.Transaction, txType TxType) error

var appCfg *config.Config
var cfgInitVersion config.ConsensusVerson

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
		types.BurnTx:               validateBurnTx,
		types.ChangeProfileTx:      validateChangeProfileTx,
		types.DeleteFlipTx:         validateDeleteFlipTx,
		types.DeployContractTx:     validateDeployContractTx,
		types.CallContractTx:       validateCallContractTx,
		types.TerminateContractTx:  validateTerminateContractTx,
		types.DelegateTx:           validateDelegateTx,
		types.UndelegateTx:         validateUndelegateTx,
		types.KillDelegatorTx:      validateKillDelegatorTx,
		types.StoreToIpfsTx:        validateStoreToIpfsTx,
		types.ReplenishStakeTx:     validateReplenishStakeTx,
	}
}
func SetAppConfig(cfg *config.Config) {
	appCfg = cfg
}

func getValidator(txType types.TxType) (validator, bool) {
	if appCfg != nil && cfgInitVersion != appCfg.Consensus.Version {
		cfgInitVersion = appCfg.Consensus.Version
	}
	v, ok := validators[txType]
	return v, ok
}

func checkIfNonNegative(value *big.Int) error {
	if value == nil {
		return nil
	}
	if value.Sign() == -1 {
		return NegativeValue
	}
	return nil
}

func ValidateTx(appState *appstate.AppState, tx *types.Transaction, minFeePerGas *big.Int, txType TxType) error {
	sender, _ := types.Sender(tx)

	if sender == (common.Address{}) {
		return InvalidSignature
	}

	if appCfg != nil && appCfg.Consensus.EnableUpgrade11 {
		if len(tx.Payload) > MaxPayloadSizeUpgrade11 {
			return InvalidPayload
		}
	} else if len(tx.Payload) > MaxPayloadSize {
		return InvalidPayload
	}

	if err := checkIfNonNegative(tx.Amount); err != nil {
		return errors.Wrap(err, "amount")
	}

	if err := checkIfNonNegative(tx.MaxFee); err != nil {
		return errors.Wrap(err, "maxFee")
	}

	if err := checkIfNonNegative(tx.Tips); err != nil {
		return errors.Wrap(err, "tips")
	}

	globalEpoch := appState.State.Epoch()

	if globalEpoch > tx.Epoch {
		return InvalidEpoch
	}

	nonce, epoch := appState.State.GetNonce(sender), appState.State.GetEpoch(sender)

	if nonce >= tx.AccountNonce && epoch == globalEpoch && tx.Epoch == globalEpoch {
		return errors.Wrapf(InvalidNonce, "state nonce: %v, state epoch: %v, tx nonce: %v, tx epoch: %v", nonce, epoch, tx.AccountNonce, tx.Epoch)
	}

	minFee := fee.CalculateFee(appState.ValidatorsCache.NetworkSize(), minFeePerGas, tx)
	if minFee.Cmp(tx.MaxFeeOrZero()) == 1 {
		return InvalidMaxFee
	}

	currentPeriod := appState.State.ValidationPeriod()
	if _, isCeremonial := CeremonialTxs[tx.Type]; !isCeremonial && txType != InBlockTx && (currentPeriod == state.FlipLotteryPeriod || currentPeriod == state.ShortSessionPeriod) {
		return LateTx
	}

	if err := ValidateFee(appState, tx, txType, minFeePerGas); err != nil {
		return err
	}
	if err := validateTotalCost(sender, appState, tx, txType); err != nil {
		return err
	}

	validator, ok := getValidator(tx.Type)
	if !ok {
		return errors.New("unknown tx type")
	}
	if err := validator(appState, tx, txType); err != nil {
		return err
	}

	return nil
}

func validateTotalCost(sender common.Address, appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	var cost *big.Int
	if txType != InBlockTx {
		cost = fee.CalculateMaxCost(tx)
	} else {
		cost = fee.CalculateCost(appState.ValidatorsCache.NetworkSize(), appState.State.FeePerGas(), tx)
		if _, ok := contractTxs[tx.Type]; ok {
			cost = fee.CalculateMaxCost(tx)
		}
	}
	if cost.Sign() > 0 && appState.State.GetBalance(sender).Cmp(cost) < 0 {
		return InsufficientFunds
	}
	return nil
}

func validateCeremonyTx(sender common.Address, appState *appstate.AppState, tx *types.Transaction) error {
	if appState.State.HasValidationTx(sender, tx.Type) {
		return DuplicatedTx
	}
	if appState.State.ValidationPeriod() == state.NonePeriod {
		return EarlyTx
	}
	return nil
}

func ValidateFee(appState *appstate.AppState, tx *types.Transaction, txType TxType, minFeePerGas *big.Int) error {
	enableUpgrade10 := appCfg != nil && appCfg.Consensus.EnableUpgrade10
	if enableUpgrade10 {
		enableUpgrade11 := appCfg != nil && appCfg.Consensus.EnableUpgrade11
		maxBlockGas := types.MaxBlockSize(enableUpgrade11)
		gasCost := minFeePerGas
		if gasCost != nil && gasCost.Sign() > 0 {
			maxTxGas := math.ToInt(decimal.NewFromBigInt(tx.MaxFeeOrZero(), 0).Div(decimal.NewFromBigInt(gasCost, 0))).Uint64()
			if maxTxGas > maxBlockGas {
				return TooHighMaxFee
			}
		}
	}
	if txType != InBlockTx {
		return nil
	}
	feeAmount := fee.CalculateFee(appState.ValidatorsCache.NetworkSize(), appState.State.FeePerGas(), tx)
	if feeAmount.Cmp(tx.MaxFeeOrZero()) == 1 {
		return BigFee
	}
	return nil
}

// specific validation for sendTx
func validateSendTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	if tx.To == nil {
		return RecipientRequired
	}

	return nil
}

// specific validation for approving tx
func validateActivationTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
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

	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}

	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}
	if *tx.To == appState.State.GodAddress() && appState.State.Epoch() > 0 {
		return InvalidRecipient
	}
	if appState.ValidatorsCache.IsValidated(*tx.To) {
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
	if appCfg != nil && appCfg.Consensus.EnableUpgrade10 {
		if recipientState == state.Invite && sender != *tx.To {
			return InvalidRecipient
		}
	}

	return nil
}

func validateSendInviteTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}
	godAddress := appState.State.GodAddress()
	if sender != godAddress && appState.State.GetInvites(sender) == 0 {
		return InsufficientInvites
	}
	if sender == godAddress && appState.State.GodAddressInvites() == 0 {
		return InsufficientInvites
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	if appState.State.GetIdentityState(*tx.To) != state.Undefined {
		return InvalidRecipient
	}
	if *tx.To == godAddress && (sender != godAddress || appState.State.Epoch() > 0) {
		return InvalidRecipient
	}

	return nil
}

func validateSubmitFlipTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	if appState.State.GetIdentityState(sender) < state.Candidate {
		return NotCandidate
	}

	identity := appState.State.GetIdentity(sender)
	god := appState.State.GodAddress()
	noFlips := identity.GetMaximumAvailableFlips() == uint8(len(identity.Flips))
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
		if flip.Pair == attachment.Pair {
			return DuplicatedFlipPair
		}
	}

	return nil
}

func validateSubmitAnswersHashTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if len(tx.Payload) != common.HashLength {
		return InvalidPayload
	}
	if appState.State.ValidationPeriod() < state.ShortSessionPeriod && txType == InBlockTx {
		return EarlyTx
	}
	if !state.IsCeremonyCandidate(appState.State.GetIdentity(sender)) {
		return NotCandidate
	}
	if err := validateCeremonyTx(sender, appState, tx); err != nil {
		return err
	}

	return nil
}

func validateSubmitShortAnswersTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if txType == InboundTx && appState.State.ValidationPeriod() == state.AfterLongSessionPeriod {
		return LateTx
	}
	if appState.State.ValidationPeriod() < state.LongSessionPeriod && txType == InBlockTx {
		return EarlyTx
	}

	identity := appState.State.GetIdentity(sender)
	if !state.IsCeremonyCandidate(identity) {
		return NotCandidate
	}

	if err := validateCeremonyTx(sender, appState, tx); err != nil {
		return err
	}

	attachment := attachments.ParseShortAnswerAttachment(tx)

	if attachment == nil {
		return InvalidPayload
	}

	return nil
}

func validateSubmitLongAnswersTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if txType == InboundTx && appState.State.ValidationPeriod() == state.AfterLongSessionPeriod {
		return LateTx
	}
	if appState.State.ValidationPeriod() < state.ShortSessionPeriod && txType == InBlockTx {
		return EarlyTx
	}
	if !state.IsCeremonyCandidate(appState.State.GetIdentity(sender)) {
		return NotCandidate
	}

	if err := validateCeremonyTx(sender, appState, tx); err != nil {
		return err
	}

	// we do not check VRF proof until first validation
	if appState.State.Epoch() == 0 {
		return nil
	}

	if types.IsValidLongSessionAnswers(tx) {
		return nil
	}

	attachment := attachments.ParseLongAnswerAttachment(tx)

	if attachment == nil || len(attachment.Proof) == 0 || len(attachment.Salt) == 0 {
		return InvalidPayload
	}

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

	types.MarkAsValidLongSessionAnswers(tx)

	return nil
}

func validateEvidenceTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if appState.State.ValidationPeriod() < state.LongSessionPeriod && txType == InBlockTx {
		return EarlyTx
	}
	identity := appState.State.GetIdentity(sender)
	if !state.IsCeremonyCandidate(identity) {
		return NotCandidate
	}
	if identity.State == state.Candidate && appState.ValidatorsCache.NetworkSize() != 0 || identity.Delegatee() != nil {
		return InvalidSender
	}
	discriminated := identity.IsDiscriminated(appState.State.Epoch())
	if discriminated && sender != appState.State.GodAddress() {
		return InvalidSender
	}
	if err := validateCeremonyTx(sender, appState, tx); err != nil {
		return err
	}
	return nil
}

func validateOnlineStatusTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	if !appState.ValidatorsCache.IsValidated(sender) && !appState.ValidatorsCache.IsPool(sender) {
		return InvalidSender
	}

	attachment := attachments.ParseOnlineStatusAttachment(tx)

	if attachment == nil {
		return InvalidPayload
	}

	identity := appState.State.GetIdentity(sender)
	if identity.Delegatee() != nil {
		return InvalidSender
	}

	hasPendingStatusSwitch := appState.State.HasStatusSwitchAddresses(sender)
	hasDelayedOfflinePenalty := appState.State.HasDelayedOfflinePenalty(sender)
	isOnline := appState.ValidatorsCache.IsOnlineIdentity(sender)
	isOffline := !isOnline

	if attachment.Online && (isOnline && !hasPendingStatusSwitch && !hasDelayedOfflinePenalty || isOffline && hasPendingStatusSwitch) {
		return IsAlreadyOnline
	}
	if !attachment.Online && (isOffline && !hasPendingStatusSwitch || isOnline && hasPendingStatusSwitch) {
		return IsAlreadyOffline
	}

	return nil
}

func validateKillIdentityTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	identityState := appState.State.GetIdentityState(sender)
	if identityState == state.Candidate || identityState == state.Newbie || identityState == state.Killed {
		return InvalidSender
	}
	if sender == appState.State.GodAddress() {
		return nil
	}
	if identityState == state.Undefined || identityState == state.Invite {
		return InvalidSender
	}
	return nil
}

func validateKillInviteeTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)
	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}
	if *tx.To == appState.State.GodAddress() {
		return InvalidRecipient
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	inviter := appState.State.GetInviter(*tx.To)
	if inviter == nil || inviter.Address != sender {
		return InvalidRecipient
	}
	identityState := appState.State.GetIdentityState(*tx.To)
	if identityState != state.Invite && identityState != state.Candidate {
		return InvalidRecipient
	}
	return nil
}

func validateChangeGodAddressTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)
	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if sender != appState.State.GodAddress() {
		return InvalidSender
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}

	return nil
}

func validateBurnTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	if tx.To != nil {
		return InvalidRecipient
	}
	attachment := attachments.ParseBurnAttachment(tx)
	if attachment == nil || len(attachment.Key) == 0 {
		return InvalidPayload
	}
	return nil
}

func validateChangeProfileTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	if tx.To != nil {
		return InvalidRecipient
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	attachment := attachments.ParseChangeProfileAttachment(tx)
	if attachment == nil {
		return InvalidPayload
	}
	return nil
}

func validateDeleteFlipTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}
	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}

	attachment := attachments.ParseDeleteFlipAttachment(tx)
	if attachment == nil {
		return InvalidPayload
	}

	identity := appState.State.GetIdentity(sender)

	exist := false
	for _, flip := range identity.Flips {
		if bytes.Compare(flip.Cid, attachment.Cid) == 0 {
			exist = true
			break
		}
	}
	if !exist {
		return FlipIsMissing
	}

	return nil
}

func validateCallContractTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}

	codeHash := appState.State.GetCodeHash(*tx.To)
	if codeHash == nil {
		return InvalidRecipient
	}

	attachment := attachments.ParseCallContractAttachment(tx)
	if attachment == nil {
		return InvalidPayload
	}

	return nil
}

func validateDeployContractTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {

	if tx.To != nil {
		return InvalidRecipient
	}

	minStake := big.NewInt(0).Mul(appState.State.FeePerGas(), big.NewInt(3000000))

	if tx.AmountOrZero().Cmp(minStake) < 0 {
		return InvalidDeployAmount
	}

	attachment := attachments.ParseDeployContractAttachment(tx)
	if attachment == nil {
		return InvalidPayload
	}
	if len(attachment.Code) > 0 && (appCfg == nil || !appCfg.Consensus.EnableUpgrade11) {
		return InvalidPayload
	}
	if _, ok := embedded.AvailableContracts[attachment.CodeHash]; !ok && len(attachment.Code) == 0 {
		return InvalidPayload
	}
	return nil
}

func validateTerminateContractTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}

	codeHash := appState.State.GetCodeHash(*tx.To)
	if codeHash == nil {
		return InvalidRecipient
	}

	if _, ok := embedded.AvailableContracts[*codeHash]; !ok && appCfg != nil && appCfg.Consensus.EnableUpgrade11 {
		return InvalidRecipient
	}

	if appCfg != nil && appCfg.Consensus.EnableUpgrade11 && !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}

	attachment := attachments.ParseTerminateContractAttachment(tx)
	if attachment == nil {
		return InvalidPayload
	}

	return nil
}

func validateDelegateTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)
	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}

	if sender == *tx.To {
		return InvalidRecipient
	}

	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}

	if appState.ValidatorsCache.IsPool(sender) {
		return InvalidSender
	}

	if appState.State.Delegatee(*tx.To) != nil {
		return InvalidRecipient
	}

	enableUpgrade10 := appCfg != nil && appCfg.Consensus.EnableUpgrade10
	if !enableUpgrade10 {
		to := appState.State.GetIdentity(*tx.To)
		if to.PendingUndelegation() != nil {
			return InvalidRecipient
		}
	}

	delegatee := appState.State.Delegatee(sender)
	delegationSwitch := appState.State.DelegationSwitch(sender)

	if delegatee == nil {
		if delegationSwitch != nil && !delegationSwitch.Delegatee.IsEmpty() {
			return SenderHasDelegatee
		}
		if !enableUpgrade10 {
			identity := appState.State.GetIdentity(sender)
			if prevDelegatee := identity.PendingUndelegation(); prevDelegatee != nil && *prevDelegatee != *tx.To {
				return InvalidRecipient
			}
		}
	} else {
		if delegationSwitch == nil || !delegationSwitch.Delegatee.IsEmpty() {
			return SenderHasDelegatee
		}
		if *delegatee != *tx.To {
			return InvalidRecipient
		}
	}

	if enableUpgrade10 {
		if appState.State.GetPenaltySeconds(sender) > 0 {
			return SenderHasPenalty
		}
	}

	return nil
}

func validateUndelegateTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To != nil {
		return InvalidRecipient
	}

	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}

	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}

	delegatee := appState.State.Delegatee(sender)
	delegationSwitch := appState.State.DelegationSwitch(sender)

	if delegatee != nil {
		if delegationSwitch != nil && delegationSwitch.Delegatee.IsEmpty() {
			return SenderHasNoDelegatee
		}
	} else {
		if delegationSwitch == nil || delegationSwitch.Delegatee.IsEmpty() {
			return SenderHasNoDelegatee
		}
	}

	if appState.State.DelegationEpoch(sender) == appState.State.Epoch() {
		return WrongEpoch
	}

	return nil
}

func validateKillDelegatorTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	sender, _ := types.Sender(tx)

	if tx.To == nil || *tx.To == (common.Address{}) {
		return RecipientRequired
	}

	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	delegatee := appState.State.Delegatee(*tx.To)
	if delegatee == nil || *delegatee != sender {
		return InvalidSender
	}
	return nil
}

func validateStoreToIpfsTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	if tx.To != nil {
		return InvalidRecipient
	}

	if !common.ZeroOrNil(tx.AmountOrZero()) {
		return InvalidAmount
	}

	attachment := attachments.ParseStoreToIpfsAttachment(tx)
	if attachment == nil || attachment.Cid == nil {
		return InvalidPayload
	}

	_, err := cid.Cast(attachment.Cid)
	if err != nil {
		return err
	}
	return nil
}

func validateReplenishStakeTx(appState *appstate.AppState, tx *types.Transaction, txType TxType) error {
	if tx.To == nil {
		return RecipientRequired
	}
	recipient := appState.State.GetIdentity(*tx.To)
	canReplenishStake := recipient.State != state.Undefined && recipient.State != state.Killed || appCfg != nil && appCfg.Consensus.EnableUpgrade10 && *tx.To == appState.State.GodAddress()
	if !canReplenishStake {
		return InvalidRecipient
	}
	if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod {
		return LateTx
	}
	return nil
}
