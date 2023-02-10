package api

import (
	"context"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/deferredtx"
	"github.com/idena-network/idena-go/subscriptions"
	"github.com/idena-network/idena-go/vm"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	models "github.com/idena-network/idena-wasm-binding/lib/protobuf"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"math/big"
	"strconv"
)

type ContractApi struct {
	baseApi     *BaseApi
	bc          *blockchain.Blockchain
	deferredTxs *deferredtx.Job
	subManager  *subscriptions.Manager
}

// NewContractApi creates a new NetApi instance
func NewContractApi(baseApi *BaseApi, bc *blockchain.Blockchain, deferredTxs *deferredtx.Job, subManager *subscriptions.Manager) *ContractApi {
	return &ContractApi{baseApi: baseApi, bc: bc, deferredTxs: deferredTxs, subManager: subManager}
}

type DeployArgs struct {
	From     common.Address  `json:"from"`
	CodeHash hexutil.Bytes   `json:"codeHash"`
	Amount   decimal.Decimal `json:"amount"`
	Args     DynamicArgs     `json:"args"`
	Nonce    hexutil.Bytes   `json:"nonce"`
	MaxFee   decimal.Decimal `json:"maxFee"`
	Code     hexutil.Bytes   `json:"code"`
}

type CallArgs struct {
	From           common.Address  `json:"from"`
	Contract       common.Address  `json:"contract"`
	Method         string          `json:"method"`
	Amount         decimal.Decimal `json:"amount"`
	Args           DynamicArgs     `json:"args"`
	MaxFee         decimal.Decimal `json:"maxFee"`
	BroadcastBlock uint64          `json:"broadcastBlock"`
}

type TerminateArgs struct {
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

type ReadonlyCallArgs struct {
	Contract common.Address `json:"contract"`
	Method   string         `json:"method"`
	Format   string         `json:"format"`
	Args     DynamicArgs    `json:"args"`
}

type EventsArgs struct {
	Contract common.Address `json:"contract"`
}

type KeyWithFormat struct {
	Key    string `json:"key"`
	Format string `json:"format"`
}

type ContractData struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value,omitempty"`
	Error string      `json:"error,omitempty"`
}

func (a DynamicArg) ToBytes() ([]byte, error) {
	switch a.Format {
	case "byte":
		i, err := strconv.ParseUint(a.Value, 10, 8)
		if err != nil {
			return nil, errors.Errorf("cannot parse byte: \"%v\"", a.Value)
		}
		return []byte{byte(i)}, nil
	case "int8":
		i, err := strconv.ParseInt(a.Value, 10, 8)
		if err != nil {
			return nil, errors.Errorf("cannot parse int8: \"%v\"", a.Value)
		}
		return common.ToBytes(i), nil
	case "uint64":
		i, err := strconv.ParseUint(a.Value, 10, 64)
		if err != nil {
			return nil, errors.Errorf("cannot parse uint64: \"%v\"", a.Value)
		}
		return common.ToBytes(i), nil
	case "int64":
		i, err := strconv.ParseInt(a.Value, 10, 64)
		if err != nil {
			return nil, errors.Errorf("cannot parse int64: \"%v\"", a.Value)
		}
		return common.ToBytes(i), nil
	case "string":
		return []byte(a.Value), nil
	case "bigint":
		v := new(big.Int)
		_, ok := v.SetString(a.Value, 10)
		if !ok {
			return nil, errors.Errorf("cannot parse bigint: \"%v\"", a.Value)
		}
		return v.Bytes(), nil
	case "hex":
		data, err := hexutil.Decode(a.Value)
		if err != nil {
			return nil, errors.Errorf("cannot parse hex: \"%v\"", a.Value)
		}
		return data, nil
	case "dna":
		d, err := decimal.NewFromString(a.Value)
		if err != nil {
			return nil, errors.Errorf("cannot parse dna: \"%v\"", a.Value)
		}
		return blockchain.ConvertToInt(d).Bytes(), nil
	default:
		data, err := hexutil.Decode(a.Value)
		if err != nil {
			return nil, errors.Errorf("cannot parse hex: \"%v\"", a.Value)
		}
		return data, nil
	}
}

