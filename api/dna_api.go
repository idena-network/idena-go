package api

import (
	"crypto/ecdsa"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/hexutil"
	"idena-go/consensus"
	"idena-go/core/mempool"
	"idena-go/core/state"
	"idena-go/crypto"
	"idena-go/keystore"
	"idena-go/rlp"
	"math/big"
)

type DnaApi struct {
	engine *consensus.Engine
	txpool *mempool.TxPool
	ks     *keystore.KeyStore
}

func NewDnaApi(engine *consensus.Engine, txpool *mempool.TxPool, ks *keystore.KeyStore) *DnaApi {
	return &DnaApi{engine, txpool, ks}
}

type State struct {
	Name string `json:"name"`
}

func (api *DnaApi) State() State {
	return State{
		Name: api.engine.GetProcess(),
	}
}

func (api *DnaApi) GetCoinbaseAddr() common.Address {
	return crypto.PubkeyToAddress(*api.engine.GetKey().Public().(*ecdsa.PublicKey))
}

type Balance struct {
	Stake   *big.Float `json:"stake"`
	Balance *big.Float `json:"balance"`
}

func (api *DnaApi) GetBalance(address common.Address) Balance {
	state := api.engine.GetAppState()

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
	Nonce   uint64         `json:"nonce"`
	Payload *hexutil.Bytes `json:"payload"`
}

func (api *DnaApi) SendTransaction(args SendTxArgs) (common.Hash, error) {

	tx := types.Transaction{
		AccountNonce: args.Nonce,
		Type:         args.Type,
		To:           &args.To,
		Amount:       convertToInt(args.Amount),
	}

	if tx.AccountNonce == 0 {
		tx.AccountNonce = api.engine.GetAppState().NonceCache.GetNonce(args.From) + 1
	}

	if args.Payload != nil {
		tx.Payload = *args.Payload
	}

	signedTx, err := api.signTransaction(args.From, &tx)

	if err != nil {
		return common.Hash{}, err
	}

	if err := api.txpool.Add(signedTx); err != nil {
		return common.Hash{}, err
	}

	return signedTx.Hash(), nil
}

func (api *DnaApi) signTransaction(from common.Address, tx *types.Transaction) (*types.Transaction, error) {
	if from == api.GetCoinbaseAddr() {
		return types.SignTx(tx, api.engine.GetKey())
	}
	account, err := api.ks.Find(keystore.Account{Address: from})
	if err != nil {
		return nil, err
	}
	return api.ks.SignTx(account, tx)
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
	api.engine.GetAppState().State.IterateIdentities(func(key []byte, value []byte) bool {
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
