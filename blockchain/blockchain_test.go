package blockchain

import (
	fee2 "github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/tests"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

func Test_ApplyBlockRewards(t *testing.T) {
	chain, _, _, _ := NewTestBlockchain(false, nil)

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
	tips := new(big.Int).Mul(big.NewInt(1e+18), big.NewInt(10))

	appState := chain.appState.Readonly(1)
	chain.applyBlockRewards(fee, tips, appState, block, chain.Head)

	burnFee := decimal.NewFromBigInt(fee, 0)
	coef := decimal.NewFromFloat32(0.9)

	burnFee = burnFee.Mul(coef)
	intBurn := math.ToInt(burnFee)
	intFeeReward := new(big.Int)
	intFeeReward.Sub(fee, intBurn)

	totalReward := big.NewInt(0).Add(chain.config.Consensus.BlockReward, intFeeReward)
	totalReward.Add(totalReward, tips)
	_, stake := splitReward(totalReward, chain.config.Consensus)

	expectedBalance := big.NewInt(0)
	expectedBalance.Add(expectedBalance, chain.config.Consensus.BlockReward)
	expectedBalance.Add(expectedBalance, intFeeReward)
	expectedBalance.Add(expectedBalance, tips)
	expectedBalance.Sub(expectedBalance, stake)

	require.Equal(t, 0, expectedBalance.Cmp(appState.State.GetBalance(chain.coinBaseAddress)))
	require.Equal(t, 0, stake.Cmp(appState.State.GetStakeBalance(chain.coinBaseAddress)))
}

func Test_ApplyInviteTx(t *testing.T) {
	chain, _, _, _ := NewTestBlockchain(false, nil)
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
	chain, appState, _, _ := NewTestBlockchain(false, nil)

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
	chain, appState, _, _ := NewTestBlockchain(true, nil)

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

	chain.appState.State.SetFeePerByte(new(big.Int).Div(big.NewInt(1e+18), big.NewInt(1000)))
	fee := fee2.CalculateFee(chain.appState.ValidatorsCache.NetworkSize(), chain.appState.State.FeePerByte(), tx)

	chain.ApplyTxOnState(chain.appState, signed)

	require.Equal(state.Killed, appState.State.GetIdentityState(sender))
	require.Equal(new(big.Int).Sub(balance, amount), appState.State.GetBalance(sender))

	require.Equal(new(big.Int).Add(new(big.Int).Sub(stake, fee), amount), appState.State.GetBalance(receiver))
}

func Test_ApplyKillInviteeTx(t *testing.T) {
	chain, appState, _, _ := NewTestBlockchain(true, nil)

	inviterKey, _ := crypto.GenerateKey()
	inviter := crypto.PubkeyToAddress(inviterKey.PublicKey)
	invitee := tests.GetRandAddr()
	anotherInvitee := tests.GetRandAddr()

	appState.State.SetInviter(invitee, inviter, common.Hash{})
	appState.State.SetInviter(anotherInvitee, inviter, common.Hash{})
	appState.State.AddInvitee(inviter, invitee, common.Hash{})
	appState.State.AddInvitee(inviter, anotherInvitee, common.Hash{})

	appState.State.SetInvites(inviter, 0)
	appState.State.SetState(inviter, state.Verified)
	appState.State.SetState(invitee, state.Candidate)

	appState.State.GetOrNewAccountObject(inviter).SetBalance(new(big.Int).Mul(big.NewInt(50), common.DnaBase))
	appState.State.GetOrNewIdentityObject(invitee).SetStake(new(big.Int).Mul(big.NewInt(10), common.DnaBase))

	tx := &types.Transaction{
		Type:         types.KillInviteeTx,
		AccountNonce: 1,
		To:           &invitee,
	}
	signedTx, _ := types.SignTx(tx, inviterKey)

	chain.appState.State.SetFeePerByte(new(big.Int).Div(big.NewInt(1e+18), big.NewInt(1000)))
	fee := fee2.CalculateFee(chain.appState.ValidatorsCache.NetworkSize(), chain.appState.State.FeePerByte(), tx)

	chain.ApplyTxOnState(chain.appState, signedTx)

	require.Equal(t, uint8(1), appState.State.GetInvites(inviter))
	require.Equal(t, 1, len(appState.State.GetInvitees(inviter)))
	require.Equal(t, anotherInvitee, appState.State.GetInvitees(inviter)[0].Address)
	newBalance := new(big.Int).Mul(big.NewInt(60), common.DnaBase)
	newBalance.Sub(newBalance, fee)
	require.Equal(t, newBalance, appState.State.GetBalance(inviter))

	require.Equal(t, state.Killed, appState.State.GetIdentityState(invitee))
	require.Nil(t, appState.State.GetInviter(invitee))
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

func Test_applyNextBlockFee(t *testing.T) {
	conf := GetDefaultConsensusConfig(false)
	conf.MinFeePerByte = big.NewInt(0).Div(common.DnaBase, big.NewInt(100))
	chain, _, _, _ := NewTestBlockchainWithConfig(true, conf, &config.ValidationConfig{}, nil, -1, -1)

	appState := chain.appState.Readonly(1)

	block := generateBlock(4, 10000) // block size 770008
	chain.applyNextBlockFee(appState, block)
	require.Equal(t, big.NewInt(10585842132568359), appState.State.FeePerByte())

	block = generateBlock(5, 5000) // block size 385008
	chain.applyNextBlockFee(appState, block)
	require.Equal(t, big.NewInt(10234318711227387), appState.State.FeePerByte())

	block = generateBlock(6, 0)
	chain.applyNextBlockFee(appState, block)
	require.Equal(t, chain.config.Consensus.MinFeePerByte, appState.State.FeePerByte())
}

type txWithTimestamp struct {
	tx        *types.Transaction
	timestamp uint64
}

func TestBlockchain_SaveTxs(t *testing.T) {
	require := require.New(t)

	chain, _, _, key := NewTestBlockchain(true, nil)

	key2, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	txs := []txWithTimestamp{
		{tx: tests.GetFullTx(1, 1, key, types.SendTx, nil, &addr), timestamp: 10},
		{tx: tests.GetFullTx(2, 1, key, types.SendTx, nil, &addr), timestamp: 20},
		{tx: tests.GetFullTx(4, 1, key, types.SendTx, nil, &addr), timestamp: 30},
		{tx: tests.GetFullTx(5, 1, key, types.SendTx, nil, &addr), timestamp: 35},
		{tx: tests.GetFullTx(6, 1, key, types.SendTx, nil, &addr), timestamp: 50},
		{tx: tests.GetFullTx(7, 1, key, types.SendTx, nil, &addr), timestamp: 80},
		{tx: tests.GetFullTx(9, 1, key, types.SendTx, nil, &addr), timestamp: 80},
		{tx: tests.GetFullTx(9, 1, key, types.SendTx, nil, &addr), timestamp: 456},
		{tx: tests.GetFullTx(10, 1, key, types.SendTx, nil, &addr), timestamp: 456},
		{tx: tests.GetFullTx(1, 2, key, types.SendTx, nil, &addr), timestamp: 500},
		{tx: tests.GetFullTx(2, 2, key, types.SendTx, nil, &addr), timestamp: 500},

		{tx: tests.GetFullTx(1, 1, key2, types.SendTx, nil, &addr), timestamp: 20},
		{tx: tests.GetFullTx(8, 1, key2, types.SendTx, nil, &addr), timestamp: 80},
		{tx: tests.GetFullTx(10, 1, key2, types.SendTx, nil, &addr), timestamp: 80},
		{tx: tests.GetFullTx(4, 1, key2, types.SendTx, nil, &addr), timestamp: 456},
	}

	for _, item := range txs {
		header := &types.Header{
			ProposedHeader: &types.ProposedHeader{
				Time:       new(big.Int).SetUint64(item.timestamp),
				FeePerByte: big.NewInt(1),
			},
		}
		chain.SaveTxs(header, []*types.Transaction{item.tx})
	}

	data, token := chain.ReadTxs(addr, 5, nil)

	require.Equal(5, len(data))
	require.Equal(uint32(2), data[0].Tx.AccountNonce)
	require.Equal(uint32(1), data[1].Tx.AccountNonce)
	require.Equal(uint32(10), data[2].Tx.AccountNonce)
	require.Equal(uint32(9), data[3].Tx.AccountNonce)
	require.Equal(uint32(4), data[4].Tx.AccountNonce)
	require.Equal(uint64(456), data[4].Timestamp)
	require.NotNil(token)

	data, token = chain.ReadTxs(addr, 4, token)

	require.Equal(4, len(data))
	require.Equal(uint32(10), data[0].Tx.AccountNonce)
	require.Equal(uint32(9), data[1].Tx.AccountNonce)
	require.Equal(uint32(8), data[2].Tx.AccountNonce)
	require.Equal(uint32(7), data[3].Tx.AccountNonce)
	require.NotNil(token)

	data, token = chain.ReadTxs(addr, 10, token)

	require.Equal(6, len(data))
	require.Equal(uint32(6), data[0].Tx.AccountNonce)
	require.Equal(uint32(5), data[1].Tx.AccountNonce)
	require.Equal(uint32(4), data[2].Tx.AccountNonce)
	require.Equal(uint32(2), data[3].Tx.AccountNonce)
	require.Equal(uint32(1), data[4].Tx.AccountNonce)
	require.Equal(uint32(1), data[5].Tx.AccountNonce)
	require.Equal(uint64(20), data[4].Timestamp)
	require.Equal(uint64(10), data[5].Timestamp)
	require.Nil(token)
}

func generateBlock(height uint64, txsCount int) *types.Block {
	txs := make([]*types.Transaction, 0)
	for i := 0; i < txsCount; i++ {
		tx := &types.Transaction{
			Type: types.SendTx,
		}
		key, _ := crypto.GenerateKey()
		signedTx, _ := types.SignTx(tx, key)
		txs = append(txs, signedTx)
	}
	header := &types.ProposedHeader{
		Height: height,
		TxHash: types.DeriveSha(types.Transactions(txs)),
	}

	block := &types.Block{
		Header: &types.Header{
			ProposedHeader: header,
		},
		Body: &types.Body{
			Transactions: txs,
		},
	}

	return block
}

func TestBlockchain_GetTopBlockHashes(t *testing.T) {
	chain, _ := NewTestBlockchainWithBlocks(90, 9)
	hashes := chain.GetTopBlockHashes(50)
	require.Len(t, hashes, 50)
	require.Equal(t, hashes[0], chain.GetBlockHeaderByHeight(100).Hash())
	require.Equal(t, hashes[49], chain.GetBlockHeaderByHeight(51).Hash())
}

func TestBlockchain_ReadBlockForForkedPeer(t *testing.T) {
	chain, _ := NewTestBlockchainWithBlocks(90, 9)
	hashes := chain.GetTopBlockHashes(50)
	for i := 0; i < len(hashes)-1; i++ {
		hashes[i] = common.Hash{byte(i)}
	}
	bundles := chain.ReadBlockForForkedPeer(hashes)
	require.Len(t, bundles, 49)
}

func Test_ApplyBurnTx(t *testing.T) {
	senderKey, _ := crypto.GenerateKey()
	balance := new(big.Int).Mul(common.DnaBase, big.NewInt(100))

	alloc := make(map[common.Address]config.GenesisAllocation)
	sender := crypto.PubkeyToAddress(senderKey.PublicKey)
	alloc[sender] = config.GenesisAllocation{
		Balance: balance,
	}

	chain, _, _, _ := NewTestBlockchain(true, alloc)

	tx := &types.Transaction{
		Type:         types.BurnTx,
		AccountNonce: 1,
		Amount:       new(big.Int).Mul(common.DnaBase, big.NewInt(10)),
		Tips:         new(big.Int).Mul(common.DnaBase, big.NewInt(1)),
	}
	signedTx, _ := types.SignTx(tx, senderKey)

	appState := chain.appState
	appState.State.SetFeePerByte(new(big.Int).Div(common.DnaBase, big.NewInt(1000)))
	fee := fee2.CalculateFee(appState.ValidatorsCache.NetworkSize(), appState.State.FeePerByte(), tx)
	expectedBalance := new(big.Int).Mul(big.NewInt(89), common.DnaBase)
	expectedBalance.Sub(expectedBalance, fee)

	chain.ApplyTxOnState(appState, signedTx)

	require.Equal(t, 1, fee.Sign())
	require.Equal(t, expectedBalance, appState.State.GetBalance(sender))
}

func Test_Blockchain_SaveBurntCoins(t *testing.T) {
	require := require.New(t)

	chain, _, _, key := NewTestBlockchain(true, nil)
	chain.config.Blockchain.BurnTxRange = 3

	key2, _ := crypto.GenerateKey()

	createHeader := func(height uint64) *types.Header {
		return &types.Header{
			ProposedHeader: &types.ProposedHeader{
				Height: height,
				Time:   big.NewInt(0),
			},
		}
	}

	// Block height=1
	chain.SaveTxs(createHeader(1), []*types.Transaction{
		tests.GetFullTx(0, 0, key2, types.BurnTx, big.NewInt(4), nil),
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(2), nil),
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(3), nil),
	})

	addr := crypto.PubkeyToAddress(key.PublicKey)
	addr2 := crypto.PubkeyToAddress(key2.PublicKey)

	burntCoins := chain.ReadTotalBurntCoins()
	require.Equal(2, len(burntCoins))
	require.Equal(addr, burntCoins[0].Address)
	require.Equal(big.NewInt(5), burntCoins[0].Amount)
	require.Equal(addr2, burntCoins[1].Address)
	require.Equal(big.NewInt(4), burntCoins[1].Amount)

	// Block height=2
	chain.SaveTxs(createHeader(2), []*types.Transaction{
		tests.GetFullTx(0, 0, key2, types.BurnTx, big.NewInt(5), nil),
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(1), nil),
		tests.GetFullTx(0, 0, key, types.SendTx, big.NewInt(2), nil),
	})

	burntCoins = chain.ReadTotalBurntCoins()
	require.Equal(2, len(burntCoins))
	require.Equal(addr2, burntCoins[0].Address)
	require.Equal(big.NewInt(9), burntCoins[0].Amount)
	require.Equal(addr, burntCoins[1].Address)
	require.Equal(big.NewInt(6), burntCoins[1].Amount)

	// Block height=4
	chain.SaveTxs(createHeader(4), []*types.Transaction{
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(3), nil),
	})

	burntCoins = chain.ReadTotalBurntCoins()
	require.Equal(2, len(burntCoins))
	require.Equal(addr2, burntCoins[0].Address)
	require.Equal(big.NewInt(5), burntCoins[0].Amount)
	require.Equal(addr, burntCoins[1].Address)
	require.Equal(big.NewInt(4), burntCoins[1].Amount)

	// Block height=7
	chain.SaveTxs(createHeader(7), []*types.Transaction{
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(1), nil),
	})

	burntCoins = chain.ReadTotalBurntCoins()
	require.Equal(1, len(burntCoins))
	require.Equal(addr, burntCoins[0].Address)
	require.Equal(big.NewInt(1), burntCoins[0].Amount)
}
