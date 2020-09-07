package env

import (
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
	"github.com/stretchr/testify/require"
	db2 "github.com/tendermint/tm-db"
	"math/big"
	"math/rand"
	"testing"
)

func TestEnvImp_Caches(t *testing.T) {

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
		Type:         types.DeployContract,
		Amount:       common.DnaBase,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)
	ctx := NewDeployContextImpl(tx, attachment.CodeHash)

	db := db2.NewMemDB()
	appState := appstate.NewAppState(db, eventbus.New())

	secStore := secstore.NewSecStore()

	gas := &GasCounter{gasLimit: -1}

	env := NewEnvImp(appState, createHeader(3, 21), gas, secStore)

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

}
