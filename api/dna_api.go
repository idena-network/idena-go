package api

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/ceremony"
	"github.com/idena-network/idena-go/core/profile"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"time"
)

type DnaApi struct {
	bc             *blockchain.Blockchain
	baseApi        *BaseApi
	ceremony       *ceremony.ValidationCeremony
	appVersion     string
	profileManager *profile.Manager
}

func NewDnaApi(baseApi *BaseApi, bc *blockchain.Blockchain, ceremony *ceremony.ValidationCeremony, appVersion string,
	profileManager *profile.Manager) *DnaApi {
	return &DnaApi{bc, baseApi, ceremony, appVersion, profileManager}
}

type State struct {
	Name string `json:"name"`
}

func (api *DnaApi) State() State {
	return State{
		Name: api.baseApi.engine.GetProcess(),
	}
}

func (api *DnaApi) GetCoinbaseAddr() common.Address {
	return api.baseApi.getCurrentCoinbase()
}

type Balance struct {
	Stake        decimal.Decimal `json:"stake"`
	Balance      decimal.Decimal `json:"balance"`
	Nonce        uint32          `json:"nonce"`
	MempoolNonce uint32          `json:"mempoolNonce"`
}

func (api *DnaApi) GetBalance(address common.Address) Balance {
	state := api.baseApi.getReadonlyAppState()
	currentEpoch := state.State.Epoch()
	nonce, epoch := state.State.GetNonce(address), state.State.GetEpoch(address)
	if epoch < currentEpoch {
		nonce = 0
	}

	return Balance{
		Stake:        blockchain.ConvertToFloat(state.State.GetStakeBalance(address)),
		Balance:      blockchain.ConvertToFloat(state.State.GetBalance(address)),
		Nonce:        nonce,
		MempoolNonce: state.NonceCache.GetNonce(address, currentEpoch),
	}
}

// SendTxArgs represents the arguments to submit a new transaction into the transaction pool.
type SendTxArgs struct {
	Type     types.TxType    `json:"type"`
	From     common.Address  `json:"from"`
	To       *common.Address `json:"to"`
	Amount   decimal.Decimal `json:"amount"`
	MaxFee   decimal.Decimal `json:"maxFee"`
	Payload  *hexutil.Bytes  `json:"payload"`
	Tips     decimal.Decimal `json:"tips"`
	UseProto bool            `json:"useProto"`
	BaseTxArgs
}

// SendInviteArgs represents the arguments to send invite
type SendInviteArgs struct {
	To     common.Address  `json:"to"`
	Amount decimal.Decimal `json:"amount"`
	BaseTxArgs
}

type ActivateInviteArgs struct {
	Key    string         `json:"key"`
	PubKey *hexutil.Bytes `json:"pubKey"`
	BaseTxArgs
}

type DelegateTxArgs struct {
	To *common.Address `json:"to"`
	BaseTxArgs
}

type KillDelegatorTxArgs struct {
	To *common.Address `json:"to"`
	BaseTxArgs
}

type StoreToIpfsTxArgs struct {
	Cid string `json:"cid"`
	BaseTxArgs
}

type Invite struct {
	Hash     common.Hash    `json:"hash"`
	Receiver common.Address `json:"receiver"`
	Key      string         `json:"key"`
}

func (api *DnaApi) SendInvite(ctx context.Context, args SendInviteArgs) (Invite, error) {
	receiver := args.To
	var key *ecdsa.PrivateKey

	if receiver == (common.Address{}) {
		key, _ = crypto.GenerateKey()
		receiver = crypto.PubkeyToAddress(key.PublicKey)
	}

	hash, err := api.baseApi.sendTx(ctx, api.baseApi.getCurrentCoinbase(), &receiver, types.InviteTx, args.Amount, decimal.Zero, decimal.Zero, args.Nonce, args.Epoch, nil, nil)

	if err != nil {
		return Invite{}, err
	}

	var stringKey string
	if key != nil {
		stringKey = hex.EncodeToString(crypto.FromECDSA(key))
	}

	return Invite{
		Receiver: receiver,
		Hash:     hash,
		Key:      stringKey,
	}, nil
}

