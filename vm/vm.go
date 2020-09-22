package vm

import (
	"fmt"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/vm/embedded"
	env2 "github.com/idena-network/idena-go/vm/env"
	"github.com/pkg/errors"
)

var (
	UnexpectedTx = errors.New("unexpected tx type")
)

type VM interface {
	Run(tx *types.Transaction, gasLimit int64) *types.TxReceipt
}

type VmImpl struct {
	env        *env2.EnvImp
	appState   *appstate.AppState
	gasCounter *env2.GasCounter
}

func NewVmImpl(appState *appstate.AppState, block *types.Header, store *secstore.SecStore) *VmImpl {
	gasCounter := new(env2.GasCounter)
	return &VmImpl{env: env2.NewEnvImp(appState, block, gasCounter, store), appState: appState, gasCounter: gasCounter}
}

func (vm *VmImpl) createContract(ctx env2.CallContext, codeHash common.Hash) embedded.Contract {
	switch codeHash {
	case embedded.TimeLockContract:
		return embedded.NewTimeLock(ctx, vm.env)
	case embedded.FactEvidenceContract:
		return embedded.NewFactEvidenceContract(ctx, vm.env)
	case embedded.EvidenceLockContract:
		return embedded.NewEvidenceLock(ctx, vm.env)
	case embedded.RefundableEvidenceLockContract:
		return embedded.NewRefundableEvidenceLock(ctx, vm.env)
	case embedded.MultisigContract:
		return embedded.NewMultisig(ctx, vm.env)
	default:
		return nil
	}
}

func (vm *VmImpl) deploy(tx *types.Transaction) (addr common.Address, err error) {
	ctx := env2.NewDeployContextImpl(tx)
	attach := attachments.ParseDeployContractAttachment(tx)
	addr = ctx.ContractAddr()
	if attach == nil {
		return addr, errors.New("can't parse attachment")
	}
	contract := vm.createContract(ctx, attach.CodeHash)
	if contract == nil {
		return addr, errors.New("unknown contract")
	}
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()
	err = contract.Deploy(attach.Args...)
	return addr, err
}

func (vm *VmImpl) call(tx *types.Transaction) (addr common.Address, err error) {
	ctx := env2.NewCallContextImpl(tx)
	attach := attachments.ParseCallContractAttachment(tx)
	if attach == nil {
		return ctx.ContractAddr(), errors.New("can't parse attachment")
	}
	contract := vm.createContract(ctx, *vm.appState.State.GetCodeHash(*tx.To))
	addr = ctx.ContractAddr()
	if contract == nil {
		return addr, errors.New("unknown contract")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()
	err = contract.Call(attach.Method, attach.Args...)
	return addr, err
}

func (vm *VmImpl) terminate(tx *types.Transaction) (addr common.Address, err error) {
	ctx := env2.NewCallContextImpl(tx)
	attach := attachments.ParseTerminateContractAttachment(tx)
	if attach == nil {
		return ctx.ContractAddr(), errors.New("can't parse attachment")
	}
	contract := vm.createContract(ctx, *vm.appState.State.GetCodeHash(*tx.To))
	addr = ctx.ContractAddr()
	if contract == nil {
		return addr, errors.New("unknown contract")
	}
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()
	err = contract.Terminate(attach.Args...)
	return addr, err
}

func (vm *VmImpl) Run(tx *types.Transaction, gasLimit int64) *types.TxReceipt {
	if tx.Type != types.CallContract && tx.Type != types.DeployContract && tx.Type != types.TerminateContract {
		return &types.TxReceipt{Success: false, Error: UnexpectedTx}
	}

	vm.gasCounter.Reset(int(gasLimit))
	vm.env.Reset()

	var err error
	var contractAddr common.Address
	switch tx.Type {
	case types.DeployContract:
		contractAddr, err = vm.deploy(tx)
	case types.CallContract:
		contractAddr, err = vm.call(tx)
	case types.TerminateContract:
		contractAddr, err = vm.terminate(tx)
	}

	var events []*types.TxEvent
	if err == nil {
		events = vm.env.Commit()
	}

	sender, _ := types.Sender(tx)

	usedGas := uint64(vm.gasCounter.UsedGas)
	if gasLimit >= 0 {
		usedGas = math.Min(usedGas, uint64(gasLimit))
	}

	return &types.TxReceipt{
		GasUsed:         usedGas,
		TxHash:          tx.Hash(),
		Error:           err,
		Success:         err == nil,
		From:            sender,
		ContractAddress: contractAddr,
		Events:          events,
	}
}

func (vm *VmImpl) Read(contractAddr common.Address, method string, args ...[]byte) ([]byte, error) {
	var data []byte
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()
	contract := vm.createContract(&env2.ReadContextImpl{Contract: contractAddr}, *vm.appState.State.GetCodeHash(contractAddr))
	if contract == nil {
		return nil, errors.New("unknown contract")
	}
	vm.gasCounter.Reset(-1)
	data, err = contract.Read(method, args...)
	return data, err
}
