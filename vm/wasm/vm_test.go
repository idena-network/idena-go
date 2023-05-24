package wasm

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/idena-network/idena-go/vm/wasm/testdata"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"math/rand"
	"testing"
)

func getLatestConfig() *config.Config {
	key, _ := crypto.GenerateKey()
	secStore := secstore.NewSecStore()

	secStore.AddKey(crypto.FromECDSA(key))
	alloc := make(map[common.Address]config.GenesisAllocation)
	cfg := &config.Config{
		Network:   0x99,
		Consensus: config.ConsensusVersions[config.ConsensusV12],
		GenesisConf: &config.GenesisConf{
			Alloc:      alloc,
			GodAddress: secStore.GetAddress(),
		},
		Blockchain:       &config.BlockchainConfig{},
		OfflineDetection: config.GetDefaultOfflineDetectionConfig(),
		IsDebug:          true,
	}
	return cfg
}

func TestVm_Erc20(t *testing.T) {
	db := dbm.NewMemDB()
	appState, _ := appstate.NewAppState(db, eventbus.New())
	appState.Initialize(0)

	vm := NewWasmVM(appState, nil, createHeader(1, 1), getLatestConfig(), true)
	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)

	code, _ := testdata.Erc20()

	deployAttach := attachments.CreateDeployContractAttachment(common.Hash{}, code, nil)
	payload, _ := deployAttach.ToBytes()

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		Type:         types.DeployContractTx,
		Payload:      payload,
		Amount:       big.NewInt(10),
	}
	tx, _ = types.SignTx(tx, key)

	receipt := vm.Run(tx, 4000000)
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)

	appState.State.IterateContractStore(receipt.ContractAddress, nil, nil, func(key []byte, value []byte) bool {
		t.Logf("key=%v, value=%v\n", key, string(value))
		return false
	})
	totalSupply := int64(1000000000)
	transferAmount := int64(777)

	addr := common.Address{0x1}
	callAttach := attachments.CreateCallContractAttachment("transfer", addr.Bytes(), big.NewInt(transferAmount).Bytes())
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
	receipt = vm.Run(tx, 10000000)
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)

	addrBalance := appState.State.GetContractValue(receipt.ContractAddress, append([]byte("b:"), addr.Bytes()...))
	require.True(t, big.NewInt(transferAmount).Cmp(big.NewInt(0).SetBytes(addrBalance)) == 0)

	ownerBalance := appState.State.GetContractValue(receipt.ContractAddress, append([]byte("b:"), crypto.PubkeyToAddress(key.PublicKey).Bytes()...))
	require.True(t, big.NewInt(totalSupply-transferAmount).Cmp(big.NewInt(0).SetBytes(ownerBalance)) == 0)

	appState.State.IterateContractStore(receipt.ContractAddress, nil, nil, func(key []byte, value []byte) bool {
		t.Logf("key=%v, value=%v\n", key, value)
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
	receipt = vm.Run(tx, 10000000)
	t.Logf("%+v\n", receipt)
	require.False(t, receipt.Success)
}

var nonce = uint32(1)

func deployContract(key *ecdsa.PrivateKey, appState *appstate.AppState, code []byte, args ...[]byte) *types.TxReceipt {
	vm := NewWasmVM(appState, nil, createHeader(1, 1), getLatestConfig(), true)
	deployAttach := attachments.CreateDeployContractAttachment(common.Hash{}, code, nil, args...)
	payload, _ := deployAttach.ToBytes()

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: nonce,
		Type:         types.DeployContractTx,
		Payload:      payload,
		Amount:       big.NewInt(0),
	}
	tx, _ = types.SignTx(tx, key)
	nonce++
	return vm.Run(tx, 5000000)
}

func callContract(key *ecdsa.PrivateKey, appState *appstate.AppState, contract common.Address, method string, args ...[]byte) *types.TxReceipt {
	vm := NewWasmVM(appState, nil, createHeader(1, 1), getLatestConfig(), true)
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
	return vm.Run(tx, 612000000)
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

	value := appState.State.GetContractValue(receipt.ContractAddress, []byte("sum"))
	require.Equal(t, common.ToBytes(uint64(7)), value)
}

