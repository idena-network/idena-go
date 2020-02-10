package blockchain

import (
	"fmt"
	"github.com/idena-network/idena-go/blockchain/attachments"
	fee2 "github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
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
	chain.applyBlockRewards(fee, tips, appState, block, chain.Head, nil)

	burnFee := decimal.NewFromBigInt(fee, 0)
	coef := decimal.NewFromFloat32(0.9)

	burnFee = burnFee.Mul(coef)
	intBurn := math.ToInt(burnFee)
	intFeeReward := new(big.Int)
	intFeeReward.Sub(fee, intBurn)

	totalReward := big.NewInt(0).Add(chain.config.Consensus.BlockReward, intFeeReward)
	totalReward.Add(totalReward, chain.config.Consensus.FinalCommitteeReward)
	totalReward.Add(totalReward, tips)

	expectedBalance, stake := splitReward(totalReward, chain.config.Consensus)

	fmt.Printf("%v\n%v", expectedBalance, appState.State.GetBalance(chain.coinBaseAddress))

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

	chain.ApplyTxOnState(chain.appState, signed, nil)

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

	chain.ApplyTxOnState(chain.appState, signed, nil)
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

	chain.ApplyTxOnState(chain.appState, signed, nil)

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

	chain.ApplyTxOnState(chain.appState, signedTx, nil)

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

func Test_applyVrfProposerThreshold(t *testing.T) {
	conf := GetDefaultConsensusConfig(false)
	conf.MinFeePerByte = big.NewInt(0).Div(common.DnaBase, big.NewInt(100))
	chain, _ := NewTestBlockchainWithBlocks(100, 0)

	chain.GenerateEmptyBlocks(10)
	require.Equal(t, 10, chain.appState.State.EmptyBlocksCount())

	chain.GenerateBlocks(5)
	require.Equal(t, 10, chain.appState.State.EmptyBlocksCount())

	chain.GenerateBlocks(100)
	require.Equal(t, 0.5, chain.appState.State.VrfProposerThreshold())
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
		{tx: tests.GetFullTx(1, 1, key, types.SendTx, nil, &addr, nil), timestamp: 10},
		{tx: tests.GetFullTx(2, 1, key, types.SendTx, nil, &addr, nil), timestamp: 20},
		{tx: tests.GetFullTx(4, 1, key, types.SendTx, nil, &addr, nil), timestamp: 30},
		{tx: tests.GetFullTx(5, 1, key, types.SendTx, nil, &addr, nil), timestamp: 35},
		{tx: tests.GetFullTx(6, 1, key, types.SendTx, nil, &addr, nil), timestamp: 50},
		{tx: tests.GetFullTx(7, 1, key, types.SendTx, nil, &addr, nil), timestamp: 80},
		{tx: tests.GetFullTx(9, 1, key, types.SendTx, nil, &addr, nil), timestamp: 80},
		{tx: tests.GetFullTx(9, 1, key, types.SendTx, nil, &addr, nil), timestamp: 456},
		{tx: tests.GetFullTx(10, 1, key, types.SendTx, nil, &addr, nil), timestamp: 456},
		{tx: tests.GetFullTx(1, 2, key, types.SendTx, nil, &addr, nil), timestamp: 500},
		{tx: tests.GetFullTx(2, 2, key, types.SendTx, nil, &addr, nil), timestamp: 500},

		{tx: tests.GetFullTx(1, 1, key2, types.SendTx, nil, &addr, nil), timestamp: 20},
		{tx: tests.GetFullTx(8, 1, key2, types.SendTx, nil, &addr, nil), timestamp: 80},
		{tx: tests.GetFullTx(10, 1, key2, types.SendTx, nil, &addr, nil), timestamp: 80},
		{tx: tests.GetFullTx(4, 1, key2, types.SendTx, nil, &addr, nil), timestamp: 456},
	}

	for _, item := range txs {
		header := &types.Header{
			ProposedHeader: &types.ProposedHeader{
				Time:       new(big.Int).SetUint64(item.timestamp),
				FeePerByte: big.NewInt(1),
			},
		}
		chain.HandleTxs(header, []*types.Transaction{item.tx})
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

	chain.ApplyTxOnState(appState, signedTx, nil)

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
	chain.HandleTxs(createHeader(1), []*types.Transaction{
		tests.GetFullTx(0, 0, key2, types.BurnTx, big.NewInt(3), nil,
			attachments.CreateBurnAttachment("3")),
		tests.GetFullTx(0, 0, key2, types.BurnTx, big.NewInt(4), nil,
			attachments.CreateBurnAttachment("2")),
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(2), nil,
			attachments.CreateBurnAttachment("1")),
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(3), nil,
			attachments.CreateBurnAttachment("1")),
	})

	addr := crypto.PubkeyToAddress(key.PublicKey)
	addr2 := crypto.PubkeyToAddress(key2.PublicKey)

	burntCoins := chain.ReadTotalBurntCoins()
	require.Equal(3, len(burntCoins))
	require.Equal(addr, burntCoins[0].Address)
	require.Equal(big.NewInt(5), burntCoins[0].Amount)

	require.Equal(addr2, burntCoins[1].Address)
	require.Equal("2", burntCoins[1].Key)
	require.Equal(big.NewInt(4), burntCoins[1].Amount)

	require.Equal(addr2, burntCoins[2].Address)
	require.Equal("3", burntCoins[2].Key)
	require.Equal(big.NewInt(3), burntCoins[2].Amount)

	// Block height=2
	chain.HandleTxs(createHeader(2), []*types.Transaction{
		tests.GetFullTx(0, 0, key2, types.BurnTx, big.NewInt(5), nil,
			attachments.CreateBurnAttachment("2")),
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(1), nil,
			attachments.CreateBurnAttachment("1")),
		tests.GetFullTx(0, 0, key, types.SendTx, big.NewInt(2), nil, nil),
	})

	burntCoins = chain.ReadTotalBurntCoins()
	require.Equal(3, len(burntCoins))
	require.Equal(addr2, burntCoins[0].Address)
	require.Equal(big.NewInt(9), burntCoins[0].Amount)
	require.Equal("2", burntCoins[0].Key)

	require.Equal(addr, burntCoins[1].Address)
	require.Equal(big.NewInt(6), burntCoins[1].Amount)

	require.Equal(addr2, burntCoins[2].Address)
	require.Equal("3", burntCoins[2].Key)
	require.Equal(big.NewInt(3), burntCoins[2].Amount)

	// Block height=4
	chain.HandleTxs(createHeader(4), []*types.Transaction{
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(3), nil,
			attachments.CreateBurnAttachment("1")),
	})

	burntCoins = chain.ReadTotalBurntCoins()
	require.Equal(2, len(burntCoins))
	require.Equal(addr2, burntCoins[0].Address)
	require.Equal(big.NewInt(5), burntCoins[0].Amount)
	require.Equal(addr, burntCoins[1].Address)
	require.Equal(big.NewInt(4), burntCoins[1].Amount)

	// Block height=7
	chain.HandleTxs(createHeader(7), []*types.Transaction{
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(1), nil,
			attachments.CreateBurnAttachment("1")),
	})

	burntCoins = chain.ReadTotalBurntCoins()
	require.Equal(1, len(burntCoins))
	require.Equal(addr, burntCoins[0].Address)
	require.Equal(big.NewInt(1), burntCoins[0].Amount)
}

