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
	"github.com/idena-network/idena-go/crypto/vrf/p256"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"math/rand"
	"testing"
)

func TestFactChecking_Call(t *testing.T) {
	db := dbm.NewMemDB()
	appState := appstate.NewAppState(db, eventbus.New())

	appState.State.SetFeePerByte(big.NewInt(1))
	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)

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

	crypto.PubkeyToAddress(key.PublicKey)
	attachment := attachments.CreateDeployContractAttachment(FactEvidenceContract, []byte{0x1}, common.ToBytes(uint64(10)))
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
	ctx := env.NewDeployContextImpl(tx)

	createHeader := func(height uint64, time int64) *types.Header {
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
	gas := new(env.GasCounter)
	gas.Reset(-1)

	// deploy
	e := env.NewEnvImp(appState, createHeader(2, 1), gas, secStore)
	contract := NewFactEvidenceContract(ctx, e)

	require.NoError(t, contract.Deploy(attachment.Args...))
	e.Commit()
	contractAddr := ctx.ContractAddr()

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
	contract = NewFactEvidenceContract(env.NewCallContextImpl(txNotOwner), e)

	require.Error(t, contract.Call("startVoting"))
	e.Reset()
	e = env.NewEnvImp(appState, createHeader(3, 21), gas, secStore)
	contract = NewFactEvidenceContract(env.NewCallContextImpl(tx), e)

	require.Error(t, contract.Call("startVoting"))
	e.Reset()

	appState.State.SetBalance(contractAddr, big.NewInt(20000000))

	require.NoError(t, contract.Call("startVoting"))
	e.Commit()

	require.Equal(t, common.ToBytes(uint64(1)), appState.State.GetContractValue(contractAddr, []byte("state")))
	require.Equal(t, big.NewInt(1000000).Bytes(), appState.State.GetContractValue(contractAddr, []byte("votingMinPayment")))

	seed := types.Seed{}
	seed.SetBytes(common.ToBytes(uint64(3)))
	require.Equal(t, seed.Bytes(), appState.State.GetContractValue(contractAddr, []byte("vrfSeed")))
	require.Equal(t, common.ToBytes(uint64(100)), appState.State.GetContractValue(contractAddr, []byte("committeeSize")))

	// send proofs
	winnerVote := byte(1)
	votedIdentities := map[common.Address]struct{}{}

	for _, key := range identities {

		addr := crypto.PubkeyToAddress(key.PublicKey)

		signer, _ := p256.NewVRFSigner(key)

		hash, proof := signer.Evaluate(appState.State.GetContractValue(contractAddr, []byte("vrfSeed")))

		v := new(big.Float).SetInt(new(big.Int).SetBytes(hash[:]))

		q := new(big.Float).Quo(v, maxHash).SetPrec(10)

		threshold, _ := helpers.ExtractUInt64(0, appState.State.GetContractValue(contractAddr, []byte("committeeSize")))
		shouldBeError := false
		if v, _ := q.Float64(); v < 1-float64(threshold)/float64(appState.ValidatorsCache.NetworkSize()) {
			shouldBeError = true
		}

		salt := addr.Bytes()
		voteHash := crypto.Hash(append(common.ToBytes(winnerVote), salt...))
		callAttach = attachments.CreateCallContractAttachment("sendVoteProof", voteHash[:], proof)
		payload, _ = callAttach.ToBytes()
		tx = &types.Transaction{
			Epoch:        0,
			AccountNonce: 2,
			To:           &contractAddr,
			Type:         types.CallContract,
			Payload:      payload,
		}
		tx, _ = types.SignTx(tx, key)

		e = env.NewEnvImp(appState, createHeader(4, 21), gas, secStore)
		contract = NewFactEvidenceContract(env.NewCallContextImpl(tx), e)
		err := contract.Call(callAttach.Method, callAttach.Args...)
		if shouldBeError {
			e.Reset()
			require.Error(t, err)
		} else {
			e.Commit()
			require.NoError(t, err)
			votedIdentities[addr] = struct{}{}
		}
	}

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
		tx, _ = types.SignTx(tx, key)
		e = env.NewEnvImp(appState, createHeader(4320*2, 21), gas, secStore)
		contract = NewFactEvidenceContract(env.NewCallContextImpl(tx), e)

		err := contract.Call(callAttach.Method, callAttach.Args...)
		if _, ok := votedIdentities[addr]; !ok {
			e.Reset()
			require.Error(t, err)
		} else {
			e.Commit()
			require.NoError(t, err)
		}
	}

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
	e = env.NewEnvImp(appState, createHeader(4320*2, 21), gas, secStore)
	contract = NewFactEvidenceContract(env.NewCallContextImpl(tx), e)

	require.NoError(t, contract.Call(callAttach.Method, callAttach.Args...))
	e.Commit()
	t.Logf("finish voting, gas=%v", gas.UsedGas)

	require.Equal(t, []byte{winnerVote}, appState.State.GetContractValue(contractAddr, []byte("result")))

	for addr, _ := range votedIdentities {
		require.True(t, appState.State.GetBalance(addr).Sign() == 1)
	}
	require.True(t, appState.State.GetBalance(contractAddr).Sign() == 0)

	//terminate
	destAddr := common.Address{0x2}
	terminateAttach := attachments.CreateTerminateContractAttachment(destAddr.Bytes())
	payload, _ = terminateAttach.ToBytes()
	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &contractAddr,
		Type:         types.TerminateContract,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)

	e = env.NewEnvImp(appState, createHeader(4320*2, 21), gas, secStore)
	contract = NewFactEvidenceContract(env.NewCallContextImpl(tx), e)
	require.NoError(t, contract.Terminate(terminateAttach.Args...))
	e.Commit()
	require.Equal(t, 0, appState.State.GetBalance(destAddr).Cmp(common.DnaBase))
	require.Nil(t, appState.State.GetCodeHash(contractAddr))
	require.Equal(t,0 , appState.State.GetStakeBalance(contractAddr).Sign())

	require.Nil(t, appState.State.GetContractValue(contractAddr, []byte("vrfSeed")))
}