func (d DynamicArgs) ToSlice() ([][]byte, error) {

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
			bytes, err := a.ToBytes()
			if err != nil {
				return nil, err
			}
			data = append(data, bytes)
		} else {
			data = append(data, nil)
		}
	}
	return data, nil
}

type TxReceipt struct {
	Contract     common.Address  `json:"contract"`
	Method       string          `json:"method"`
	Success      bool            `json:"success"`
	GasUsed      uint64          `json:"gasUsed"`
	TxHash       *common.Hash    `json:"txHash"`
	Error        string          `json:"error"`
	GasCost      decimal.Decimal `json:"gasCost"`
	TxFee        decimal.Decimal `json:"txFee"`
	ActionResult *ActionResult   `json:"actionResult"`
	Events       []Event         `json:"events"`
}

type ActionResult struct {
	InputAction      InputAction     `json:"inputAction"`
	Success          bool            `json:"success"`
	Error            string          `json:"error"`
	GasUsed          uint64          `json:"gasUsed"`
	RemainingGas     uint64          `json:"remainingGas"`
	OutputData       hexutil.Bytes   `json:"outputData"`
	SubActionResults []*ActionResult `json:"subActionResults"`
}

type InputAction struct {
	ActionType uint32        `json:"actionType"`
	Amount     hexutil.Bytes `json:"amount"`
	Method     string        `json:"method"`
	Args       hexutil.Bytes `json:"args"`
	GasLimit   uint64        `json:"gasLimit"`
}

type Event struct {
	Contract common.Address  `json:"contract"`
	Event    string          `json:"event"`
	Args     []hexutil.Bytes `json:"args"`
}

type MapItem struct {
	Key   interface{} `json:"key"`
	Value interface{} `json:"value"`
}

type IterateMapResponse struct {
	Items             []*MapItem     `json:"items"`
	ContinuationToken *hexutil.Bytes `json:"continuationToken"`
}

func (api *ContractApi) buildDeployContractTx(args DeployArgs, estimate bool) (*types.Transaction, error) {
	var codeHash common.Hash
	codeHash.SetBytes(args.CodeHash)

	from := args.From
	if from == (common.Address{}) {
		from = api.baseApi.getCurrentCoinbase()
	}
	convertedArgs, err := args.Args.ToSlice()
	if err != nil {
		return nil, err
	}
	payload, _ := attachments.CreateDeployContractAttachment(codeHash, args.Code, args.Nonce, convertedArgs...).ToBytes()
	tx := api.baseApi.getTx(from, nil, types.DeployContractTx, args.Amount, args.MaxFee, decimal.Zero, 0, 0, payload)
	return api.signIfNeeded(from, tx, estimate)
}

func (api *ContractApi) buildCallContractTx(args CallArgs, estimate bool) (*types.Transaction, error) {

	from := args.From
	if from == (common.Address{}) {
		from = api.baseApi.getCurrentCoinbase()
	}
	convertedArgs, err := args.Args.ToSlice()
	if err != nil {
		return nil, err
	}
	payload, _ := attachments.CreateCallContractAttachment(args.Method, convertedArgs...).ToBytes()
	tx := api.baseApi.getTx(from, &args.Contract, types.CallContractTx, args.Amount, args.MaxFee, decimal.Zero, 0, 0,
		payload)
	return api.signIfNeeded(from, tx, estimate)
}