func Test_DeleteFlipTx(t *testing.T) {
	senderKey, _ := crypto.GenerateKey()
	balance := big.NewInt(1_000_000)

	alloc := make(map[common.Address]config.GenesisAllocation)
	sender := crypto.PubkeyToAddress(senderKey.PublicKey)
	alloc[sender] = config.GenesisAllocation{
		Balance: balance,
	}

	chain, _, _, _ := NewTestBlockchain(true, alloc)

	tx := &types.Transaction{
		Type:         types.DeleteFlipTx,
		AccountNonce: 1,
		Amount:       big.NewInt(100),
		MaxFee:       big.NewInt(620_000),
		Tips:         big.NewInt(10),
		Payload:      attachments.CreateDeleteFlipAttachment([]byte{0x1, 0x2, 0x3}),
	}
	signedTx, _ := types.SignTx(tx, senderKey)

	appState := chain.appState
	appState.State.AddFlip(sender, []byte{0x1, 0x2, 0x2}, 0)
	appState.State.AddFlip(sender, []byte{0x1, 0x2, 0x3}, 1)
	appState.State.AddFlip(sender, []byte{0x1, 0x2, 0x4}, 2)
	appState.State.SetFeePerByte(big.NewInt(1))

	fee := fee2.CalculateFee(appState.ValidatorsCache.NetworkSize(), appState.State.FeePerByte(), tx)
	expectedBalance := big.NewInt(999_990)
	expectedBalance.Sub(expectedBalance, fee)

	chain.ApplyTxOnState(appState, signedTx, nil)

	require.Equal(t, 1, fee.Sign())
	require.Equal(t, expectedBalance, appState.State.GetBalance(sender))
	identity := appState.State.GetIdentity(sender)
	require.Equal(t, 2, len(identity.Flips))
}

