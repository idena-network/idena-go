package env

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/pkg/errors"
	"math/big"
)

type Env interface {
	BlockNumber() uint64
	BlockTimeStamp() int64
	SetValue(ctx CallContext, key []byte, value []byte)
	GetValue(ctx CallContext, key []byte) []byte
	RemoveValue(ctx CallContext, key []byte)
	Deploy(addr common.Address, codeHash common.Hash)
	Send(ctx CallContext, dest common.Address, amount *big.Int) error
	MinFeePerByte() *big.Int
	Balance(address common.Address) *big.Int
	BlockSeed() []byte
	NetworkSize() int
	State(sender common.Address) state.IdentityState
	PubKey(addr common.Address) []byte
	Iterate(ctx CallContext, minKey []byte, maxKey []byte, f func(key []byte, value []byte) bool)
	BurnAll(ctx CallContext)
}

type EnvImp struct {
	state *appstate.AppState
	block *types.Header
}

func NewEnvImp(state *appstate.AppState, block *types.Header) *EnvImp {
	return &EnvImp{state: state, block: block}
}

func (e *EnvImp) Send(ctx CallContext, dest common.Address, amount *big.Int) error {
	balance := e.state.State.GetBalance(ctx.ContractAddr())
	if balance.Cmp(amount) < 0 {
		return errors.New("insufficient flips")
	}
	if amount.Sign() < 0 {
		return errors.New("value must be non-negative")
	}
	e.state.State.SubBalance(ctx.ContractAddr(), amount)
	e.state.State.AddBalance(dest, amount)
	return nil
}

func (e *EnvImp) Deploy(addr common.Address, codeHash common.Hash) {
	e.state.State.DeployContract(addr, codeHash)
}

func (e *EnvImp) BlockTimeStamp() int64 {
	return e.block.Time()
}

func (e *EnvImp) BlockNumber() uint64 {
	return e.block.Height()
}

func (e *EnvImp) SetValue(ctx CallContext, key []byte, value []byte) {
	e.state.State.SetContractValue(ctx.ContractAddr(), key, value)
}

func (e *EnvImp) GetValue(ctx CallContext, key []byte) []byte {
	return e.state.State.GetContractValue(ctx.ContractAddr(), key)
}

func (e *EnvImp) RemoveValue(ctx CallContext, key []byte) {
	e.state.State.RemoveContractValue(ctx.ContractAddr(), key)
}

func (e *EnvImp) MinFeePerByte() *big.Int {
	return e.state.State.FeePerByte()
}

func (e *EnvImp) Balance(address common.Address) *big.Int {
	return e.state.State.GetBalance(address)
}

func (e *EnvImp) BlockSeed() []byte {
	return e.block.Seed().Bytes()
}

func (e *EnvImp) NetworkSize() int {
	return e.state.ValidatorsCache.NetworkSize()
}

func (e *EnvImp) State(sender common.Address) state.IdentityState {
	return e.state.State.GetIdentityState(sender)
}

func (e *EnvImp) PubKey(addr common.Address) []byte {
	return e.state.State.GetIdentity(addr).PubKey
}

func (e *EnvImp) Iterate(ctx CallContext, minKey []byte, maxKey []byte, f func(key []byte, value []byte) (stopped bool)) {
	e.state.State.IterateContractStore(ctx.ContractAddr(), minKey, maxKey, f)
}

func (e *EnvImp) BurnAll(ctx CallContext) {
	e.state.State.SetBalance(ctx.ContractAddr(), common.Big0)
}

type CallContext interface {
	Sender() common.Address
	ContractAddr() common.Address
	Epoch() uint16
	Nonce() uint32
	PayAmount() *big.Int
}

type CallContextImpl struct {
	tx *types.Transaction
}

func (c *CallContextImpl) PayAmount() *big.Int {
	return c.tx.AmountOrZero()
}

func (c *CallContextImpl) Epoch() uint16 {
	return c.tx.Epoch
}

func (c *CallContextImpl) Nonce() uint32 {
	return c.tx.AccountNonce
}

func NewCallContextImpl(tx *types.Transaction) *CallContextImpl {
	return &CallContextImpl{tx: tx}
}

func (c *CallContextImpl) ContractAddr() common.Address {
	return *c.tx.To
}

func (c *CallContextImpl) Sender() common.Address {
	addr, _ := types.Sender(c.tx)
	return addr
}

type DeployContextImpl struct {
	tx *types.Transaction
}

func (d *DeployContextImpl) PayAmount() *big.Int {
	return common.Big0
}

func NewDeployContextImpl(tx *types.Transaction) *DeployContextImpl {
	return &DeployContextImpl{tx: tx}
}

func (d *DeployContextImpl) Epoch() uint16 {
	return d.tx.Epoch
}

func (d *DeployContextImpl) Nonce() uint32 {
	return d.tx.AccountNonce
}

func (d *DeployContextImpl) Sender() common.Address {
	addr, _ := types.Sender(d.tx)
	return addr
}

func (d *DeployContextImpl) ContractAddr() common.Address {
	hash := crypto.Hash(append(append(d.Sender().Bytes(), common.ToBytes(d.tx.Epoch)...), common.ToBytes(d.tx.AccountNonce)...))
	var result common.Address
	result.SetBytes(hash[:])
	return result
}
