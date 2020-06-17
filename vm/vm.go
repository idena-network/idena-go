package vm

import (
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/vm/embedded"
	env2 "github.com/idena-network/idena-go/vm/env"
	"github.com/pkg/errors"
	"math/big"
)

var (
	UnexpectedTx = errors.New("unexpected tx type")
)

type VM interface {
	Run(tx *types.Transaction) *types.TxReceipt
}

type VmImpl struct {
	env      env2.Env
	appState *appstate.AppState
}

func NewVmImpl(appState *appstate.AppState, block *types.Header) *VmImpl {
	return &VmImpl{env: env2.NewEnvImp(appState, block), appState: appState}
}

func (vm *VmImpl) createContract(ctx env2.CallContext, codeHash common.Hash) embedded.Contract {
	switch codeHash {
	case embedded.TimeLockContract:
		return embedded.NewTimeLock(ctx, vm.env)
	default:
		return nil
	}
}

func (vm *VmImpl) deploy(tx *types.Transaction) (int, common.Address, error) {
	ctx := env2.NewDeployContextImpl(tx)
	attach := attachments.ParseDeployContractAttachment(tx)
	if attach == nil {
		return 0, ctx.ContractAddr(), errors.New("can't parse attachment")
	}
	contract := vm.createContract(ctx, attach.CodeHash)
	if contract == nil {
		return 0, ctx.ContractAddr(), errors.New("unknown contract")
	}
	return embedded.ContractDeployGas, ctx.ContractAddr(), contract.Deploy(attach.Args...)
}

func (vm *VmImpl) call(tx *types.Transaction) (int, common.Address, error) {
	ctx := env2.NewCallContextImpl(tx)
	attach := attachments.ParseCallContractAttachment(tx)
	if attach == nil {
		return 0, ctx.ContractAddr(), errors.New("can't parse attachment")
	}
	contract := vm.createContract(ctx, *vm.appState.State.GetCodeHash(*tx.To))
	if contract == nil {
		return 0, ctx.ContractAddr(), errors.New("unknown contract")
	}
	return embedded.ContractCallGas, ctx.ContractAddr(), contract.Call(attach.Method, attach.Args...)
}

func (vm *VmImpl) Run(tx *types.Transaction) *types.TxReceipt {
	if tx.Type != types.CallContract && tx.Type != types.DeployContract {
		return &types.TxReceipt{Success: false, Error: UnexpectedTx}
	}
	var err error
	var gas int
	var contractAddr common.Address
	switch tx.Type {
	case types.CallContract:
		gas, contractAddr, err = vm.call(tx)
	case types.DeployContract:
		gas, contractAddr, err = vm.deploy(tx)
	}
	sender, _ := types.Sender(tx)
	return &types.TxReceipt{
		GasUsed:         big.NewInt(int64(gas)),
		TxHash:          tx.Hash(),
		Error:           err,
		Success:         err == nil,
		From:            sender,
		ContractAddress: contractAddr,
	}
}
