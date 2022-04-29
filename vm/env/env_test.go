package env

import (
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/stretchr/testify/require"
	db2 "github.com/tendermint/tm-db"
	"math/big"
	"math/rand"
	"testing"
)

func TestEnvImp_basicMethods(t *testing.T) {

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

	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)
	//addr := crypto.PubkeyToAddress(key.PublicKey)
	attachment := attachments.CreateDeployContractAttachment(common.Hash{0x1})
	payload, _ := attachment.ToBytes()

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: 1,
		Type:         types.DeployContractTx,
		Amount:       common.DnaBase,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)
	ctx := NewDeployContextImpl(tx, attachment.CodeHash)

	db := db2.NewMemDB()
	appState, _ := appstate.NewAppState(db, eventbus.New())

	gas := &GasCounter{gasLimit: -1}

	env := NewEnvImp(appState, createHeader(3, 21), gas, nil)

	require.Error(t, env.Send(ctx, common.Address{0x1}, big.NewInt(1)))
	env.Reset()

	appState.State.AddBalance(ctx.ContractAddr(), big.NewInt(100))
	appState.State.SetContractValue(ctx.ContractAddr(), []byte{0x2}, []byte{0x2})
	appState.State.SetContractValue(ctx.ContractAddr(), []byte{0x5}, []byte{0x5})

	require.NoError(t, env.Send(ctx, common.Address{0x1}, big.NewInt(1)))

	require.Len(t, env.balancesCache, 2)

	require.True(t, big.NewInt(1).Cmp(env.balancesCache[common.Address{0x1}]) == 0)

	require.True(t, big.NewInt(99).Cmp(env.balancesCache[ctx.ContractAddr()]) == 0)

	require.True(t, env.Balance(ctx.ContractAddr()).Cmp(big.NewInt(99)) == 0)

	env.Deploy(ctx)

	require.Len(t, env.deployedContractCache, 1)

	require.Equal(t, attachment.CodeHash, env.deployedContractCache[ctx.ContractAddr()].CodeHash)

	require.True(t, common.DnaBase.Cmp(env.deployedContractCache[ctx.ContractAddr()].Stake) == 0)

	env.SetValue(ctx, []byte{0x1}, []byte{0x1})
	env.SetValue(ctx, []byte{0x2}, []byte{0x2})
	env.SetValue(ctx, []byte{0x3}, []byte{0x3})

	require.Len(t, env.contractStoreCache[ctx.ContractAddr()], 3)

	require.Equal(t, []byte{0x1}, env.GetValue(ctx, []byte{0x1}))
	require.Equal(t, []byte{0x2}, env.GetValue(ctx, []byte{0x2}))
	require.Equal(t, []byte{0x3}, env.GetValue(ctx, []byte{0x3}))

	env.RemoveValue(ctx, []byte{0x1})
	env.RemoveValue(ctx, []byte{0x2})

	require.True(t, env.contractStoreCache[ctx.ContractAddr()][string([]byte{0x1})].removed)
	require.True(t, env.contractStoreCache[ctx.ContractAddr()][string([]byte{0x2})].removed)

	env.SetValue(ctx, []byte{0x2}, []byte{0x2})
	require.False(t, env.contractStoreCache[ctx.ContractAddr()][string([]byte{0x2})].removed)

	cnt := 0
	env.Iterate(ctx, nil, nil, func(key []byte, value []byte) (stopped bool) {
		cnt++
		return false
	})
	require.Equal(t, 3, cnt)

	env.Iterate(ctx, nil, []byte{0x2}, func(key []byte, value []byte) (stopped bool) {
		require.Equal(t, []byte{0x2}, key)
		require.Equal(t, []byte{0x2}, value)
		return false
	})

	env.Event("test1", []byte{0x1}, []byte{0x2})
	env.Event("test2", []byte{0x2})

	events := env.Commit()
	env.Reset()
	require.Len(t, events, 2)
	require.Equal(t, events[0].EventName, "test1")
	require.Equal(t, events[0].Data, [][]byte{{0x1}, {0x2}})
	require.Equal(t, events[1].EventName, "test2")
	require.Equal(t, events[1].Data, [][]byte{{0x2}})

	require.Equal(t, 0, appState.State.GetBalance(common.Address{0x1}).Cmp(big.NewInt(1)))
	require.Equal(t, 0, appState.State.GetBalance(ctx.ContractAddr()).Cmp(big.NewInt(99)))

	require.Equal(t, 0, appState.State.GetContractStake(ctx.ContractAddr()).Cmp(common.DnaBase))
	require.Equal(t, attachment.CodeHash, *appState.State.GetCodeHash(ctx.ContractAddr()))

	attach := attachments.CreateTerminateContractAttachment()
	payload, _ = attach.ToBytes()

	contract := ctx.ContractAddr()
	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &contract,
		Type:         types.TerminateContractTx,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)
	terminateCtx := NewCallContextImpl(tx, attachment.CodeHash)
	env.Terminate(terminateCtx, [][]byte{{0x2}}, common.Address{0x04})
	env.Commit()

	require.Equal(t, (*common.Hash)(nil), appState.State.GetCodeHash(ctx.ContractAddr()))
	require.True(t, appState.State.GetContractStake(ctx.ContractAddr()) == nil)
	require.Equal(t, 0, appState.State.GetBalance(common.Address{0x4}).Cmp(big.NewInt(0).Quo(common.DnaBase, big.NewInt(2))))

	cnt = 0
	env.Iterate(ctx, nil, nil, func(key []byte, value []byte) (stopped bool) {
		require.Equal(t, []byte{0x2}, key)
		require.Equal(t, []byte{0x2}, value)
		cnt++
		return false
	})
	require.Equal(t, 1, cnt)
}
