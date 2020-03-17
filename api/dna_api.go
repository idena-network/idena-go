package api

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/ceremony"
	"github.com/idena-network/idena-go/core/profile"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/rlp"
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
	Stake   decimal.Decimal `json:"stake"`
	Balance decimal.Decimal `json:"balance"`
	Nonce   uint32          `json:"nonce"`
}

func (api *DnaApi) GetBalance(address common.Address) Balance {
	state := api.baseApi.getAppState()

	return Balance{
		Stake:   blockchain.ConvertToFloat(state.State.GetStakeBalance(address)),
		Balance: blockchain.ConvertToFloat(state.State.GetBalance(address)),
		Nonce:   state.State.GetNonce(address),
	}
}

// SendTxArgs represents the arguments to sumbit a new transaction into the transaction pool.
type SendTxArgs struct {
	Type    types.TxType    `json:"type"`
	From    common.Address  `json:"from"`
	To      *common.Address `json:"to"`
	Amount  decimal.Decimal `json:"amount"`
	MaxFee  decimal.Decimal `json:"maxFee"`
	Payload *hexutil.Bytes  `json:"payload"`
	Tips    decimal.Decimal `json:"tips"`
	BaseTxArgs
}

// SendInviteArgs represents the arguments to send invite
type SendInviteArgs struct {
	To     common.Address  `json:"to"`
	Amount decimal.Decimal `json:"amount"`
	BaseTxArgs
}

type ActivateInviteArgs struct {
	Key string          `json:"key"`
	To  *common.Address `json:"to"`
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
	payload := api.baseApi.secStore.GetPubKey()
	to := args.To
	if to == nil {
		coinbase := api.baseApi.getCurrentCoinbase()
		to = &coinbase
	}
	hash, err := api.baseApi.sendTx(ctx, from, to, types.ActivationTx, decimal.Zero, decimal.Zero, decimal.Zero, args.Nonce, args.Epoch, payload, key)

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

func (api *DnaApi) SendTransaction(ctx context.Context, args SendTxArgs) (common.Hash, error) {

	var payload []byte
	if args.Payload != nil {
		payload = *args.Payload
	}

	return api.baseApi.sendTx(ctx, args.From, args.To, args.Type, args.Amount, args.MaxFee, args.Tips, args.Nonce, args.Epoch, payload, nil)
}

type FlipWords struct {
	Words [2]uint32 `json:"words"`
	Used  bool      `json:"used"`
	Id    int       `json:"id"`
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
}

func (api *DnaApi) Identities() []Identity {
	var identities []Identity
	epoch := api.baseApi.getAppState().State.Epoch()
	api.baseApi.getAppState().State.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])

		var data state.Identity
		if err := rlp.DecodeBytes(value, &data); err != nil {
			return false
		}
		var flipKeyWordPairs []int
		if addr == api.GetCoinbaseAddr() {
			flipKeyWordPairs = api.ceremony.FlipKeyWordPairs()
		}
		identities = append(identities, convertIdentity(epoch, addr, data, flipKeyWordPairs))

		return false
	})

	for idx := range identities {
		identities[idx].Online = getIdentityOnlineStatus(api.baseApi.getAppState(), identities[idx].Address)
	}

	return identities
}

func (api *DnaApi) Identity(address *common.Address) Identity {
	var flipKeyWordPairs []int
	coinbase := api.GetCoinbaseAddr()
	if address == nil || *address == coinbase {
		address = &coinbase
		flipKeyWordPairs = api.ceremony.FlipKeyWordPairs()
	}

	converted := convertIdentity(api.baseApi.getAppState().State.Epoch(), *address, api.baseApi.getAppState().State.GetIdentity(*address), flipKeyWordPairs)
	converted.Online = getIdentityOnlineStatus(api.baseApi.getAppState(), *address)
	return converted
}

func getIdentityOnlineStatus(state *appstate.AppState, addr common.Address) bool {
	isOnline := state.ValidatorsCache.IsOnlineIdentity(addr)
	hasPendingStatusSwitch := state.State.HasStatusSwitchAddresses(addr)
	if hasPendingStatusSwitch {
		return !isOnline
	} else {
		return isOnline
	}
}

func convertIdentity(currentEpoch uint16, address common.Address, data state.Identity, flipKeyWordPairs []int) Identity {
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
		QualifiedFlips:      data.QualifiedFlips,
		ShortFlipPoints:     data.GetShortFlipPoints(),
		Flips:               result,
		Generation:          data.Generation,
		Code:                data.Code,
		Invitees:            invitees,
		Penalty:             blockchain.ConvertToFloat(data.Penalty),
		LastValidationFlags: flags,
	}
}

type Epoch struct {
	Epoch                  uint16    `json:"epoch"`
	NextValidation         time.Time `json:"nextValidation"`
	CurrentPeriod          string    `json:"currentPeriod"`
	CurrentValidationStart time.Time `json:"currentValidationStart"`
}

func (api *DnaApi) Epoch() Epoch {
	s := api.baseApi.getAppState()
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
		Epoch:                  s.State.Epoch(),
		NextValidation:         s.State.NextValidationTime(),
		CurrentPeriod:          res,
		CurrentValidationStart: api.ceremony.ShortSessionBeginTime(),
	}
}

type CeremonyIntervals struct {
	FlipLotteryDuration      float64
	ShortSessionDuration     float64
	LongSessionDuration      float64
	AfterLongSessionDuration float64
}

func (api *DnaApi) CeremonyIntervals() CeremonyIntervals {
	cfg := api.bc.Config()
	networkSize := api.baseApi.getAppState().ValidatorsCache.NetworkSize()

	return CeremonyIntervals{
		FlipLotteryDuration:      cfg.Validation.GetFlipLotteryDuration().Seconds(),
		ShortSessionDuration:     cfg.Validation.GetShortSessionDuration().Seconds(),
		LongSessionDuration:      cfg.Validation.GetLongSessionDuration(networkSize).Seconds(),
		AfterLongSessionDuration: cfg.Validation.GetAfterLongSessionDuration().Seconds(),
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

type ChangeProfileResponse struct {
	TxHash common.Hash `json:"txHash"`
	Hash   string      `json:"hash"`
}

type ProfileResponse struct {
	Info     *hexutil.Bytes `json:"info"`
	Nickname string         `json:"nickname"`
}

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

func (api *DnaApi) Profile(address *common.Address) (ProfileResponse, error) {
	if address == nil {
		coinbase := api.GetCoinbaseAddr()
		address = &coinbase
	}
	identity := api.baseApi.getAppState().State.GetIdentity(*address)
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
	return rlp.Hash(value)
}
