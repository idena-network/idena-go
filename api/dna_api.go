package api

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/ceremony"
	"github.com/idena-network/idena-go/core/profile"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/deferredtx"
	"github.com/idena-network/idena-go/vm"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"math/big"
	"strconv"
	"time"
)

type DnaApi struct {
	bc             *blockchain.Blockchain
	baseApi        *BaseApi
	ceremony       *ceremony.ValidationCeremony
	appVersion     string
	profileManager *profile.Manager
	deferredTxs    *deferredtx.Job
}

func NewDnaApi(baseApi *BaseApi, bc *blockchain.Blockchain, ceremony *ceremony.ValidationCeremony, appVersion string,
	profileManager *profile.Manager, deferredTxs *deferredtx.Job) *DnaApi {
	return &DnaApi{bc, baseApi, ceremony, appVersion, profileManager, deferredTxs}
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
	currentEpoch := state.State.Epoch()
	nonce, epoch := state.State.GetNonce(address), state.State.GetEpoch(address)
	if epoch < currentEpoch {
		nonce = 0
	}

	return Balance{
		Stake:   blockchain.ConvertToFloat(state.State.GetStakeBalance(address)),
		Balance: blockchain.ConvertToFloat(state.State.GetBalance(address)),
		Nonce:   nonce,
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
		if err := data.FromBytes(value); err != nil {
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

	totalPoints, totalFlips := common.CalculateIdentityScores(data.Scores, data.GetShortFlipPoints(), data.QualifiedFlips)

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
	FlipLotteryDuration  float64
	ShortSessionDuration float64
	LongSessionDuration  float64
}

func (api *DnaApi) CeremonyIntervals() CeremonyIntervals {
	cfg := api.bc.Config()
	networkSize := api.baseApi.getAppState().ValidatorsCache.NetworkSize()

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

type DeployContractArgs struct {
	From     common.Address  `json:"from"`
	CodeHash hexutil.Bytes   `json:"codeHash"`
	Amount   decimal.Decimal `json:"amount"`
	Args     DynamicArgs     `json:"args"`
	MaxFee   decimal.Decimal `json:"maxFee"`
}

type CallContractArgs struct {
	From           common.Address  `json:"from"`
	Contract       common.Address  `json:"contract"`
	Method         string          `json:"method"`
	Amount         decimal.Decimal `json:"amount"`
	Args           DynamicArgs     `json:"args"`
	MaxFee         decimal.Decimal `json:"maxFee"`
	BroadcastBlock uint64          `json:"broadcastBlock"`
}

type TerminateContractArgs struct {
	From     common.Address  `json:"from"`
	Contract common.Address  `json:"contract"`
	Args     DynamicArgs     `json:"args"`
	MaxFee   decimal.Decimal `json:"maxFee"`
}

type DynamicArgs []*DynamicArg

type DynamicArg struct {
	Index  int    `json:"index"`
	Format string `json:"format"`
	Value  string `json:"value"`
}

type ReadonlyCallContractArgs struct {
	Contract common.Address `json:"contract"`
	Method   string         `json:"method"`
	Format   string         `json:"format"`
	Args     DynamicArgs    `json:"args"`
}

func (a DynamicArg) ToBytes() []byte {
	switch a.Format {
	case "byte":
		i, err := strconv.ParseInt(a.Value, 10, 8)
		if err != nil {
			return nil
		}
		return []byte{byte(i)}
	case "uint64":
		i, err := strconv.ParseInt(a.Value, 10, 64)
		if err != nil {
			return nil
		}
		return common.ToBytes(uint64(i))
	case "string":
		return []byte(a.Value)
	case "bigint":
		v := new(big.Int)
		v.SetString(a.Value, 10)
		return v.Bytes()
	case "hex":
		data, err := hexutil.Decode(a.Value)
		if err != nil {
			return nil
		}
		return data
	case "dna":
		d, err := decimal.NewFromString(a.Value)
		if err != nil {
			return nil
		}
		return blockchain.ConvertToInt(d).Bytes()
	default:
		data, err := hexutil.Decode(a.Value)
		if err != nil {
			return nil
		}
		return data
	}
}

func (d DynamicArgs) ToSlice() [][]byte {

	m := make(map[int]*DynamicArg)
	maxIndex := -1
	for _, a := range d {
		m[a.Index] = a
		if a.Index > maxIndex {
			maxIndex = a.Index
		}
	}
	var data [][]byte

	for i := 0; i <= maxIndex; i++ {
		if a, ok := m[i]; ok {
			data = append(data, a.ToBytes())
		} else {
			data = append(data, nil)
		}
	}
	return data
}

type TxReceipt struct {
	Contract common.Address  `json:"contract"`
	Success  bool            `json:"success"`
	GasUsed  uint64          `json:"gasUsed"`
	TxHash   common.Hash     `json:"txHash"`
	Error    string          `json:"error"`
	GasCost  decimal.Decimal `json:"gasCost"`
	TxFee    decimal.Decimal `json:"txFee"`
}

func (api *DnaApi) buildDeployContractTx(args DeployContractArgs) (*types.Transaction, error) {
	var codeHash common.Hash
	codeHash.SetBytes(args.CodeHash)

	from := args.From
	if from == (common.Address{}) {
		from = api.GetCoinbaseAddr()
	}

	payload, _ := attachments.CreateDeployContractAttachment(codeHash, args.Args.ToSlice()...).ToBytes()
	return api.baseApi.getSignedTx(from, nil, types.DeployContract, args.Amount,
		args.MaxFee, decimal.Zero,
		0, 0,
		payload, nil)
}

func (api *DnaApi) buildCallContractTx(args CallContractArgs) (*types.Transaction, error) {

	from := args.From
	if from == (common.Address{}) {
		from = api.GetCoinbaseAddr()
	}

	payload, _ := attachments.CreateCallContractAttachment(args.Method, args.Args.ToSlice()...).ToBytes()
	return api.baseApi.getSignedTx(from, &args.Contract, types.CallContract, args.Amount,
		args.MaxFee, decimal.Zero,
		0, 0,
		payload, nil)
}

func (api *DnaApi) buildTerminateContractTx(args TerminateContractArgs) (*types.Transaction, error) {

	from := args.From
	if from == (common.Address{}) {
		from = api.GetCoinbaseAddr()
	}

	payload, _ := attachments.CreateTerminateContractAttachment(args.Args.ToSlice()...).ToBytes()
	return api.baseApi.getSignedTx(from, &args.Contract, types.TerminateContract, decimal.Zero,
		args.MaxFee, decimal.Zero,
		0, 0,
		payload, nil)
}

func (api *DnaApi) EstimateDeployContract(args DeployContractArgs) (*TxReceipt, error) {
	appState := api.baseApi.getAppState()
	vm := vm.NewVmImpl(appState, api.bc.Head, api.baseApi.secStore)
	tx, err := api.buildDeployContractTx(args)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateTx(appState, tx, appState.State.FeePerGas(), validation.MempoolTx); err != nil {
		return nil, err
	}
	r := vm.Run(tx, -1)
	r.GasCost = api.bc.GetGasCost(appState, r.GasUsed)
	return convertReceipt(tx, r, appState.State.FeePerGas()), nil
}

func (api *DnaApi) EstimateCallContract(args CallContractArgs) (*TxReceipt, error) {
	appState := api.baseApi.getAppState()
	vm := vm.NewVmImpl(appState, api.bc.Head, api.baseApi.secStore)
	tx, err := api.buildCallContractTx(args)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateTx(appState, tx, appState.State.FeePerGas(), validation.MempoolTx); err != nil {
		return nil, err
	}
	r := vm.Run(tx, -1)
	r.GasCost = api.bc.GetGasCost(appState, r.GasUsed)
	return convertReceipt(tx, r, appState.State.FeePerGas()), nil
}

func (api *DnaApi) EstimateTerminateContract(args TerminateContractArgs) (*TxReceipt, error) {
	appState := api.baseApi.getAppState()
	vm := vm.NewVmImpl(appState, api.bc.Head, api.baseApi.secStore)
	tx, err := api.buildTerminateContractTx(args)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateTx(appState, tx, appState.State.FeePerGas(), validation.MempoolTx); err != nil {
		return nil, err
	}
	r := vm.Run(tx, -1)
	r.GasCost = api.bc.GetGasCost(appState, r.GasUsed)
	return convertReceipt(tx, r, appState.State.FeePerGas()), nil
}

func convertReceipt(tx *types.Transaction, receipt *types.TxReceipt, feePerByte *big.Int) *TxReceipt {
	fee := fee.CalculateFee(1, feePerByte, tx)
	var err string
	if receipt.Error != nil {
		err = receipt.Error.Error()
	}
	return &TxReceipt{
		Success:  receipt.Success,
		Error:    err,
		Contract: receipt.ContractAddress,
		TxHash:   receipt.TxHash,
		GasUsed:  receipt.GasUsed,
		GasCost:  blockchain.ConvertToFloat(receipt.GasCost),
		TxFee:    blockchain.ConvertToFloat(fee),
	}
}

func (api *DnaApi) DeployContract(ctx context.Context, args DeployContractArgs) (common.Hash, error) {
	tx, err := api.buildDeployContractTx(args)
	if err != nil {
		return common.Hash{}, err
	}
	return api.baseApi.sendInternalTx(ctx, tx)
}

func (api *DnaApi) CallContract(ctx context.Context, args CallContractArgs) (common.Hash, error) {
	tx, err := api.buildCallContractTx(args)
	if err != nil {
		return common.Hash{}, err
	}
	if args.BroadcastBlock > 0 {
		err = api.deferredTxs.AddDeferredTx(args.From, &args.Contract, blockchain.ConvertToInt(args.Amount), tx.Payload, common.Big0, args.BroadcastBlock)
		return tx.Hash(), err
	}
	return api.baseApi.sendInternalTx(ctx, tx)
}
func (api *DnaApi) TerminateContract(ctx context.Context, args TerminateContractArgs) (common.Hash, error) {
	tx, err := api.buildTerminateContractTx(args)
	if err != nil {
		return common.Hash{}, err
	}
	return api.baseApi.sendInternalTx(ctx, tx)
}

func (api *DnaApi) ReadContractData(contract common.Address, key string, format string) (interface{}, error) {
	data := api.baseApi.getAppState().State.GetContractValue(contract, []byte(key))
	if data == nil {
		return nil, errors.New("data is nil")
	}
	return conversion(format, data)
}

func (api *DnaApi) ReadonlyCallContract(args ReadonlyCallContractArgs) (interface{}, error) {
	vm := vm.NewVmImpl(api.baseApi.getAppState(), api.bc.Head, api.baseApi.secStore)
	data, err := vm.Read(args.Contract, args.Method, args.Args.ToSlice()...)
	if err != nil {
		return nil, err
	}
	return conversion(args.Format, data)
}

func (api *DnaApi) GetContractData(contract common.Address) interface{} {
	hash := api.baseApi.getAppState().State.GetCodeHash(contract)
	stake := api.baseApi.getAppState().State.GetContractStake(contract)
	return struct {
		Hash  *common.Hash
		Stake decimal.Decimal
	}{
		hash,
		blockchain.ConvertToFloat(stake),
	}
}

func conversion(convertTo string, data []byte) (interface{}, error) {
	switch convertTo {
	case "byte":
		return helpers.ExtractByte(0, data)
	case "uint64":
		return helpers.ExtractUInt64(0, data)
	case "string":
		return string(data), nil
	case "bigint":
		v := new(big.Int)
		v.SetBytes(data)
		return v.String(), nil
	case "hex":
		return hexutil.Encode(data), nil
	case "dna":
		v := new(big.Int)
		v.SetBytes(data)
		return blockchain.ConvertToFloat(v), nil
	default:
		return hexutil.Encode(data), nil
	}
}