func Test_Blockchain_OnlineStatusSwitch(t *testing.T) {
	require := require.New(t)
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)
	consensus := GetDefaultConsensusConfig(true)
	consensus.StatusSwitchRange = 10
	cfg := &config.Config{
		Network:   0x99,
		Consensus: consensus,
		GenesisConf: &config.GenesisConf{
			Alloc: map[common.Address]config.GenesisAllocation{
				addr: {
					State: uint8(state.Verified),
				},
			},
			GodAddress:        addr,
			FirstCeremonyTime: 4070908800, //01.01.2099
		},
		Validation: &config.ValidationConfig{},
		Blockchain: &config.BlockchainConfig{},
	}
	chain, state := NewCustomTestBlockchainWithConfig(5, 0, key, cfg)

	//  add pending request to switch online
	tx, _ := chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(2, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true)))
	err := chain.txpool.Add(tx)
	require.NoError(err)

	// apply pending status switch
	chain.GenerateBlocks(1)
	require.Equal(1, len(state.State.StatusSwitchAddresses()))
	require.False(state.IdentityState.IsOnline(addr))

	// fail to switch online again
	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(2, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true)))
	err = chain.txpool.Add(tx)
	require.Error(err, "should not validate tx if switch is already pending")

	// switch status to online
	chain.GenerateBlocks(3)
	require.Equal(uint64(10), chain.Head.Height())
	require.Zero(len(state.State.StatusSwitchAddresses()))
	require.True(state.IdentityState.IsOnline(addr))
	require.True(chain.Head.Flags().HasFlag(types.IdentityUpdate))

	// fail to switch online again
	chain.GenerateBlocks(5)
	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(2, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true)))
	err = chain.txpool.Add(tx)
	require.Error(err, "should not validate tx if identity already has online status")

	// add pending request to switch offline
	chain.GenerateBlocks(4)
	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(2, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(false)))
	err = chain.txpool.Add(tx)
	require.NoError(err)

	// switch status to offline
	chain.GenerateBlocks(1)
	require.Equal(uint64(20), chain.Head.Height())
	require.Zero(len(state.State.StatusSwitchAddresses()))
	require.False(state.IdentityState.IsOnline(addr))
	require.True(chain.Head.Flags().HasFlag(types.IdentityUpdate))

	// add pending request to switch offline
	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(2, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true)))
	err = chain.txpool.Add(tx)
	require.NoError(err)
	chain.GenerateBlocks(1)

	require.Equal(1, len(state.State.StatusSwitchAddresses()))

	// remove pending request to switch online
	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(2, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(false)))
	err = chain.txpool.Add(tx)
	require.NoError(err)
	chain.GenerateBlocks(1)

	require.Zero(len(state.State.StatusSwitchAddresses()))

	// 30th block should not update identity statuses, no pending requests
	chain.GenerateBlocks(8)
	require.Equal(uint64(30), chain.Head.Height())
	require.False(state.IdentityState.IsOnline(addr))
	require.False(chain.Head.Flags().HasFlag(types.IdentityUpdate))

	chain.GenerateBlocks(70)
	require.Equal(uint64(100), chain.Head.Height())
}

