package blockchain

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

func Test_ApplyBlockRewards(t *testing.T) {
	chain, _, _ := NewTestBlockchain(false, nil)

	header := &types.ProposedHeader{
		Height:         2,
		ParentHash:     chain.Head.Hash(),
		Time:           new(big.Int).SetInt64(time.Now().UTC().Unix()),
		ProposerPubKey: chain.pubKey,
		TxHash:         types.DeriveSha(types.Transactions([]*types.Transaction{})),
		Coinbase:       chain.coinBaseAddress,
	}

	block := &types.Block{
		Header: &types.Header{
			ProposedHeader: header,
		},
		Body: &types.Body{
			Transactions: []*types.Transaction{},
		},
	}
	fee := new(big.Int)
	fee.Mul(big.NewInt(1e+18), big.NewInt(100))

	appState := chain.appState.Readonly(1)
	chain.applyBlockRewards(fee, appState, block, chain.Head)

	stake := decimal.NewFromBigInt(chain.config.Consensus.BlockReward, 0)
	stake = stake.Mul(decimal.NewFromFloat32(chain.config.Consensus.StakeRewardRate))
	intStake := math.ToInt(&stake)

	burnFee := decimal.NewFromBigInt(fee, 0)
	coef := decimal.NewFromFloat32(0.9)

	burnFee = burnFee.Mul(coef)
	intBurn := math.ToInt(&burnFee)
	intFeeReward := new(big.Int)
	intFeeReward.Sub(fee, intBurn)

	expectedBalance := big.NewInt(0)
	expectedBalance.Add(expectedBalance, chain.config.Consensus.BlockReward)
	expectedBalance.Add(expectedBalance, intFeeReward)
	expectedBalance.Sub(expectedBalance, intStake)

	require.Equal(t, 0, expectedBalance.Cmp(appState.State.GetBalance(chain.coinBaseAddress)))
	require.Equal(t, 0, intStake.Cmp(appState.State.GetStakeBalance(chain.coinBaseAddress)))
}

func Test_ApplyInviteTx(t *testing.T) {
	chain, _, _ := NewTestBlockchain(false, nil)
	stateDb := chain.appState.State

	key, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	receiver := crypto.PubkeyToAddress(key2.PublicKey)

	const balance = 200000
	b := new(big.Int).SetInt64(int64(balance))
	account := stateDb.GetOrNewAccountObject(addr)
	account.SetBalance(b.Mul(b, common.DnaBase))
	id := stateDb.GetOrNewIdentityObject(addr)
	id.AddInvite(1)

	tx := &types.Transaction{
		Type:         types.InviteTx,
		Amount:       big.NewInt(1e+18),
		AccountNonce: 1,
		To:           &receiver,
	}

	signed, _ := types.SignTx(tx, key)

	chain.ApplyTxOnState(chain.appState, signed)

	require.Equal(t, uint8(0), stateDb.GetInvites(addr))
	require.Equal(t, state.Invite, stateDb.GetIdentityState(receiver))
	require.Equal(t, -1, big.NewInt(0).Cmp(stateDb.GetBalance(receiver)))
}

func Test_ApplyActivateTx(t *testing.T) {
	chain, appState, _ := NewTestBlockchain(false, nil)

	key, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()
	sender := crypto.PubkeyToAddress(key.PublicKey)

	receiver := crypto.PubkeyToAddress(key2.PublicKey)

	const balance = 200000
	b := new(big.Int).SetInt64(int64(balance))
	account := appState.State.GetOrNewAccountObject(sender)
	account.SetBalance(b.Mul(b, common.DnaBase))
	id := appState.State.GetOrNewIdentityObject(sender)
	id.SetState(state.Invite)

	tx := &types.Transaction{
		Type:         types.ActivationTx,
		Amount:       big.NewInt(0),
		AccountNonce: 1,
		To:           &receiver,
	}

	signed, _ := types.SignTx(tx, key)

	chain.ApplyTxOnState(chain.appState, signed)
	require.Equal(t, state.Killed, appState.State.GetIdentityState(sender))
	require.Equal(t, 0, big.NewInt(0).Cmp(appState.State.GetBalance(sender)))

	require.Equal(t, state.Candidate, appState.State.GetIdentityState(receiver))
	require.Equal(t, -1, big.NewInt(0).Cmp(appState.State.GetBalance(receiver)))
}

