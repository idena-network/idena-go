package api

import (
	"crypto/ecdsa"
	"encoding/hex"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/hexutil"
	"idena-go/core/state"
	"idena-go/crypto"
	"idena-go/rlp"
	"math/big"
)

type DnaApi struct {
	bc      *blockchain.Blockchain
	baseApi *BaseApi
}

func NewDnaApi(baseApi *BaseApi, bc *blockchain.Blockchain) *DnaApi {
	return &DnaApi{bc, baseApi}
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
	Stake   *big.Float `json:"stake"`
	Balance *big.Float `json:"balance"`
}

func (api *DnaApi) GetBalance(address common.Address) Balance {
	state := api.baseApi.engine.GetAppState()

	return Balance{
		Stake:   convertToFloat(state.State.GetStakeBalance(address)),
		Balance: convertToFloat(state.State.GetBalance(address)),
	}
}

// SendTxArgs represents the arguments to sumbit a new transaction into the transaction pool.
type SendTxArgs struct {
	Type    types.TxType   `json:"type"`
	From    common.Address `json:"from"`
	To      common.Address `json:"to"`
	Amount  *big.Float     `json:"amount"`
	Nonce   uint32         `json:"nonce"`
	Epoch   uint16         `json:"epoch"`
	Payload *hexutil.Bytes `json:"payload"`
}

// SendInviteArgs represents the arguments to send invite
type SendInviteArgs struct {
	To     common.Address `json:"to"`
	Amount *big.Float     `json:"amount"`
	Nonce  uint32         `json:"nonce"`
	Epoch  uint16         `json:"epoch"`
}

type ActivateInviteArgs struct {
	Key   string         `json:"key"`
	To    common.Address `json:"to"`
	Nonce uint32         `json:"nonce"`
	Epoch uint16         `json:"epoch"`
}

type Invite struct {
	Hash     common.Hash    `json:"hash"`
	Receiver common.Address `json:"receiver"`
	Key      string         `json:"key"`
}

func (api *DnaApi) SendInvite(args SendInviteArgs) (Invite, error) {
	receiver := args.To
	var key *ecdsa.PrivateKey

	if receiver == (common.Address{}) {
		key, _ = crypto.GenerateKey()
		receiver = crypto.PubkeyToAddress(key.PublicKey)
	}

	hash, err := api.baseApi.sendTx(api.baseApi.getCurrentCoinbase(), receiver, types.InviteTx, args.Amount, args.Nonce, args.Epoch, nil, key)

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

func (api *DnaApi) ActivateInvite(args ActivateInviteArgs) (common.Hash, error) {
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

	hash, err := api.baseApi.sendTx(from, args.To, types.ActivationTx, nil, args.Nonce, args.Epoch, nil, key)

	if err != nil {
		return common.Hash{}, err
	}

	return hash, nil
}

func (api *DnaApi) SendTransaction(args SendTxArgs) (common.Hash, error) {

	var payload []byte
	if args.Payload != nil {
		payload = *args.Payload
	}

	return api.baseApi.sendTx(args.From, args.To, args.Type, args.Amount, args.Nonce, args.Epoch, payload, nil)
}

type Identity struct {
	Address  common.Address `json:"address"`
	Nickname string         `json:"nickname"`
	Stake    *big.Float     `json:"stake"`
	Invites  uint8          `json:"invites"`
	Age      uint16         `json:"age"`
	State    string         `json:"state"`
}

func (api *DnaApi) Identities() []Identity {
	var identities []Identity
	api.baseApi.engine.GetAppState().State.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])

		var data state.Identity
		if err := rlp.DecodeBytes(value, &data); err != nil {
			return false
		}

		var s string
		switch data.State {
		case state.Invite:
			s = "Invite"
			break
		case state.Candidate:
			s = "Candidate"
			break
		case state.Verified:
			s = "Verified"
			break
		case state.Suspended:
			s = "Suspended"
			break
		case state.Killed:
			s = "Killed"
			break
		default:
			s = "Undefined"
			break
		}

		var nickname string
		if data.Nickname != nil {
			nickname = string(data.Nickname[:])
		}

		identities = append(identities, Identity{
			Address:  addr,
			State:    s,
			Stake:    convertToFloat(data.Stake),
			Age:      data.Age,
			Invites:  data.Invites,
			Nickname: nickname,
		})

		return false
	})

	return identities
}

type Block struct {
	Hash   common.Hash
	Height uint64
}

func (api *DnaApi) LastBlock() Block {
	lastBlock := api.bc.Head
	return Block{
		Hash:   lastBlock.Hash(),
		Height: lastBlock.Height(),
	}
}

type Epoch struct {
	Epoch          uint16
	NextEpochBlock uint64
}

func (api *DnaApi) Epoch() Epoch {
	s := api.baseApi.engine.GetAppState()

	return Epoch{
		Epoch:          s.State.Epoch(),
		NextEpochBlock: s.State.NextEpochBlock(),
	}
}

func convertToInt(amount *big.Float) *big.Int {
	if amount == nil {
		return nil
	}
	initial := new(big.Float).SetInt(common.DnaBase)
	result, _ := new(big.Float).Mul(initial, amount).Int(nil)

	return result
}

func convertToFloat(amount *big.Int) *big.Float {
	if amount == nil {
		return nil
	}
	bigAmount := new(big.Float).SetInt(amount)
	result := new(big.Float).Quo(bigAmount, new(big.Float).SetInt(common.DnaBase))

	return result
}
