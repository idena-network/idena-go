package blockchain

import (
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/math"
	"idena-go/core/state"
	"idena-go/crypto"
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
		ProposerPubKey: crypto.FromECDSAPub(chain.pubKey),
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

	appState := chain.appState.ForCheck(1)
	chain.applyBlockRewards(fee, appState, block)

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
	require.Equal(t, uint8(1), appState.State.GetInvites(chain.coinBaseAddress))
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

	chain.applyTxOnState(chain.appState, signed)

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

	chain.applyTxOnState(chain.appState, signed)
	require.Equal(t, state.Killed, appState.State.GetIdentityState(sender))
	require.Equal(t, 0, big.NewInt(0).Cmp(appState.State.GetBalance(sender)))

	require.Equal(t, state.Candidate, appState.State.GetIdentityState(receiver))
	require.Equal(t, -1, big.NewInt(0).Cmp(appState.State.GetBalance(receiver)))
}

func Test_getNextValidationTime(t *testing.T) {
	require := require.New(t)

	now, _ := time.Parse(time.RFC3339, "2019-01-01T05:05:00+00:00")
	t1, _ := time.Parse(time.RFC3339, "2019-01-01T05:00:00+00:00")

	require.Equal(t1.Add(time.Minute*6), getNextValidationTime(time.Minute, t1, now))

	require.Equal(t1.Add(time.Hour), getNextValidationTime(time.Hour, t1, now))

	require.Equal(t1.Add(time.Second*59*6), getNextValidationTime(time.Second*59, t1, now))

}