func (api *ContractApi) buildTerminateContractTx(args TerminateArgs, estimate bool) (*types.Transaction, error) {

	from := args.From
	if from == (common.Address{}) {
		from = api.baseApi.getCurrentCoinbase()
	}
	convertedArgs, err := args.Args.ToSlice()
	if err != nil {
		return nil, err
	}
	payload, _ := attachments.CreateTerminateContractAttachment(convertedArgs...).ToBytes()
	tx := api.baseApi.getTx(from, &args.Contract, types.TerminateContractTx, decimal.Zero, args.MaxFee, decimal.Zero, 0,
		0, payload)
	return api.signIfNeeded(from, tx, estimate)
}

func (api *ContractApi) signIfNeeded(from common.Address, tx *types.Transaction, estimate bool) (*types.Transaction, error) {
	sign := !estimate || api.baseApi.canSign(from)
	if !sign {
		return tx, nil
	}
	return api.baseApi.signTransaction(from, tx, nil)
}

func (api *ContractApi) EstimateDeploy(args DeployArgs) (*TxReceipt, error) {
	appState := api.baseApi.getAppStateForCheck()
	vm := vm.NewVmImpl(appState, api.bc, api.bc.Head, nil, api.bc.Config())
	tx, err := api.buildDeployContractTx(args, true)
	if err != nil {
		return nil, err
	}
	var from *common.Address
	if tx.Signed() {
		if err := validation.ValidateTx(appState, tx, appState.State.FeePerGas(), validation.MempoolTx); err != nil {
			return nil, err
		}
	} else {
		from = &args.From
	}
	r := vm.Run(tx, from, -1)
	r.GasCost = api.bc.GetGasCost(appState, r.GasUsed)
	return convertEstimatedReceipt(tx, r, appState.State.FeePerGas()), nil
}

func (api *ContractApi) EstimateCall(args CallArgs) (*TxReceipt, error) {
	appState := api.baseApi.getAppStateForCheck()
	vm := vm.NewVmImpl(appState, api.bc, api.bc.Head, nil, api.bc.Config())
	tx, err := api.buildCallContractTx(args, true)
	if err != nil {
		return nil, err
	}
	var from *common.Address
	if tx.Signed() {
		if err := validation.ValidateTx(appState, tx, appState.State.FeePerGas(), validation.MempoolTx); err != nil {
			return nil, err
		}
	} else {
		from = &args.From
	}
	if !common.ZeroOrNil(tx.Amount) {
		var sender common.Address
		if tx.Signed() {
			sender, _ = types.Sender(tx)
		} else {
			sender = args.From
		}
		appState.State.SubBalance(sender, tx.Amount)
		appState.State.AddBalance(*tx.To, tx.Amount)
	}

	r := vm.Run(tx, from, -1)
	r.GasCost = api.bc.GetGasCost(appState, r.GasUsed)
	return convertEstimatedReceipt(tx, r, appState.State.FeePerGas()), nil
}

func (api *ContractApi) EstimateTerminate(args TerminateArgs) (*TxReceipt, error) {
	appState := api.baseApi.getAppStateForCheck()
	vm := vm.NewVmImpl(appState, api.bc, api.bc.Head, nil, api.bc.Config())
	tx, err := api.buildTerminateContractTx(args, true)
	if err != nil {
		return nil, err
	}
	var from *common.Address
	if tx.Signed() {
		if err := validation.ValidateTx(appState, tx, appState.State.FeePerGas(), validation.MempoolTx); err != nil {
			return nil, err
		}
	} else {
		from = &args.From
	}
	r := vm.Run(tx, from, -1)
	r.GasCost = api.bc.GetGasCost(appState, r.GasUsed)
	return convertEstimatedReceipt(tx, r, appState.State.FeePerGas()), nil
}

func convertActionResultBytes(actionResult []byte) *ActionResult {
	if len(actionResult) == 0 {
		return nil
	}
	protoModel := &models.ActionResult{}

	if err := proto.Unmarshal(actionResult, protoModel); err != nil {
		return nil
	}
	return convertActionResult(protoModel)
}