func (api *DnaApi) ActivateInvite(ctx context.Context, args ActivateInviteArgs) (common.Hash, error) {
	var key *ecdsa.PrivateKey
	from := api.baseApi.getCurrentCoinbase()
	if len(args.Key) > 0 {
		var err error
		b, err := hex.DecodeString(args.Key)
		if err != nil {
			return common.Hash{}, err
		}
		key, err = crypto.ToECDSA(b)
		if err != nil {
			return common.Hash{}, err
		}
		from = crypto.PubkeyToAddress(key.PublicKey)
	}

	var pubKey []byte
	if args.PubKey != nil {
		pubKey = *args.PubKey
	} else {
		pubKey = api.baseApi.secStore.GetPubKey()
	}
	to, err := crypto.PubKeyBytesToAddress(pubKey)
	if err != nil {
		return common.Hash{}, err
	}
	hash, err := api.baseApi.sendTx(ctx, from, &to, types.ActivationTx, decimal.Zero, decimal.Zero, decimal.Zero, args.Nonce, args.Epoch, pubKey, key)

	if err != nil {
		return common.Hash{}, err
	}

	return hash, nil
}

func (api *DnaApi) BecomeOnline(ctx context.Context, args BaseTxArgs) (common.Hash, error) {
	from := api.baseApi.getCurrentCoinbase()
	hash, err := api.baseApi.sendTx(ctx, from, nil, types.OnlineStatusTx, decimal.Zero, decimal.Zero, decimal.Zero, args.Nonce, args.Epoch, attachments.CreateOnlineStatusAttachment(true), nil)

	if err != nil {
		return common.Hash{}, err
	}

	return hash, nil
}

func (api *DnaApi) BecomeOffline(ctx context.Context, args BaseTxArgs) (common.Hash, error) {
	from := api.baseApi.getCurrentCoinbase()
	hash, err := api.baseApi.sendTx(ctx, from, nil, types.OnlineStatusTx, decimal.Zero, decimal.Zero, decimal.Zero, args.Nonce, args.Epoch, attachments.CreateOnlineStatusAttachment(false), nil)

	if err != nil {
		return common.Hash{}, err
	}

	return hash, nil
}

func (api *DnaApi) Delegate(ctx context.Context, args DelegateTxArgs) (common.Hash, error) {
	from := api.baseApi.getCurrentCoinbase()
	hash, err := api.baseApi.sendTx(ctx, from, args.To, types.DelegateTx, decimal.Zero, decimal.Zero, decimal.Zero, args.Nonce, args.Epoch, nil, nil)

	if err != nil {
		return common.Hash{}, err
	}

	return hash, nil
}

func (api *DnaApi) Undelegate(ctx context.Context, args BaseTxArgs) (common.Hash, error) {
	from := api.baseApi.getCurrentCoinbase()
	hash, err := api.baseApi.sendTx(ctx, from, nil, types.UndelegateTx, decimal.Zero, decimal.Zero, decimal.Zero, args.Nonce, args.Epoch, nil, nil)

	if err != nil {
		return common.Hash{}, err
	}
	return hash, nil
}

func (api *DnaApi) KillDelegator(ctx context.Context, args KillDelegatorTxArgs) (common.Hash, error) {
	from := api.baseApi.getCurrentCoinbase()
	hash, err := api.baseApi.sendTx(ctx, from, args.To, types.KillDelegatorTx, decimal.Zero, decimal.Zero, decimal.Zero, args.Nonce, args.Epoch, nil, nil)

	if err != nil {
		return common.Hash{}, err
	}
	return hash, nil
}

