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
	attachment := attachments.CreateDeployContractAttachment(FactCheckingContract, []byte{0x1}, common.ToBytes(uint64(10)))
	payload, err := attachment.ToBytes()
	require.NoError(t, err)

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: 1,
		Type:         types.DeployContract,
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

	// deploy
	contract := NewFactCheckingContract(ctx, env.NewEnvImp(appState, createHeader(2, 1)))
	require.NoError(t, contract.Deploy(attachment.Args...))

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

	contract = NewFactCheckingContract(env.NewCallContextImpl(txNotOwner), env.NewEnvImp(appState, createHeader(3, 21)))
	require.Error(t, contract.Call("startVoting"))

	contract = NewFactCheckingContract(env.NewCallContextImpl(tx), env.NewEnvImp(appState, createHeader(3, 21)))
	require.Error(t, contract.Call("startVoting"))

	appState.State.SetBalance(contractAddr, big.NewInt(200000))

	require.NoError(t, contract.Call("startVoting"))

	require.Equal(t, common.ToBytes(uint64(1)), appState.State.GetContractValue(contractAddr, []byte("state")))
	require.Equal(t, big.NewInt(10000).Bytes(), appState.State.GetContractValue(contractAddr, []byte("votingMinPayment")))

	seed := types.Seed{}
	seed.SetBytes(common.ToBytes(uint64(3)))
	require.Equal(t, seed.Bytes(), appState.State.GetContractValue(contractAddr, []byte("vrfSeed")))
	require.Equal(t, common.ToBytes(uint64(100)), appState.State.GetContractValue(contractAddr, []byte("vrfThreshold")))

	// send proofs
	for _, key := range identities {

		addr := crypto.PubkeyToAddress(key.PublicKey)

		signer, _ := p256.NewVRFSigner(key)

		hash, proof := signer.Evaluate(appState.State.GetContractValue(contractAddr, []byte("vrfSeed")))

		v := new(big.Float).SetInt(new(big.Int).SetBytes(hash[:]))

		q := new(big.Float).Quo(v, maxHash).SetPrec(10)

		threshold, _ := helpers.ExtractUInt64(0, appState.State.GetContractValue(contractAddr, []byte("vrfThreshold")))
		shouldBeError := false
		if v, _ := q.Float64(); v < 1-float64(threshold)/float64(appState.ValidatorsCache.NetworkSize()) {
			shouldBeError = true
		}

		salt := addr.Bytes()

		callAttach = attachments.CreateCallContractAttachment("sendVoteProof", salt, proof)
		payload, _ = callAttach.ToBytes()
		tx = &types.Transaction{
			Epoch:        0,
			AccountNonce: 2,
			To:           &contractAddr,
			Type:         types.CallContract,
			Payload:      payload,
		}
		tx, _ = types.SignTx(tx, key)

		contract = NewFactCheckingContract(env.NewCallContextImpl(tx), env.NewEnvImp(appState, createHeader(4, 21)))
		err := contract.Call(callAttach.Method, callAttach.Args...)
		if shouldBeError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
}
