package wasm

import (
	"errors"
	"fmt"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/vm/costs"
	"github.com/idena-network/idena-wasm-binding/lib"
)

type WasmVM struct {
	appState *appstate.AppState
	head     *types.Header
}

func (vm *WasmVM) deploy(env *WasmEnv, tx *types.Transaction, limit uint64) (contractAddr common.Address, gasUsed uint64, actionResult []byte, err error) {
	attach := attachments.ParseDeployContractAttachment(tx)
	contractAddr = createContractAddr(tx)
	actionResult = []byte{}
	if attach == nil {
		return contractAddr, limit, actionResult, errors.New("can't parse attachment")
	}
	if len(attach.Code) == 0 {
		return contractAddr, limit, actionResult, errors.New("code is empty")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
			gasUsed = limit
		}
	}()
	gasUsed, actionResult, err = lib.Deploy(lib.NewGoAPI(env, &lib.GasMeter{}), attach.Code, attach.Args, limit)
	if err == nil {
		env.Deploy(attach.Code)
	}
	return contractAddr, gasUsed, actionResult, err
}

func (vm *WasmVM) call(env *WasmEnv, tx *types.Transaction, limit uint64) (contract common.Address, gasUsed uint64, actionResult []byte, method string, err error) {
	contract = *tx.To
	code := vm.appState.State.GetContractCode(contract)
	actionResult = []byte{}

	if len(code) == 0 {
		return contract, limit, actionResult, "", errors.New("code is empty")
	}
	attach := attachments.ParseCallContractAttachment(tx)
	if attach == nil {
		return contract, limit, actionResult, "", errors.New("can't parse attachment")
	}
	method = attach.Method
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
			gasUsed = limit
		}
	}()
	gasUsed, actionResult, err = lib.Execute(lib.NewGoAPI(env, &lib.GasMeter{}), code, attach.Method, attach.Args, limit)
	return *tx.To, gasUsed, actionResult, attach.Method, err
}

func (vm *WasmVM) Run(tx *types.Transaction, gasLimit uint64) *types.TxReceipt {
	ctx := NewContractContext(tx)
	env := NewWasmEnv(vm.appState, ctx, vm.head)

	var usedGas uint64
	var err error
	var contract common.Address
	var actionResult []byte
	var method string

	sender, _ := types.Sender(tx)

	switch tx.Type {
	case types.DeployContractTx:
		method = "deploy"
		contract, usedGas, actionResult, err = vm.deploy(env, tx, gasLimit)
	case types.CallContractTx:
		contract, usedGas, actionResult, method, err = vm.call(env, tx, gasLimit)
	}
	var events []*types.TxEvent
	if err == nil {
		events = env.InternalCommit()
	}

	if gasLimit >= 0 {
		usedGas = math.Min(usedGas, gasLimit)
	}

	usedGas = costs.WasmGasToGas(usedGas)

	return &types.TxReceipt{
		GasUsed:         usedGas,
		TxHash:          tx.Hash(),
		Error:           err,
		Success:         err == nil,
		From:            sender,
		ContractAddress: contract,
		Events:          events,
		Method:          method,
		ActionResult:    actionResult,
	}
}

func NewWasmVM(appState *appstate.AppState, head *types.Header) *WasmVM {
	return &WasmVM{appState: appState, head: head}
}