func (api *DnaApi) StoreToIpfs(ctx context.Context, args StoreToIpfsTxArgs) (common.Hash, error) {
	from := api.baseApi.getCurrentCoinbase()
	c, err := cid.Decode(args.Cid)
	if err != nil {
		return common.Hash{}, err
	}
	data, err := api.baseApi.ipfs.Get(c.Bytes(), 0)
	if err != nil || len(data) == 0 {
		return common.Hash{}, errors.New("cannot read ipfs content from local storage")
	}

	hash, err := api.baseApi.sendTx(ctx, from, nil, types.StoreToIpfsTx, decimal.Zero, decimal.Zero, decimal.Zero, args.Nonce, args.Epoch, attachments.CreateStoreToIpfsAttachment(c.Bytes(), uint32(len(data))), nil)

	if err != nil {
		return common.Hash{}, err
	}
	return hash, nil
}

func (api *DnaApi) SendTransaction(ctx context.Context, args SendTxArgs) (common.Hash, error) {

	var payload []byte
	if args.Payload != nil {
		payload = *args.Payload
	}

	//TODO: remove after UI update
	if args.Type == types.KillTx {
		args.To = nil
	}

	return api.baseApi.sendTx(ctx, args.From, args.To, args.Type, args.Amount, args.MaxFee, args.Tips, args.Nonce, args.Epoch, payload, nil)
}

type FlipWords struct {
	Words [2]uint32 `json:"words"`
	Used  bool      `json:"used"`
	Id    int       `json:"id"`
}

type Inviter struct {
	TxHash      common.Hash    `json:"txHash"`
	Address     common.Address `json:"address"`
	EpochHeight uint32         `json:"epochHeight"`
}

type Identity struct {
	Address             common.Address  `json:"address"`
	ProfileHash         string          `json:"profileHash"`
	Stake               decimal.Decimal `json:"stake"`
	Invites             uint8           `json:"invites"`
	Age                 uint16          `json:"age"`
	State               string          `json:"state"`
	PubKey              string          `json:"pubkey"`
	RequiredFlips       uint8           `json:"requiredFlips"`
	AvailableFlips      uint8           `json:"availableFlips"`
	FlipKeyWordPairs    []FlipWords     `json:"flipKeyWordPairs"`
	MadeFlips           uint8           `json:"madeFlips"`
	QualifiedFlips      uint32          `json:"totalQualifiedFlips"`
	ShortFlipPoints     float32         `json:"totalShortFlipPoints"`
	Flips               []string        `json:"flips"`
	Online              bool            `json:"online"`
	Generation          uint32          `json:"generation"`
	Code                hexutil.Bytes   `json:"code"`
	Invitees            []state.TxAddr  `json:"invitees"`
	Penalty             decimal.Decimal `json:"penalty"`
	LastValidationFlags []string        `json:"lastValidationFlags"`
	Delegatee           *common.Address `json:"delegatee"`
	DelegationEpoch     uint16          `json:"delegationEpoch"`
	DelegationNonce     uint32          `json:"delegationNonce"`
	PendingUndelegation *common.Address `json:"pendingUndelegation"`
	IsPool              bool            `json:"isPool"`
	Inviter             *Inviter        `json:"inviter"`
	ShardId             uint32          `json:"shardId"`
}

func (api *DnaApi) Identities() []Identity {
	var identities []Identity

	appState := api.baseApi.getReadonlyAppState()

	epoch := appState.State.Epoch()
	appState.State.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])

		var data state.Identity
		if err := data.FromBytes(value); err != nil {
			return false
		}
		var flipKeyWordPairs []int
		if addr == api.GetCoinbaseAddr() {
			flipKeyWordPairs = api.ceremony.FlipKeyWordPairs()
		}
		identities = append(identities, convertIdentity(epoch, addr, data, flipKeyWordPairs, appState))

		return false
	})

	return identities
}

func (api *DnaApi) Identity(address *common.Address) Identity {
	var flipKeyWordPairs []int
	coinbase := api.GetCoinbaseAddr()
	if address == nil || *address == coinbase {
		address = &coinbase
		flipKeyWordPairs = api.ceremony.FlipKeyWordPairs()
	}

	appState := api.baseApi.getReadonlyAppState()
	return convertIdentity(appState.State.Epoch(), *address, appState.State.GetIdentity(*address), flipKeyWordPairs, appState)
}

