package vm

import (
	"fmt"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/idena-network/idena-go/vm/embedded"
	env2 "github.com/idena-network/idena-go/vm/env"
	"github.com/pkg/errors"
)

var (
	UnexpectedTx = errors.New("unexpected tx type")
)

type VM interface {
	Run(tx *types.Transaction, gasLimit int64) *types.TxReceipt
	Read(contractAddr common.Address, method string, args ...[]byte) ([]byte, error)
}

type VmImpl struct {
	env            *env2.EnvImp
	appState       *appstate.AppState
	gasCounter     *env2.GasCounter
	statsCollector collector.StatsCollector
	cfg            *config.Config
}

type VmCreator = func(appState *appstate.AppState, block *types.Header, store *secstore.SecStore, statsCollector collector.StatsCollector, cfg *config.Config) VM

func NewVmImpl(appState *appstate.AppState, block *types.Header, store *secstore.SecStore, statsCollector collector.StatsCollector, cfg *config.Config) VM {
	gasCounter := new(env2.GasCounter)
	return &VmImpl{env: env2.NewEnvImp(appState, block, gasCounter, store, statsCollector), appState: appState, gasCounter: gasCounter,
		statsCollector: statsCollector, cfg: cfg}
}

func (vm *VmImpl) createContract(ctx env2.CallContext) embedded.Contract {
	switch ctx.CodeHash() {
	case embedded.TimeLockContract:
		return embedded.NewTimeLock(ctx, vm.env, vm.statsCollector)
	case embedded.OracleVotingContract:
		return embedded.NewOracleVotingContract3(ctx, vm.env, vm.statsCollector)
	case embedded.OracleLockContract:
		return embedded.NewOracleLock2(ctx, vm.env, vm.statsCollector)
	case embedded.RefundableOracleLockContract:
		return embedded.NewRefundableOracleLock2(ctx, vm.env, vm.statsCollector)
	case embedded.MultisigContract:
		return embedded.NewMultisig(ctx, vm.env, vm.statsCollector)
	default:
		return nil
	}
}

func (vm *VmImpl) deploy(tx *types.Transaction) (addr common.Address, err error) {
	attach := attachments.ParseDeployContractAttachment(tx)
	ctx := env2.NewDeployContextImpl(tx, attach.CodeHash)
	addr = ctx.ContractAddr()
	if attach == nil {
		return addr, errors.New("can't parse attachment")
	}
	contract := vm.createContract(ctx)
	if contract == nil {
		return addr, errors.New("unknown contract")
	}
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()
	err = contract.Deploy(attach.Args...)
	vm.env.Deploy(ctx)
	return addr, err
}

func (vm *VmImpl) call(tx *types.Transaction) (addr common.Address, method string, err error) {

	codeHash := vm.appState.State.GetCodeHash(*tx.To)
	if codeHash == nil {
		return common.Address{}, "", errors.New("destination is not a contract")
	}
	ctx := env2.NewCallContextImpl(tx, *codeHash)
	attach := attachments.ParseCallContractAttachment(tx)
	if attach == nil {
		return ctx.ContractAddr(), "", errors.New("can't parse attachment")
	}
	contract := vm.createContract(ctx)
	addr = ctx.ContractAddr()
	if contract == nil {
		return addr, "", errors.New("unknown contract")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()
	err = contract.Call(attach.Method, attach.Args...)
	return addr, attach.Method, err
}

func (vm *VmImpl) terminate(tx *types.Transaction) (addr common.Address, err error) {
	ctx := env2.NewCallContextImpl(tx, *vm.appState.State.GetCodeHash(*tx.To))
	attach := attachments.ParseTerminateContractAttachment(tx)
	if attach == nil {
		return ctx.ContractAddr(), errors.New("can't parse attachment")
	}
	contract := vm.createContract(ctx)
	addr = ctx.ContractAddr()
	if contract == nil {
		return addr, errors.New("unknown contract")
	}
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()
	var stakeDest common.Address
	stakeDest, err = contract.Terminate(attach.Args...)
	if err == nil {
		vm.env.Terminate(ctx, stakeDest)
	}
	return addr, err
}

func (vm *VmImpl) Run(tx *types.Transaction, gasLimit int64) *types.TxReceipt {
	if tx.Type != types.CallContractTx && tx.Type != types.DeployContractTx && tx.Type != types.TerminateContractTx {
		return &types.TxReceipt{Success: false, Error: UnexpectedTx}
	}

	vm.gasCounter.Reset(int(gasLimit))
	vm.env.Reset()

	var err error
	var contractAddr common.Address
	var method string
	switch tx.Type {
	case types.DeployContractTx:
		method = "deploy"
		contractAddr, err = vm.deploy(tx)
	case types.CallContractTx:
		contractAddr, method, err = vm.call(tx)
	case types.TerminateContractTx:
		method = "terminate"
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
		Method:          method,
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
	contract := vm.createContract(&env2.ReadContextImpl{Contract: contractAddr, Hash: *vm.appState.State.GetCodeHash(contractAddr)})
	if contract == nil {
		return nil, errors.New("unknown contract")
	}
	vm.gasCounter.Reset(-1)
	data, err = contract.Read(method, args...)
	return data, err
}
