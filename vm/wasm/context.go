package wasm

import (
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-wasm-binding/lib"
	"math/big"
)

type ContractContext struct {
	tx           *types.Transaction
	caller       common.Address
	originCaller common.Address
	contractAddr common.Address
	payAmount    *big.Int
}

func NewContractContext(tx *types.Transaction) *ContractContext {
	ctx := &ContractContext{tx: tx}
	ctx.caller, _ = types.Sender(tx)
	ctx.originCaller = ctx.caller
	ctx.payAmount = tx.Amount
	if tx.To != nil {
		ctx.contractAddr = *tx.To
	} else {
		ctx.contractAddr = createContractAddr(tx)
	}
	return ctx
}

func (c *ContractContext) Caller() common.Address {
	return c.caller
}

func (c *ContractContext) ContractAddr() common.Address {
	return c.contractAddr
}

func (c *ContractContext) Epoch() uint16 {
	return c.tx.Epoch
}

func (c *ContractContext) Nonce() uint32 {
	return c.tx.AccountNonce
}

func (c *ContractContext) PayAmount() *big.Int {
	return c.payAmount
}

func (c *ContractContext) CreateSubContext(contract lib.Address, amount *big.Int) *ContractContext {
	return &ContractContext{
		tx:           c.tx,
		payAmount:    amount,
		caller:       c.ContractAddr(),
		originCaller: c.originCaller,
		contractAddr: contract,
	}
}

func createContractAddr(tx *types.Transaction) common.Address {
	attach := attachments.ParseDeployContractAttachment(tx)
	if attach == nil {
		return common.Address{}
	}
	return ComputeContractAddrWithUnpackedArgs(attach.Code, attach.Args,  append(common.ToBytes(tx.Epoch), common.ToBytes(tx.AccountNonce)...))
}