func convertIdentity(currentEpoch uint16, address common.Address, data state.Identity, flipKeyWordPairs []int, appState *appstate.AppState) Identity {
	var s string
	switch data.State {
	case state.Invite:
		s = "Invite"
	case state.Candidate:
		s = "Candidate"
	case state.Newbie:
		s = "Newbie"
	case state.Verified:
		s = "Verified"
	case state.Suspended:
		s = "Suspended"
	case state.Zombie:
		s = "Zombie"
	case state.Killed:
		s = "Killed"
	case state.Human:
		s = "Human"
	default:
		s = "Undefined"
	}

	var flags []string
	if data.LastValidationStatus.HasFlag(state.AllFlipsNotQualified) {
		flags = append(flags, "AllFlipsNotQualified")
	}
	if data.LastValidationStatus.HasFlag(state.AtLeastOneFlipNotQualified) {
		flags = append(flags, "AtLeastOneFlipNotQualified")
	}
	if data.LastValidationStatus.HasFlag(state.AtLeastOneFlipReported) {
		flags = append(flags, "AtLeastOneFlipReported")
	}

	var profileHash string
	if len(data.ProfileHash) > 0 {
		c, _ := cid.Parse(data.ProfileHash)
		profileHash = c.String()
	}

	var result []string
	usedPairs := mapset.NewSet()
	for _, v := range data.Flips {
		c, _ := cid.Parse(v.Cid)
		result = append(result, c.String())
		usedPairs.Add(v.Pair)
	}

	var convertedFlipKeyWordPairs []FlipWords
	for i := 0; i < len(flipKeyWordPairs)/2; i++ {
		convertedFlipKeyWordPairs = append(convertedFlipKeyWordPairs,
			FlipWords{
				Words: [2]uint32{uint32(flipKeyWordPairs[i*2]), uint32(flipKeyWordPairs[i*2+1])},
				Used:  usedPairs.Contains(uint8(i)),
				Id:    i,
			})
	}

	var invitees []state.TxAddr
	if len(data.Invitees) > 0 {
		invitees = data.Invitees
	}

	age := uint16(0)
	if data.Birthday > 0 {
		age = currentEpoch - data.Birthday
	}

	totalPoints, totalFlips := common.CalculateIdentityScores(data.Scores, data.GetShortFlipPoints(), data.QualifiedFlips)

	isOnline := appState.ValidatorsCache.IsOnlineIdentity(address)
	hasPendingStatusSwitch := appState.State.HasStatusSwitchAddresses(address)
	if hasPendingStatusSwitch {
		isOnline = !isOnline
	}
	if appState.State.HasDelayedOfflinePenalty(address) {
		isOnline = false
	}

	delegatee := data.Delegatee()
	pendingUndelegation := data.PendingUndelegation()
	switchDelegation := appState.State.DelegationSwitch(address)
	if switchDelegation != nil {
		if switchDelegation.Delegatee.IsEmpty() {
			delegatee = nil
		} else {
			delegatee = &switchDelegation.Delegatee
			pendingUndelegation = nil
		}
	}

	var inviter *Inviter
	if data.Inviter != nil {
		inviter = &Inviter{
			TxHash:      data.Inviter.TxHash,
			Address:     data.Inviter.Address,
			EpochHeight: data.Inviter.EpochHeight,
		}
	}

	return Identity{
		Address:             address,
		State:               s,
		Stake:               blockchain.ConvertToFloat(data.Stake),
		Age:                 age,
		Invites:             data.Invites,
		ProfileHash:         profileHash,
		PubKey:              fmt.Sprintf("%x", data.PubKey),
		RequiredFlips:       data.RequiredFlips,
		AvailableFlips:      data.GetMaximumAvailableFlips(),
		FlipKeyWordPairs:    convertedFlipKeyWordPairs,
		MadeFlips:           uint8(len(data.Flips)),
		QualifiedFlips:      totalFlips,
		ShortFlipPoints:     totalPoints,
		Flips:               result,
		Generation:          data.Generation,
		Code:                data.Code,
		Invitees:            invitees,
		Penalty:             blockchain.ConvertToFloat(data.Penalty),
		LastValidationFlags: flags,
		Delegatee:           delegatee,
		DelegationEpoch:     data.DelegationEpoch,
		DelegationNonce:     data.DelegationNonce,
		PendingUndelegation: pendingUndelegation,
		Online:              isOnline,
		IsPool:              appState.ValidatorsCache.IsPool(address),
		Inviter:             inviter,
		ShardId:             uint32(data.ShiftedShardId()),
	}
}

