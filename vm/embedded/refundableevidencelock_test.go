package embedded

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"math/rand"
	"testing"
)

func TestRefundableEvidenceLock_Call(t *testing.T) {

	oraceVotingAddr := common.Address{0x1}
	successAddr := common.Address{0x2}
	failAddr := common.Address{0x3}

	db := dbm.NewMemDB()
	appState := appstate.NewAppState(db, eventbus.New())

	appState.State.DeployContract(oraceVotingAddr, OracleVotingContract, common.DnaBase)

	appState.State.SetFeePerGas(big.NewInt(1))
	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)
	addr := crypto.PubkeyToAddress(key.PublicKey)
	appState.State.SetState(addr, state.Newbie)
	appState.State.SetBalance(addr, common.DnaBase)
	appState.State.SetPubKey(addr, crypto.FromECDSAPub(&key.PublicKey))
	appState.IdentityState.Add(addr)

	secStore := secstore.NewSecStore()

	var identities []*ecdsa.PrivateKey

	for i := 0; i < 2000; i++ {
		key, _ := crypto.GenerateKeyFromSeed(rnd)
		identities = append(identities, key)
		addr := crypto.PubkeyToAddress(key.PublicKey)
		appState.State.SetState(addr, state.Newbie)
		appState.State.SetBalance(addr, common.DnaBase)
		appState.State.SetPubKey(addr, crypto.FromECDSAPub(&key.PublicKey))
		appState.IdentityState.Add(addr)
	}
	appState.Commit(nil)

	appState.Initialize(1)

	attachment := attachments.CreateDeployContractAttachment(RefundableEvidenceLockContract, oraceVotingAddr.Bytes(),
		common.ToBytes(byte(1)), successAddr.Bytes(), failAddr.Bytes(), nil, common.ToBytes(uint64(1000)), common.ToBytes(byte(5)))
	payload, err := attachment.ToBytes()
	require.NoError(t, err)

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: 1,
		Type:         types.DeployContract,
		Amount:       common.DnaBase,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)
	ctx := env.NewDeployContextImpl(tx, attachment.CodeHash)

	gas := new(env.GasCounter)
	gas.Reset(-1)

	// deploy
	e := env.NewEnvImp(appState, createHeader(2, 1), gas, secStore, nil)
	contract := NewRefundableEvidenceLock(ctx, e, nil)

	contractAddr := ctx.ContractAddr()

	appState.State.AddBalance(contractAddr, big.NewInt(0).Mul(common.DnaBase, big.NewInt(500)))
	require.NoError(t, contract.Deploy(attachment.Args...))
	e.Commit()

	for i := 0; i < 100; i++ {
		key := identities[i]
		//addr := crypto.PubkeyToAddress(key.PublicKey)

		attachment := attachments.CreateCallContractAttachment("deposit")
		payload, err := attachment.ToBytes()
		require.NoError(t, err)

		tx := &types.Transaction{
			Epoch:        0,
			AccountNonce: 1,
			Type:         types.CallContract,
			To:           &contractAddr,
			Amount:       common.DnaBase,
			Payload:      payload,
		}
		tx, _ = types.SignTx(tx, key)

		ctx := env.NewCallContextImpl(tx, RefundableEvidenceLockContract)
		gas.Reset(-1)
		e = env.NewEnvImp(appState, createHeader(4, 21), gas, secStore, nil)
		contract = NewRefundableEvidenceLock(ctx, e, nil)
		err = contract.Call(attachment.Method, attachment.Args...)
		e.Commit()
		require.NoError(t, err)
	}

	require.True(t, big.NewInt(0).Mul(common.DnaBase, big.NewInt(5)).Cmp(appState.State.GetBalance(oraceVotingAddr)) == 0)

	totalSum := e.ReadContractData(contractAddr, []byte("sum"))

	require.Equal(t, totalSum, big.NewInt(0).Mul(big.NewInt(100), common.DnaBase).Bytes())

	deposit := e.ReadContractData(contractAddr, append([]byte("deposits"), crypto.PubkeyToAddress(identities[50].PublicKey).Bytes()...))

	require.Equal(t, common.DnaBase.Bytes(), deposit)

	callAttach := attachments.CreateCallContractAttachment("refund")
	payload, _ = attachment.ToBytes()

	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 1,
		Type:         types.CallContract,
		To:           &contractAddr,
		Amount:       common.DnaBase,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)
	gas.Reset(-1)
	e = env.NewEnvImp(appState, createHeader(4, 21), gas, secStore, nil)
	contract = NewRefundableEvidenceLock(env.NewCallContextImpl(tx, RefundableEvidenceLockContract), e, nil)
	err = contract.Call(callAttach.Method, callAttach.Args...)
	require.Error(t, err)
	e.Reset()

	callAttach = attachments.CreateCallContractAttachment("push")
	payload, _ = attachment.ToBytes()

	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 1,
		Type:         types.CallContract,
		To:           &contractAddr,
		Amount:       common.DnaBase,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)
	gas.Reset(-1)
	e = env.NewEnvImp(appState, createHeader(4, 21), gas, secStore, nil)
	contract = NewRefundableEvidenceLock(env.NewCallContextImpl(tx, RefundableEvidenceLockContract), e, nil)
	err = contract.Call(callAttach.Method, callAttach.Args...)
	require.Error(t, err)
	e.Reset()

}