func Test_ApplySubmitCeremonyTxs(t *testing.T) {
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)
	consensus := GetDefaultConsensusConfig(true)
	consensus.StatusSwitchRange = 10
	cfg := &config.Config{
		Network:   0x99,
		Consensus: consensus,
		GenesisConf: &config.GenesisConf{
			Alloc: map[common.Address]config.GenesisAllocation{
				addr: {
					State: uint8(state.Verified),
				},
			},
			GodAddress:        addr,
			FirstCeremonyTime: 1, //01.01.2099
		},
		Validation: &config.ValidationConfig{
			ShortSessionDuration: 1 * time.Second,
			LongSessionDuration:  1 * time.Second,
		},
		Blockchain: &config.BlockchainConfig{},
	}
	chain, _ := NewCustomTestBlockchainWithConfig(0, 0, key, cfg)

	stateDb := chain.appState.State

	tx := &types.Transaction{
		Type:         types.SubmitAnswersHashTx,
		AccountNonce: 1,
		Epoch:        0,
		Payload:      common.Hash{0x1}.Bytes(),
	}

	signed, err := types.SignTx(tx, key)
	require.NoError(t, err)
	err = chain.txpool.Add(signed)
	require.NoError(t, err)

	chain.GenerateBlocks(3)

	require.True(t, stateDb.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.False(t, stateDb.HasValidationTx(addr, types.SubmitShortAnswersTx))

	tx = &types.Transaction{
		Type:         types.EvidenceTx,
		AccountNonce: 2,
		Payload:      []byte{0x1},
	}
	signed, _ = types.SignTx(tx, key)
	err = chain.txpool.Add(signed)
	require.NoError(t, err)

	chain.GenerateBlocks(1)
	require.True(t, stateDb.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.True(t, stateDb.HasValidationTx(addr, types.EvidenceTx))

	tx = &types.Transaction{
		Type:         types.EvidenceTx,
		AccountNonce: 3,
		Payload:      []byte{0x1},
	}
	signed, _ = types.SignTx(tx, key)

	err = chain.txpool.Add(signed)
	require.True(t, err == validation.DuplicatedTx)
}

func Test_Blockchain_GodAddressInvitesLimit(t *testing.T) {
	require := require.New(t)
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)
	chain, state := NewCustomTestBlockchain(5, 0, key)

	count := int(common.GodAddressInvitesCount(0))
	for i := 0; i < count; i++ {
		keyReceiver, _ := crypto.GenerateKey()
		receiver := crypto.PubkeyToAddress(keyReceiver.PublicKey)
		tx, _ := chain.secStore.SignTx(BuildTx(state, addr, &receiver, types.InviteTx, decimal.Zero, decimal.New(2, 0), decimal.Zero, 0, 0, nil))
		require.NoError(chain.txpool.Add(tx))
		chain.GenerateBlocks(1)
	}

	keyReceiver, _ := crypto.GenerateKey()
	receiver := crypto.PubkeyToAddress(keyReceiver.PublicKey)
	tx, _ := chain.secStore.SignTx(BuildTx(state, addr, &receiver, types.InviteTx, decimal.Zero, decimal.New(2, 0), decimal.Zero, 0, 0, nil))
	require.Equal(validation.InsufficientInvites, chain.txpool.Add(tx), "we should not issue invite if we exceed limit")
}