type Epoch struct {
	StartBlock     uint64    `json:"startBlock"`
	Epoch          uint16    `json:"epoch"`
	NextValidation time.Time `json:"nextValidation"`
	CurrentPeriod  string    `json:"currentPeriod"`
}

func (api *DnaApi) Epoch() Epoch {
	s := api.baseApi.getReadonlyAppState()
	var res string
	switch s.State.ValidationPeriod() {
	case state.NonePeriod:
		res = "None"
	case state.FlipLotteryPeriod:
		res = "FlipLottery"
		if api.ceremony.ShortSessionStarted() {
			res = "ShortSession"
		}
	case state.ShortSessionPeriod:
		res = "ShortSession"
	case state.LongSessionPeriod:
		res = "LongSession"
	case state.AfterLongSessionPeriod:
		res = "AfterLongSession"
	}

	return Epoch{
		Epoch:          s.State.Epoch(),
		StartBlock:     s.State.EpochBlock(),
		NextValidation: s.State.NextValidationTime(),
		CurrentPeriod:  res,
	}
}

type CeremonyIntervals struct {
	FlipLotteryDuration  float64
	ShortSessionDuration float64
	LongSessionDuration  float64
}

func (api *DnaApi) CeremonyIntervals() CeremonyIntervals {
	cfg := api.bc.Config()
	networkSize := api.baseApi.getReadonlyAppState().ValidatorsCache.NetworkSize()

	return CeremonyIntervals{
		FlipLotteryDuration:  cfg.Validation.GetFlipLotteryDuration().Seconds(),
		ShortSessionDuration: cfg.Validation.GetShortSessionDuration().Seconds(),
		LongSessionDuration:  cfg.Validation.GetLongSessionDuration(networkSize).Seconds(),
	}
}

func (api *DnaApi) ExportKey(password string) (string, error) {
	if password == "" {
		return "", errors.New("password should not be empty")
	}
	return api.baseApi.secStore.ExportKey(password)
}

type ImportKeyArgs struct {
	Key      string `json:"key"`
	Password string `json:"password"`
}

func (api *DnaApi) ImportKey(args ImportKeyArgs) error {
	return api.bc.Config().ProvideNodeKey(args.Key, args.Password, true)
}

func (api *DnaApi) Version() string {
	return api.appVersion
}

func (api *DnaApi) MinimalClientVersion() string {
	return "0.25.0"
}

type BurnArgs struct {
	From   common.Address  `json:"from"`
	Key    string          `json:"key"`
	Amount decimal.Decimal `json:"amount"`
	MaxFee decimal.Decimal `json:"maxFee"`
	BaseTxArgs
}

func (api *DnaApi) Burn(ctx context.Context, args BurnArgs) (common.Hash, error) {
	from := api.baseApi.getCurrentCoinbase()
	hash, err := api.baseApi.sendTx(ctx, from, nil, types.BurnTx, args.Amount, args.MaxFee, decimal.Zero, args.Nonce,
		args.Epoch, attachments.CreateBurnAttachment(args.Key), nil)

	if err != nil {
		return common.Hash{}, err
	}

	return hash, nil
}

type ChangeProfileArgs struct {
	Info     *hexutil.Bytes  `json:"info"`
	Nickname string          `json:"nickname"`
	MaxFee   decimal.Decimal `json:"maxFee"`
}

type SendChangeProfileTxArgs struct {
	Sender *common.Address `json:"sender"`
	Cid    string          `json:"cid"`
	MaxFee decimal.Decimal `json:"maxFee"`
}

type ChangeProfileResponse struct {
	TxHash common.Hash `json:"txHash"`
	Hash   string      `json:"hash"`
}

