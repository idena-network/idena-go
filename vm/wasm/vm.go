package wasm

import (
	"errors"
	"fmt"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/vm/costs"
	"github.com/idena-network/idena-wasm-binding/lib"
)

type WasmVM struct {
	appState            *appstate.AppState
	blockHeaderProvider BlockHeaderProvider
	head                *types.Header
	isDebug             bool
}

func (vm *WasmVM) deploy(tx *types.Transaction, limit uint64) (env *WasmEnv, gasUsed uint64, actionResult []byte, err error) {
	ctx := NewContractContext(tx)
	env = NewWasmEnv(vm.appState, vm.blockHeaderProvider, ctx, vm.head, "deploy", vm.isDebug)
	attach := attachments.ParseDeployContractAttachment(tx)
	actionResult = []byte{}
	if attach == nil {
		return env, limit, actionResult, errors.New("can't parse attachment")
	}
	if len(attach.Code) == 0 {
		return env, limit, actionResult, errors.New("code is empty")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
			gasUsed = limit
		}
	}()
	env.Deploy(attach.Code)
	gasUsed, actionResult, err = lib.Deploy(lib.NewGoAPI(env, &lib.GasMeter{}), attach.Code, attach.Args, ctx.ContractAddr(), limit, vm.isDebug)
	return env, gasUsed, actionResult, err
}

func (vm *WasmVM) call(tx *types.Transaction, limit uint64) (env *WasmEnv, gasUsed uint64, actionResult []byte, method string, err error) {
	attachment := attachments.ParseCallContractAttachment(tx)
	method = ""
	if attachment != nil {
		method = attachment.Method
	}
	ctx := NewContractContext(tx)
	env = NewWasmEnv(vm.appState, vm.blockHeaderProvider, ctx, vm.head, method, vm.isDebug)
	contract := *tx.To
	code := vm.appState.State.GetContractCode(contract)
	actionResult = []byte{}

	if len(code) == 0 {
		return env, limit, actionResult, method, errors.New("code is empty")
	}
	if attachment == nil {
		return env, limit, actionResult, method, errors.New("can't parse attachment")
	}
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
			gasUsed = limit
		}
	}()
	gasUsed, actionResult, err = lib.Execute(lib.NewGoAPI(env, &lib.GasMeter{}), code, attachment.Method, attachment.Args, contract, limit, vm.isDebug)
	return env, gasUsed, actionResult, attachment.Method, err
}

func (vm *WasmVM) Run(tx *types.Transaction, wasmGasLimit uint64, commitToState bool) *types.TxReceipt {
	var usedGas uint64
	var err error
	var env *WasmEnv
	var actionResult []byte
	var method string

	sender, _ := types.Sender(tx)

	switch tx.Type {
	case types.DeployContractTx:
		method = "deploy"
		env, usedGas, actionResult, err = vm.deploy(tx, wasmGasLimit)
	case types.CallContractTx:
		env, usedGas, actionResult, method, err = vm.call(tx, wasmGasLimit)
	}
	var events []*types.TxEvent
	if err == nil && commitToState {
		events = env.InternalCommit()
	}

	if wasmGasLimit >= 0 {
		usedGas = math.Min(usedGas, wasmGasLimit)
	}

	usedGas = costs.WasmGasToGas(usedGas)

	return &types.TxReceipt{
		GasUsed:         usedGas,
		TxHash:          tx.Hash(),
		Error:           err,
		Success:         err == nil,
		From:            sender,
		ContractAddress: env.ContractAddress(nil),
		Events:          events,
		Method:          method,
		ActionResult:    actionResult,
	}
}

func NewWasmVM(appState *appstate.AppState, blockHeaderProvider BlockHeaderProvider, head *types.Header, isDebug bool) *WasmVM {
	return &WasmVM{appState: appState, blockHeaderProvider: blockHeaderProvider, head: head, isDebug: isDebug}
}
