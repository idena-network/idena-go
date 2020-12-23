package api

import (
	"context"
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
	"github.com/idena-network/idena-go/vm/helpers"
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
	MaxFee   decimal.Decimal `json:"maxFee"`
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

func (a DynamicArg) ToBytes() []byte {
	switch a.Format {
	case "byte":
		i, err := strconv.ParseInt(a.Value, 10, 8)
		if err != nil {
			return nil
		}
		return []byte{byte(i)}
	case "uint64":
		i, err := strconv.ParseInt(a.Value, 10, 64)
		if err != nil {
			return nil
		}
		return common.ToBytes(uint64(i))
	case "string":
		return []byte(a.Value)
	case "bigint":
		v := new(big.Int)
		v.SetString(a.Value, 10)
		return v.Bytes()
	case "hex":
		data, err := hexutil.Decode(a.Value)
		if err != nil {
			return nil
		}
		return data
	case "dna":
		d, err := decimal.NewFromString(a.Value)
		if err != nil {
			return nil
		}
		return blockchain.ConvertToInt(d).Bytes()
	default:
		data, err := hexutil.Decode(a.Value)
		if err != nil {
			return nil
		}
		return data
	}
}

func (d DynamicArgs) ToSlice() [][]byte {

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
			data = append(data, a.ToBytes())
		} else {
			data = append(data, nil)
		}
	}
	return data
}

type TxReceipt struct {
	Contract common.Address  `json:"contract"`
	Method   string          `json:"method"`
	Success  bool            `json:"success"`
	GasUsed  uint64          `json:"gasUsed"`
	TxHash   common.Hash     `json:"txHash"`
	Error    string          `json:"error"`
	GasCost  decimal.Decimal `json:"gasCost"`
	TxFee    decimal.Decimal `json:"txFee"`
}

type Event struct {
	Contract common.Address  `json:"contract"`
	Event    string          `json:"event"`
	Args     []hexutil.Bytes `json:"args"`
}

func (api *ContractApi) buildDeployContractTx(args DeployArgs) (*types.Transaction, error) {
	var codeHash common.Hash
	codeHash.SetBytes(args.CodeHash)

	from := args.From
	if from == (common.Address{}) {
		from = api.baseApi.getCurrentCoinbase()
	}

	payload, _ := attachments.CreateDeployContractAttachment(codeHash, args.Args.ToSlice()...).ToBytes()
	return api.baseApi.getSignedTx(from, nil, types.DeployContract, args.Amount,
		args.MaxFee, decimal.Zero,
		0, 0,
		payload, nil)
}

func (api *ContractApi) buildCallContractTx(args CallArgs) (*types.Transaction, error) {

	from := args.From
	if from == (common.Address{}) {
		from = api.baseApi.getCurrentCoinbase()
	}

	payload, _ := attachments.CreateCallContractAttachment(args.Method, args.Args.ToSlice()...).ToBytes()
	return api.baseApi.getSignedTx(from, &args.Contract, types.CallContract, args.Amount,
		args.MaxFee, decimal.Zero,
		0, 0,
		payload, nil)
}

func (api *ContractApi) buildTerminateContractTx(args TerminateArgs) (*types.Transaction, error) {

	from := args.From
	if from == (common.Address{}) {
		from = api.baseApi.getCurrentCoinbase()
	}

	payload, _ := attachments.CreateTerminateContractAttachment(args.Args.ToSlice()...).ToBytes()
	return api.baseApi.getSignedTx(from, &args.Contract, types.TerminateContract, decimal.Zero,
		args.MaxFee, decimal.Zero,
		0, 0,
		payload, nil)
}

func (api *ContractApi) EstimateDeploy(args DeployArgs) (*TxReceipt, error) {
	appState := api.baseApi.getAppStateForCheck()
	vm := vm.NewVmImpl(appState, api.bc.Head, api.baseApi.secStore, nil)
	tx, err := api.buildDeployContractTx(args)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateTx(appState, tx, appState.State.FeePerGas(), validation.MempoolTx); err != nil {
		return nil, err
	}
	r := vm.Run(tx, -1)
	r.GasCost = api.bc.GetGasCost(appState, r.GasUsed)
	return convertReceipt(tx, r, appState.State.FeePerGas()), nil
}

func (api *ContractApi) EstimateCall(args CallArgs) (*TxReceipt, error) {
	appState := api.baseApi.getAppStateForCheck()
	vm := vm.NewVmImpl(appState, api.bc.Head, api.baseApi.secStore, nil)
	tx, err := api.buildCallContractTx(args)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateTx(appState, tx, appState.State.FeePerGas(), validation.MempoolTx); err != nil {
		return nil, err
	}
	if !common.ZeroOrNil(tx.Amount) {
		sender, _ := types.Sender(tx)
		appState.State.SubBalance(sender, tx.Amount)
		appState.State.AddBalance(*tx.To, tx.Amount)
	}

	r := vm.Run(tx, -1)
	r.GasCost = api.bc.GetGasCost(appState, r.GasUsed)
	return convertReceipt(tx, r, appState.State.FeePerGas()), nil
}

func (api *ContractApi) EstimateTerminate(args TerminateArgs) (*TxReceipt, error) {
	appState := api.baseApi.getAppStateForCheck()
	vm := vm.NewVmImpl(appState, api.bc.Head, api.baseApi.secStore, nil)
	tx, err := api.buildTerminateContractTx(args)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateTx(appState, tx, appState.State.FeePerGas(), validation.MempoolTx); err != nil {
		return nil, err
	}
	r := vm.Run(tx, -1)
	r.GasCost = api.bc.GetGasCost(appState, r.GasUsed)
	return convertReceipt(tx, r, appState.State.FeePerGas()), nil
}

func convertReceipt(tx *types.Transaction, receipt *types.TxReceipt, feePerGas *big.Int) *TxReceipt {
	fee := fee.CalculateFee(1, feePerGas, tx)
	var err string
	if receipt.Error != nil {
		err = receipt.Error.Error()
	}
	return &TxReceipt{
		Success:  receipt.Success,
		Error:    err,
		Method:   receipt.Method,
		Contract: receipt.ContractAddress,
		TxHash:   receipt.TxHash,
		GasUsed:  receipt.GasUsed,
		GasCost:  blockchain.ConvertToFloat(receipt.GasCost),
		TxFee:    blockchain.ConvertToFloat(fee),
	}
}

func (api *ContractApi) Deploy(ctx context.Context, args DeployArgs) (common.Hash, error) {
	tx, err := api.buildDeployContractTx(args)
	if err != nil {
		return common.Hash{}, err
	}
	return api.baseApi.sendInternalTx(ctx, tx)
}

func (api *ContractApi) Call(ctx context.Context, args CallArgs) (common.Hash, error) {
	tx, err := api.buildCallContractTx(args)
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
	tx, err := api.buildTerminateContractTx(args)
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

func (api *ContractApi) ReadonlyCall(args ReadonlyCallArgs) (interface{}, error) {
	vm := vm.NewVmImpl(api.baseApi.getReadonlyAppState(), api.bc.Head, api.baseApi.secStore, nil)
	data, err := vm.Read(args.Contract, args.Method, args.Args.ToSlice()...)
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