type ProfileResponse struct {
	Info     *hexutil.Bytes `json:"info"`
	Nickname string         `json:"nickname"`
}

// Deprecated: Use SendChangeProfileTx
func (api *DnaApi) ChangeProfile(ctx context.Context, args ChangeProfileArgs) (ChangeProfileResponse, error) {
	profileData := profile.Profile{}
	if args.Info != nil && len(*args.Info) > 0 {
		profileData.Info = *args.Info
	}
	if len(args.Nickname) > 0 {
		profileData.Nickname = []byte(args.Nickname)
	}

	profileHash, err := api.profileManager.AddProfile(profileData)

	if err != nil {
		return ChangeProfileResponse{}, errors.Wrap(err, "failed to add profile data")
	}

	txHash, err := api.baseApi.sendTx(ctx, api.baseApi.getCurrentCoinbase(), nil, types.ChangeProfileTx, decimal.Zero,
		args.MaxFee, decimal.Zero, 0, 0, attachments.CreateChangeProfileAttachment(profileHash),
		nil)

	if err != nil {
		return ChangeProfileResponse{}, errors.Wrap(err, "failed to send tx to change profile")
	}

	c, _ := cid.Cast(profileHash)

	return ChangeProfileResponse{
		TxHash: txHash,
		Hash:   c.String(),
	}, nil
}

// Deprecated: Use Identity
func (api *DnaApi) Profile(address *common.Address) (ProfileResponse, error) {
	if address == nil {
		coinbase := api.GetCoinbaseAddr()
		address = &coinbase
	}
	identity := api.baseApi.getReadonlyAppState().State.GetIdentity(*address)
	if len(identity.ProfileHash) == 0 {
		return ProfileResponse{}, nil
	}
	identityProfile, err := api.profileManager.GetProfile(identity.ProfileHash)
	if err != nil {
		return ProfileResponse{}, err
	}
	var info *hexutil.Bytes
	if len(identityProfile.Info) > 0 {
		b := hexutil.Bytes(identityProfile.Info)
		info = &b
	}
	return ProfileResponse{
		Nickname: string(identityProfile.Nickname),
		Info:     info,
	}, nil
}

func (api *DnaApi) SendChangeProfileTx(ctx context.Context, args SendChangeProfileTxArgs) (common.Hash, error) {
	var address = args.Sender
	if address == nil {
		coinbase := api.GetCoinbaseAddr()
		address = &coinbase
	}

	if len(args.Cid) == 0 {
		return common.Hash{}, errors.New("cid should be presented")
	}

	identity := api.baseApi.getReadonlyAppState().State.GetIdentity(*address)
	if len(identity.ProfileHash) > 0 {
		_ = api.baseApi.ipfs.Unpin(identity.ProfileHash)
	}

	c, err := cid.Parse(args.Cid)
	if err != nil {
		return common.Hash{}, err
	}
	txHash, err := api.baseApi.sendTx(ctx, *address, nil, types.ChangeProfileTx, decimal.Zero,
		args.MaxFee, decimal.Zero, 0, 0, attachments.CreateChangeProfileAttachment(c.Bytes()), nil)

	if err != nil {
		return common.Hash{}, errors.Wrap(err, "failed to send tx to change profile")
	}

	if err := api.baseApi.ipfs.Pin(c.Bytes()); err != nil {
		return common.Hash{}, err
	}

	return txHash, nil
}

func (api *DnaApi) Sign(value string) hexutil.Bytes {
	hash := signatureHash(value)
	return api.baseApi.secStore.Sign(hash[:])
}

type SignatureAddressArgs struct {
	Value     string
	Signature hexutil.Bytes
}

func (api *DnaApi) SignatureAddress(args SignatureAddressArgs) (common.Address, error) {
	hash := signatureHash(args.Value)
	pubKey, err := crypto.Ecrecover(hash[:], args.Signature)
	if err != nil {
		return common.Address{}, err
	}
	addr, err := crypto.PubKeyBytesToAddress(pubKey)
	if err != nil {
		return common.Address{}, err
	}
	return addr, nil
}