func Test_ApplyKillTx(t *testing.T) {
	require := require.New(t)
	chain, appState, _ := NewTestBlockchain(true, nil)

	key, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()
	sender := crypto.PubkeyToAddress(key.PublicKey)

	receiver := crypto.PubkeyToAddress(key2.PublicKey)

	balance := new(big.Int).Mul(big.NewInt(50), common.DnaBase)
	stake := new(big.Int).Mul(big.NewInt(100), common.DnaBase)

	account := appState.State.GetOrNewAccountObject(sender)
	account.SetBalance(balance)

	id := appState.State.GetOrNewIdentityObject(sender)
	id.SetStake(stake)
	id.SetState(state.Invite)

	amount := new(big.Int).Mul(big.NewInt(1), common.DnaBase)

	tx := &types.Transaction{
		Type:         types.KillTx,
		Amount:       amount,
		AccountNonce: 1,
		To:           &receiver,
	}

	signed, _ := types.SignTx(tx, key)

	fee := types.CalculateFee(chain.appState.ValidatorsCache.NetworkSize(), tx)

	chain.ApplyTxOnState(chain.appState, signed)

	require.Equal(state.Killed, appState.State.GetIdentityState(sender))
	require.Equal(new(big.Int).Sub(balance, amount), appState.State.GetBalance(sender))

	require.Equal(new(big.Int).Add(new(big.Int).Sub(stake, fee), amount), appState.State.GetBalance(receiver))
}

type testCase struct {
	data     []*big.Int
	expected []*big.Int
}

func Test_CalculatePenalty(t *testing.T) {
	require := require.New(t)

	cases := []testCase{
		{
			data:     []*big.Int{big.NewInt(1000), big.NewInt(500), big.NewInt(900)},
			expected: []*big.Int{big.NewInt(100), big.NewInt(500), big.NewInt(900)},
		},
		{
			data:     []*big.Int{big.NewInt(1000), big.NewInt(500), big.NewInt(1200)},
			expected: []*big.Int{big.NewInt(0), big.NewInt(300), big.NewInt(1200)},
		},
		{
			data:     []*big.Int{big.NewInt(1000), big.NewInt(500), big.NewInt(1800)},
			expected: []*big.Int{big.NewInt(0), big.NewInt(0), big.NewInt(1500)},
		},
		{
			data:     []*big.Int{big.NewInt(1000), big.NewInt(500), big.NewInt(1500)},
			expected: []*big.Int{big.NewInt(0), big.NewInt(0), big.NewInt(1500)},
		},
		{
			data:     []*big.Int{big.NewInt(1000), big.NewInt(500), big.NewInt(2600)},
			expected: []*big.Int{big.NewInt(0), big.NewInt(0), big.NewInt(1500)},
		},
		{
			data:     []*big.Int{big.NewInt(1000), big.NewInt(500), nil},
			expected: []*big.Int{big.NewInt(1000), big.NewInt(500), nil},
		},
	}

	for i, item := range cases {
		a, b, c := calculatePenalty(item.data[0], item.data[1], item.data[2])

		require.Equal(0, item.expected[0].Cmp(a), "balance is wrong, case#%v", i+1)
		require.Equal(0, item.expected[1].Cmp(b), "stake is wrong, case#%v", i+1)

		if item.expected[2] == nil {
			require.Equal(item.expected[2], c, "penalty is wrong, case#%v", i+1)
		} else {
			require.Equal(0, item.expected[2].Cmp(c), "penalty is wrong, case#%v", i+1)
		}
	}

}
