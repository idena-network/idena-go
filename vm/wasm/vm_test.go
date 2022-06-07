package wasm

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/idena-network/idena-go/vm/wasm/testdata"
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"math/rand"
	"testing"
)

func TestVm_Erc20(t *testing.T) {
	db := dbm.NewMemDB()
	appState, _ := appstate.NewAppState(db, eventbus.New())
	appState.Initialize(0)

	vm := NewWasmVM(appState, createHeader(1, 1))
	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)

	code, _ := testdata.Erc20()

	deployAttach := attachments.CreateDeployContractAttachment(common.Hash{}, code)
	payload, _ := deployAttach.ToBytes()

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		Type:         types.DeployContractTx,
		Payload:      payload,
		Amount:       big.NewInt(10),
	}
	tx, _ = types.SignTx(tx, key)

	receipt := vm.Run(tx, 100000)
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)

	appState.State.IterateContractStore(receipt.ContractAddress, nil, nil, func(key []byte, value []byte) bool {
		v, _ := helpers.ExtractUInt64(0, value)
		t.Logf("key=%v, value=%v\n", key, v)
		return false
	})

	addr := common.Address{0x1}
	callAttach := attachments.CreateCallContractAttachment("transfer", addr.Bytes(), common.ToBytes(uint64(777)))
	payload, _ = callAttach.ToBytes()

	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &receipt.ContractAddress,
		Type:         types.CallContractTx,
		Payload:      payload,
		Amount:       big.NewInt(10),
	}
	tx, _ = types.SignTx(tx, key)
	receipt = vm.Run(tx, 1000000)
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)

	appState.State.IterateContractStore(receipt.ContractAddress, nil, nil, func(key []byte, value []byte) bool {
		v, _ := helpers.ExtractUInt64(0, value)
		t.Logf("key=%v, value=%v\n", key, v)
		return false
	})

	key2, _ := crypto.GenerateKeyFromSeed(rnd)
	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &receipt.ContractAddress,
		Type:         types.CallContractTx,
		Payload:      payload,
		Amount:       big.NewInt(10),
	}
	tx, _ = types.SignTx(tx, key2)
	receipt = vm.Run(tx, 1000000)
	t.Logf("%+v\n", receipt)
	require.False(t, receipt.Success)
}

var nonce = uint32(1)

func deployContract(key *ecdsa.PrivateKey, appState *appstate.AppState, code []byte, args ...[]byte) *types.TxReceipt {
	vm := NewWasmVM(appState, createHeader(1, 1))
	deployAttach := attachments.CreateDeployContractAttachment(common.Hash{}, code, args...)
	payload, _ := deployAttach.ToBytes()

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: nonce,
		Type:         types.DeployContractTx,
		Payload:      payload,
		Amount:       big.NewInt(10),
	}
	tx, _ = types.SignTx(tx, key)
	nonce++
	return vm.Run(tx, 100000)
}

func callContract(key *ecdsa.PrivateKey, appState *appstate.AppState, contract common.Address, method string, args ...[]byte) *types.TxReceipt {
	vm := NewWasmVM(appState, createHeader(1, 1))
	callAttach := attachments.CreateCallContractAttachment(method, args...)
	payload, _ := callAttach.ToBytes()

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: nonce,
		To:           &contract,
		Type:         types.CallContractTx,
		Payload:      payload,
		Amount:       big.NewInt(0),
	}
	tx, _ = types.SignTx(tx, key)
	nonce++
	return vm.Run(tx, 1000000)
}

func TestVm_IncAndSum_cross_contract_call(t *testing.T) {
	db := dbm.NewMemDB()
	appState, _ := appstate.NewAppState(db, eventbus.New())
	appState.Initialize(0)

	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)

	code, _ := testdata.IncFunc()
	receipt := deployContract(key, appState, code)
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)

	//appState.Commit(nil, true)

	code, _ = testdata.SumFunc()

	receipt = deployContract(key, appState, code, receipt.ContractAddress.Bytes())
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)


	appState.State.IterateContractStore(receipt.ContractAddress, nil, nil, func(key []byte, value []byte) bool {
		v, _ := helpers.ExtractUInt64(0, value)
		t.Logf("key=%v, value=%v\n", key, v)
		return false
	})


	receipt = callContract(key, appState, receipt.ContractAddress, "invoke", common.ToBytes(uint64(1)), common.ToBytes(uint64(5)))
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)
}
