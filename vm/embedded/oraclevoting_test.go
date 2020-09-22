package embedded

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"math/rand"
	"testing"
)

func getDbSize(db *dbm.MemDB) int {
	it, _ := db.Iterator(nil, nil)
	size := 0
	for ; it.Valid(); it.Next() {
		size += len(it.Key()) + len(it.Value())
	}
	it.Close()
	return size
}

func printDbSize(db *dbm.MemDB, descr string) {
	fmt.Printf("%v: size of memDB=%v (key+value)\n", descr, getDbSize(db))
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

func TestFactChecking_Call(t *testing.T) {
	db := dbm.NewMemDB()
	appState := appstate.NewAppState(db, eventbus.New())

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
		appState.State.SetPubKey(addr, crypto.FromECDSAPub(&key.PublicKey))
		appState.IdentityState.Add(addr)
	}
	appState.Commit(nil)

	appState.Initialize(1)

	printDbSize(db, "Before contract deploy")

	attachment := attachments.CreateDeployContractAttachment(OracleVotingContract, []byte{0x1}, common.ToBytes(uint64(10)),
		nil, nil, nil, nil, nil, nil, nil, common.ToBytes(byte(5)))
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
	e := env.NewEnvImp(appState, createHeader(2, 1), gas, secStore)
	contract := NewOracleVotingContract(ctx, e)

	require.NoError(t, contract.Deploy(attachment.Args...))
	e.Commit()
	contractAddr := ctx.ContractAddr()

	appState.Commit(nil)
	printDbSize(db, "After contract deploy")
	fmt.Printf("Deploy gas: %v\n", gas.UsedGas)
	gas.Reset(-1)
	// start voting

	callAttach := attachments.CreateCallContractAttachment("startVoting")
	payload, _ = callAttach.ToBytes()
	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &contractAddr,
		Type:         types.CallContract,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)

	txNotOwner, _ := types.SignTx(tx, identities[0])

	e = env.NewEnvImp(appState, createHeader(3, 21), gas, secStore)
	contract = NewOracleVotingContract(env.NewCallContextImpl(txNotOwner, OracleVotingContract), e)

	require.Error(t, contract.Call("startVoting"))
	e.Reset()
	gas.Reset(-1)
	e = env.NewEnvImp(appState, createHeader(3, 21), gas, secStore)
	contract = NewOracleVotingContract(env.NewCallContextImpl(tx, OracleVotingContract), e)

	require.Error(t, contract.Call("startVoting"))
	e.Reset()
	gas.Reset(-1)

	appState.State.SetBalance(contractAddr, big.NewInt(20000000))

	require.NoError(t, contract.Call("startVoting"))
	e.Commit()

	appState.Commit(nil)
	printDbSize(db, "After start voting")
	fmt.Printf("Start voting gas: %v\n", gas.UsedGas)
	gas.Reset(-1)

	require.Equal(t, common.ToBytes(uint64(1)), appState.State.GetContractValue(contractAddr, []byte("state")))
	require.Equal(t, big.NewInt(1000000).Bytes(), appState.State.GetContractValue(contractAddr, []byte("votingMinPayment")))

	seed := types.Seed{}
	seed.SetBytes(common.ToBytes(uint64(3)))
	require.Equal(t, seed.Bytes(), appState.State.GetContractValue(contractAddr, []byte("vrfSeed")))
	require.Equal(t, common.ToBytes(uint64(100)), appState.State.GetContractValue(contractAddr, []byte("committeeSize")))

	// send proofs
	winnerVote := byte(1)
	votedIdentities := map[common.Address]struct{}{}
	minPayment := big.NewInt(0).SetBytes(appState.State.GetContractValue(contractAddr, []byte("votingMinPayment")))

	voted := 0
	for _, key := range identities {

		pubBytes := crypto.FromECDSAPub(&key.PublicKey)

		addr := crypto.PubkeyToAddress(key.PublicKey)

		hash := crypto.Hash(append(pubBytes, appState.State.GetContractValue(contractAddr, []byte("vrfSeed"))...))

		v := new(big.Float).SetInt(new(big.Int).SetBytes(hash[:]))

		q := new(big.Float).Quo(v, maxHash)

		threshold, _ := helpers.ExtractUInt64(0, appState.State.GetContractValue(contractAddr, []byte("committeeSize")))
		shouldBeError := false
		if q.Cmp(big.NewFloat(1-float64(threshold)/float64(appState.ValidatorsCache.NetworkSize()))) < 0 {
			shouldBeError = true
		}
		_, err = contract.Read("proof", addr.Bytes())
		require.Equal(t, shouldBeError, err != nil)

		salt := addr.Bytes()
		voteHash, err := contract.Read("voteHash", common.ToBytes(winnerVote), salt)
		require.Nil(t, err)

		callAttach = attachments.CreateCallContractAttachment("sendVoteProof", voteHash[:])
		payload, _ = callAttach.ToBytes()
		tx = &types.Transaction{
			Epoch:        0,
			AccountNonce: 2,
			To:           &contractAddr,
			Type:         types.CallContract,
			Payload:      payload,
			Amount:       minPayment,
		}
		tx, _ = types.SignTx(tx, key)
		gas.Reset(-1)
		e = env.NewEnvImp(appState, createHeader(4, 21), gas, secStore)
		contract = NewOracleVotingContract(env.NewCallContextImpl(tx, OracleVotingContract), e)
		err = contract.Call(callAttach.Method, callAttach.Args...)
		if shouldBeError {
			e.Reset()
			require.Error(t, err)
		} else {
			voted++
			e.Commit()
			appState.State.AddBalance(contractAddr, minPayment)
			appState.Commit(nil)
			require.NoError(t, err)
			votedIdentities[addr] = struct{}{}
		}
	}
	appState.Commit(nil)

	printDbSize(db, "After send vote proofs")
	gas.Reset(-1)

	//send votes

	for _, key := range identities {
		addr := crypto.PubkeyToAddress(key.PublicKey)

		salt := addr.Bytes()
		callAttach = attachments.CreateCallContractAttachment("sendVote", common.ToBytes(winnerVote), salt)
		payload, _ = callAttach.ToBytes()
		tx = &types.Transaction{
			Epoch:        0,
			AccountNonce: 2,
			To:           &contractAddr,
			Type:         types.CallContract,
			Payload:      payload,
		}
		gas.Reset(-1)
		tx, _ = types.SignTx(tx, key)
		e = env.NewEnvImp(appState, createHeader(4320*2, 21), gas, secStore)
		contract = NewOracleVotingContract(env.NewCallContextImpl(tx, OracleVotingContract), e)

		err := contract.Call(callAttach.Method, callAttach.Args...)
		if _, ok := votedIdentities[addr]; !ok {
			e.Reset()
			require.Error(t, err)
		} else {
			e.Commit()
			appState.Commit(nil)
			require.NoError(t, err)
		}
	}

	appState.Commit(nil)
	printDbSize(db, "After send open votes")
	gas.Reset(-1)

	callAttach = attachments.CreateCallContractAttachment("finishVoting")
	payload, _ = callAttach.ToBytes()
	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &contractAddr,
		Type:         types.CallContract,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)
	gas.Reset(-1)
	e = env.NewEnvImp(appState, createHeader(4320*2, 21), gas, secStore)
	contract = NewOracleVotingContract(env.NewCallContextImpl(tx, OracleVotingContract), e)

	require.NoError(t, contract.Call(callAttach.Method, callAttach.Args...))
	e.Commit()
	appState.Commit(nil)
	printDbSize(db, "After finishVoting")

	for addr := range votedIdentities {
		b := appState.State.GetBalance(addr)
		expected, _ := big.NewInt(0).SetString("1206521", 10)
		require.Equal(t, 0, expected.Cmp(b))
	}

	stakeAfterFinish := appState.State.GetContractStake(contractAddr)

	require.Equal(t, 0, big.NewInt(0).Add(common.DnaBase, big.NewInt(1000000)).Cmp(stakeAfterFinish))

	fmt.Printf("Finish voting gas: %v\n", gas.UsedGas)

	gas.Reset(-1)
	require.Equal(t, []byte{winnerVote}, appState.State.GetContractValue(contractAddr, []byte("result")))

	for addr, _ := range votedIdentities {
		require.True(t, appState.State.GetBalance(addr).Sign() == 1)
	}
	require.True(t, appState.State.GetBalance(contractAddr).Sign() == 0)

	//terminate
	terminateAttach := attachments.CreateTerminateContractAttachment()
	payload, _ = terminateAttach.ToBytes()
	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &contractAddr,
		Type:         types.TerminateContract,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)

	e = env.NewEnvImp(appState, createHeader(4320*3+4, 21), gas, secStore)
	contract = NewOracleVotingContract(env.NewCallContextImpl(tx, OracleVotingContract), e)
	require.NoError(t, contract.Terminate(terminateAttach.Args...))
	e.Commit()

	appState.Commit(nil)
	printDbSize(db, "After terminating")
	fmt.Printf("Terminating gas: %v\n", gas.UsedGas)

	require.Equal(t, 0, appState.State.GetBalance(addr).Cmp(big.NewInt(0).Add(common.DnaBase, stakeAfterFinish)))
	require.Nil(t, appState.State.GetCodeHash(contractAddr))
	require.Equal(t, 0, appState.State.GetStakeBalance(contractAddr).Sign())

	require.Nil(t, appState.State.GetContractValue(contractAddr, []byte("vrfSeed")))
}