func convertActionResult(protoModel *models.ActionResult) *ActionResult {
	result := &ActionResult{}
	if protoModel.InputAction != nil {
		result.InputAction = InputAction{
			Args:       protoModel.InputAction.Args,
			Method:     protoModel.InputAction.Method,
			Amount:     protoModel.InputAction.Amount,
			ActionType: protoModel.InputAction.ActionType,
			GasLimit:   protoModel.InputAction.GasLimit,
		}
	}
	result.Success = protoModel.Success
	result.Error = protoModel.Error
	result.GasUsed = protoModel.GasUsed
	result.RemainingGas = protoModel.RemainingGas
	result.OutputData = protoModel.OutputData
	for _, subAction := range protoModel.SubActionResults {
		result.SubActionResults = append(result.SubActionResults, convertActionResult(subAction))
	}
	return result
}

func convertReceipt(tx *types.Transaction, receipt *types.TxReceipt, feePerGas *big.Int) *TxReceipt {
	fee := fee.CalculateFee(1, feePerGas, tx)
	var err string
	if receipt.Error != nil {
		err = receipt.Error.Error()
	}
	txHash := receipt.TxHash
	result := &TxReceipt{
		Success:      receipt.Success,
		Error:        err,
		Method:       receipt.Method,
		Contract:     receipt.ContractAddress,
		TxHash:       &txHash,
		GasUsed:      receipt.GasUsed,
		GasCost:      blockchain.ConvertToFloat(receipt.GasCost),
		TxFee:        blockchain.ConvertToFloat(fee),
		ActionResult: convertActionResultBytes(receipt.ActionResult),
	}
	for _, e := range receipt.Events {
		event := Event{
			Event: e.EventName,
		}
		for i := range e.Data {
			event.Args = append(event.Args, e.Data[i])
		}
		if !e.Contract.IsEmpty() {
			event.Contract = e.Contract
		} else {
			event.Contract = receipt.ContractAddress
		}
		result.Events = append(result.Events, event)
	}
	return result
}

func convertEstimatedReceipt(tx *types.Transaction, receipt *types.TxReceipt, feePerGas *big.Int) *TxReceipt {
	res := convertReceipt(tx, receipt, feePerGas)
	if !tx.Signed() {
		res.TxHash = nil
	}
	return res
}

func (api *ContractApi) Deploy(ctx context.Context, args DeployArgs) (common.Hash, error) {
	tx, err := api.buildDeployContractTx(args, false)
	if err != nil {
		return common.Hash{}, err
	}
	return api.baseApi.sendInternalTx(ctx, tx)
}

func (api *ContractApi) Call(ctx context.Context, args CallArgs) (common.Hash, error) {
	tx, err := api.buildCallContractTx(args, false)
	if err != nil {
		return common.Hash{}, err
	}
	if args.BroadcastBlock > 0 {
		from := args.From
		if from.IsEmpty() {
			from = api.baseApi.getCurrentCoinbase()
		}

		err = api.deferredTxs.AddDeferredTx(from, &args.Contract, blockchain.ConvertToInt(args.Amount), tx.Payload, common.Big0, args.BroadcastBlock)
		return tx.Hash(), err
	}
	return api.baseApi.sendInternalTx(ctx, tx)
}

func (api *ContractApi) Terminate(ctx context.Context, args TerminateArgs) (common.Hash, error) {
	tx, err := api.buildTerminateContractTx(args, false)
	if err != nil {
		return common.Hash{}, err
	}
	return api.baseApi.sendInternalTx(ctx, tx)
}

func (api *ContractApi) ReadData(contract common.Address, key string, format string) (interface{}, error) {
	data := api.baseApi.getReadonlyAppState().State.GetContractValue(contract, []byte(key))
	if data == nil {
		return nil, errors.New("data is nil")
	}
	return conversion(format, data)
}