func signatureHash(value string) common.Hash {
	h := crypto.Hash([]byte(value))
	return crypto.Hash(h[:])
}

type ActivateInviteToRandAddrArgs struct {
	Key string `json:"key"`
	BaseTxArgs
}

type ActivateInviteToRandAddrResponse struct {
	Hash    common.Hash    `json:"hash"`
	Address common.Address `json:"address"`
	Key     string         `json:"key"`
}

func (api *DnaApi) ActivateInviteToRandAddr(ctx context.Context, args ActivateInviteToRandAddrArgs) (ActivateInviteToRandAddrResponse, error) {
	b, err := hex.DecodeString(args.Key)
	if err != nil {
		return ActivateInviteToRandAddrResponse{}, err
	}
	var inviteKey *ecdsa.PrivateKey
	if inviteKey, err = crypto.ToECDSA(b); err != nil {
		return ActivateInviteToRandAddrResponse{}, err
	}
	from := crypto.PubkeyToAddress(inviteKey.PublicKey)
	toKey, _ := crypto.GenerateKey()
	payload := crypto.FromECDSAPub(&toKey.PublicKey)
	to := crypto.PubkeyToAddress(toKey.PublicKey)
	hash, err := api.baseApi.sendTx(ctx, from, &to, types.ActivationTx, decimal.Zero, decimal.Zero, decimal.Zero, args.Nonce, args.Epoch, payload, inviteKey)
	if err != nil {
		return ActivateInviteToRandAddrResponse{}, err
	}
	return ActivateInviteToRandAddrResponse{
		Hash:    hash,
		Address: to,
		Key:     hex.EncodeToString(crypto.FromECDSA(toKey)),
	}, nil
}

func (api *DnaApi) IsValidationReady() bool {
	return api.ceremony.IsValidationReady()
}

func (api *DnaApi) WordsSeed() hexutil.Bytes {
	seed := api.baseApi.getReadonlyAppState().State.FlipWordsSeed()
	return seed[:]
}

type GlobalState struct {
	NetworkSize int `json:"networkSize"`
}

func (api *DnaApi) GlobalState() GlobalState {
	return GlobalState{
		NetworkSize: api.baseApi.getReadonlyAppState().ValidatorsCache.NetworkSize(),
	}
}

type SendToIpfsArgs struct {
	Tx   *hexutil.Bytes `json:"tx"`
	Data *hexutil.Bytes `json:"data"`
}

func (api *DnaApi) SendToIpfs(ctx context.Context, args SendToIpfsArgs) (common.Hash, error) {
	if args.Tx == nil || args.Data == nil || len(*args.Data) == 0 {
		return common.Hash{}, errors.New("all fields are required")
	}
	tx := new(types.Transaction)
	if err := tx.FromBytes(*args.Tx); err != nil {
		return common.Hash{}, err
	}
	attachment := attachments.ParseStoreToIpfsAttachment(tx)
	if attachment == nil {
		return common.Hash{}, validation.InvalidPayload
	}
	data := *args.Data
	if uint32(len(data)) != attachment.Size {
		return common.Hash{}, errors.New("sizes mismatch")
	}
	dataCid, err := api.baseApi.ipfs.Cid(data)
	if err != nil {
		return common.Hash{}, errors.Wrap(err, "failed to calculate cid")
	}
	if bytes.Compare(attachment.Cid, dataCid.Bytes()) != 0 {
		return common.Hash{}, errors.New("cids mismatch")
	}
	if err := api.baseApi.txpool.Validate(tx); err != nil {
		return common.Hash{}, errors.Wrap(err, "tx is not valid")
	}
	if _, err = api.baseApi.ipfs.Add(data, false); err != nil {
		return common.Hash{}, errors.Wrap(err, "failed to add data to local storage")
	}
	txHash, err := api.baseApi.sendInternalTx(ctx, tx)
	if err != nil {
		return common.Hash{}, errors.Wrap(err, "failed to broadcast tx")
	}
	return txHash, nil
}