func TestVm_DeployContractViaContract(t *testing.T) {
	db := dbm.NewMemDB()
	appState, _ := appstate.NewAppState(db, eventbus.New())
	appState.Initialize(0)

	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)

	code, _ := testdata.TestCases()
	receipt := deployContract(key, appState, code)
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)

	//appState.Commit(nil, true)

	initBalance := big.NewInt(0).Mul(big.NewInt(1000000), common.DnaBase)
	appState.State.SetBalance(receipt.ContractAddress, initBalance)

	code2, _ := testdata.SumFunc()

	receipt = callContract(key, appState, receipt.ContractAddress, "test", common.ToBytes(uint32(1)), code2)
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)

	appState.State.IterateContractStore(receipt.ContractAddress, nil, nil, func(key []byte, value []byte) bool {
		v, _ := helpers.ExtractUInt64(0, value)
		t.Logf("key=%v, value=%v\n", key, v)
		return false
	})

	appState.Commit(nil)

	sum := big.NewInt(0)
	appState.State.IterateAccounts(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])
		var data state.Account
		if err := data.FromBytes(value); err != nil {
			return false
		}
		if data.Balance != nil {
			sum.Add(sum, data.Balance)
		}
		if data.Contract != nil && data.Contract.Stake != nil {
			sum.Add(sum, data.Contract.Stake)
		}
		t.Logf("addr=%v balance=%v stake=%v", addr.Hex(), ConvertToFloat(data.Balance).String(), ConvertToFloat(data.Contract.Stake).String())
		return false
	})
	require.Equal(t, initBalance.String(), sum.String())
}

func ConvertToFloat(amount *big.Int) decimal.Decimal {
	if amount == nil {
		return decimal.Zero
	}
	decimalAmount := decimal.NewFromBigInt(amount, 0)

	return decimalAmount.DivRound(decimal.NewFromBigInt(common.DnaBase, 0), 18)
}

func Test_SharedFungibleToken(t *testing.T) {
	db := dbm.NewMemDB()
	appState, _ := appstate.NewAppState(db, eventbus.New())
	appState.Initialize(0)

	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)

	addr := crypto.PubkeyToAddress(key.PublicKey)

	t.Logf("addr=%v", addr.Hex())

	code, _ := testdata.SharedFungibleToken()
	receipt := deployContract(key, appState, code, addr.Bytes(), common.Address{0xA}.Bytes())
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)

	firstContract := receipt.ContractAddress

	appState.State.SetContractValue(firstContract, []byte("b"), big.NewInt(1000).Bytes())

	appState.State.IterateContractStore(receipt.ContractAddress, nil, nil, func(key []byte, value []byte) bool {
		v, _ := helpers.ExtractUInt64(0, value)
		t.Logf("key=%v, value=%v\n", key, v)
		return false
	})

	appState.Commit(nil)

	destination := common.Address{0x3}

	receipt = callContract(key, appState, firstContract, "transferTo", destination.Bytes(), big.NewInt(100).Bytes())
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)

	appState.State.IterateContractStore(receipt.ContractAddress, nil, nil, func(key []byte, value []byte) bool {
		v, _ := helpers.ExtractUInt64(0, value)
		t.Logf("key=%v, value=%v\n", key, v)
		return false
	})

	appState.State.IterateContractStore(firstContract, nil, nil, func(key []byte, value []byte) bool {
		v, _ := helpers.ExtractUInt64(0, value)
		t.Logf("key=%v, value=%v\n", key, v)
		return false
	})
	// update addr if code is recompiled
	secondContract := common.Address{111, 16, 67, 101, 164, 106, 165, 108, 212, 68, 160, 27, 240, 49, 207, 98, 95, 98, 34, 6}

	appState.Commit(nil)

	receipt = callContract(key, appState, firstContract, "transferTo", destination.Bytes(), big.NewInt(100).Bytes())
	t.Logf("%+v\n", receipt)
	require.True(t, receipt.Success)
	require.Equal(t, big.NewInt(800), big.NewInt(0).SetBytes(appState.State.GetContractValue(firstContract, []byte("b"))))

	//check secondContract if assertion is failed
	require.Equal(t, big.NewInt(200), big.NewInt(0).SetBytes(appState.State.GetContractValue(secondContract, []byte("b"))))
}

func createHeader(height uint64, time int64) *types.Header {
	seed := types.Seed{}
	seed.SetBytes(common.ToBytes(height))
	return &types.Header{
		ProposedHeader: &types.ProposedHeader{
			BlockSeed: seed,
			Height:    height,
			Time:      time,
		},
	}
}