func (api *ContractApi) BatchReadData(contract common.Address, keys []KeyWithFormat) []ContractData {
	res := make([]ContractData, 0, len(keys))
	for _, keyWithFormat := range keys {
		data := ContractData{
			Key: keyWithFormat.Key,
		}
		if value := api.baseApi.getReadonlyAppState().State.GetContractValue(contract, []byte(keyWithFormat.Key)); value != nil {
			var err error
			data.Value, err = conversion(keyWithFormat.Format, value)
			if err != nil {
				data.Error = err.Error()
			}
		} else {
			data.Error = "data is nil"
		}
		res = append(res, data)
	}
	return res
}

func (api *ContractApi) ReadonlyCall(args ReadonlyCallArgs) (interface{}, error) {
	vm := vm.NewVmImpl(api.baseApi.getReadonlyAppState(), api.bc, api.bc.Head, nil, api.bc.Config())
	convertedArgs, err := args.Args.ToSlice()
	if err != nil {
		return nil, err
	}
	data, err := vm.Read(args.Contract, args.Method, convertedArgs...)
	if err != nil {
		return nil, err
	}
	return conversion(args.Format, data)
}

func (api *ContractApi) GetStake(contract common.Address) interface{} {
	hash := api.baseApi.getReadonlyAppState().State.GetCodeHash(contract)
	stake := api.baseApi.getReadonlyAppState().State.GetContractStake(contract)
	return struct {
		Hash  *common.Hash
		Stake decimal.Decimal
	}{
		hash,
		blockchain.ConvertToFloat(stake),
	}
}

func (api *ContractApi) SubscribeToEvent(contract common.Address, event string) error {
	return api.subManager.Subscribe(contract, event)
}

func (api *ContractApi) UnsubscribeFromEvent(contract common.Address, event string) error {
	return api.subManager.Unsubscribe(contract, event)
}

func (api *ContractApi) Events(args EventsArgs) interface{} {

	events := api.bc.ReadEvents(args.Contract)

	var list []*Event
	for idx := range events {
		e := &Event{
			Contract: events[idx].Contract,
			Event:    events[idx].Event,
		}
		list = append(list, e)
		for i := range events[idx].Args {
			e.Args = append(e.Args, events[idx].Args[i])
		}
	}
	return list
}

func (api *ContractApi) ReadMap(contract common.Address, mapName string, key hexutil.Bytes, format string) (interface{}, error) {
	data := api.baseApi.getReadonlyAppState().State.GetContractValue(contract, env.FormatMapKey([]byte(mapName), key))
	if data == nil {
		return nil, errors.New("data is nil")
	}
	return conversion(format, data)
}

func (api *ContractApi) IterateMap(contract common.Address, mapName string, continuationToken *hexutil.Bytes, keyFormat, valueFormat string, limit int) (*IterateMapResponse, error) {
	state := api.baseApi.getReadonlyAppState().State

	minKey := []byte(mapName)
	maxKey := []byte(mapName)
	for i := len([]byte(mapName)); i < common.MaxWasmContractStoreKeyLength; i++ {
		maxKey = append(maxKey, 0xFF)
	}

	if continuationToken != nil && len(*continuationToken) > 0 {
		minKey = *continuationToken
	}

	var items []*MapItem
	var err error
	var token hexutil.Bytes
	prefixLen := len([]byte(mapName))
	state.IterateContractStore(contract, minKey, maxKey, func(key []byte, value []byte) bool {

		if len(items) >= limit {
			token = key
			return true
		}

		item := new(MapItem)
		item.Key, err = conversion(keyFormat, key[prefixLen:])
		if err != nil {
			return true
		}
		item.Value, err = conversion(valueFormat, value)
		if err != nil {
			return true
		}
		items = append(items, item)
		return false
	})
	if err != nil {
		return nil, err
	}
	return &IterateMapResponse{
		Items:             items,
		ContinuationToken: &token,
	}, nil
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
