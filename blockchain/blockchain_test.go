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

	appState, _ := chain.appState.ForCheck(1)
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

	expectedBalance, stake := splitReward(totalReward, false, chain.config.Consensus)

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

func Test_ApplyDoubleKillTx(t *testing.T) {
	require := require.New(t)
	chain, appState, _, _ := NewTestBlockchain(true, nil)

	key, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()
	sender := crypto.PubkeyToAddress(key.PublicKey)
	receiver := crypto.PubkeyToAddress(key2.PublicKey)

	stake := new(big.Int).Mul(big.NewInt(100), common.DnaBase)

	id := appState.State.GetOrNewIdentityObject(sender)
	id.SetStake(stake)
	id.SetState(state.Invite)

	tx1 := &types.Transaction{
		Type:         types.KillTx,
		AccountNonce: 1,
		To:           &receiver,
		MaxFee:       common.DnaBase,
	}
	tx2 := &types.Transaction{
		Type:         types.KillTx,
		AccountNonce: 2,
		To:           &receiver,
		MaxFee:       common.DnaBase,
	}

	signedTx1, _ := types.SignTx(tx1, key)
	signedTx2, _ := types.SignTx(tx2, key)

	chain.appState.State.SetFeePerByte(chain.config.Consensus.MinFeePerByte)
	require.Nil(validation.ValidateTx(chain.appState, signedTx1, chain.config.Consensus.MinFeePerByte, validation.InBlockTx))
	require.Nil(validation.ValidateTx(chain.appState, signedTx2, chain.config.Consensus.MinFeePerByte, validation.InBlockTx))

	_, err := chain.ApplyTxOnState(chain.appState, signedTx1, nil)

	require.Nil(err)
	require.Equal(validation.InsufficientFunds, validation.ValidateTx(chain.appState, signedTx2, chain.config.Consensus.MinFeePerByte, validation.InBlockTx))
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
	appState.State.SetState(invitee, state.Newbie)

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

	require.Equal(t, uint8(0), appState.State.GetInvites(inviter))
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
	conf := config.GetDefaultConsensusConfig()
	conf.MinFeePerByte = big.NewInt(0).Div(common.DnaBase, big.NewInt(100))
	chain, _, _, _ := NewTestBlockchainWithConfig(true, conf, &config.ValidationConfig{}, nil, -1, -1, 0, 0)

	appState, _ := chain.appState.ForCheck(1)

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
	conf := config.GetDefaultConsensusConfig()
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
	consensusCfg := config.GetDefaultConsensusConfig()
	consensusCfg.Automine = true
	consensusCfg.StatusSwitchRange = 10
	cfg := &config.Config{
		Network:   0x99,
		Consensus: consensusCfg,
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
	consensusCfg := config.GetDefaultConsensusConfig()
	consensusCfg.Automine = true
	consensusCfg.StatusSwitchRange = 10
	cfg := &config.Config{
		Network:   0x99,
		Consensus: consensusCfg,
		GenesisConf: &config.GenesisConf{
			Alloc: map[common.Address]config.GenesisAllocation{
				addr: {
					State: uint8(state.Verified),
				},
			},
			GodAddress:        addr,
			FirstCeremonyTime: 1999999999,
		},
		Validation: &config.ValidationConfig{
			ShortSessionDuration: 1 * time.Second,
			LongSessionDuration:  1 * time.Second,
		},
		Blockchain: &config.BlockchainConfig{},
	}
	chain, app := NewCustomTestBlockchainWithConfig(0, 0, key, cfg)

	app.State.SetValidationPeriod(state.LongSessionPeriod)
	app.Commit(nil)

	block := chain.GenerateEmptyBlock()
	chain.Head = block.Header
	chain.txpool.ResetTo(block)

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

	require.True(t, app.State.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.False(t, app.State.HasValidationTx(addr, types.SubmitShortAnswersTx))

	tx = &types.Transaction{
		Type:         types.EvidenceTx,
		AccountNonce: 2,
		Payload:      []byte{0x1},
	}
	signed, _ = types.SignTx(tx, key)
	err = chain.txpool.Add(signed)
	require.NoError(t, err)

	chain.GenerateBlocks(1)
	require.True(t, app.State.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.True(t, app.State.HasValidationTx(addr, types.EvidenceTx))

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

func Test_setNewIdentitiesAttributes(t *testing.T) {
	require := require.New(t)
	key, _ := crypto.GenerateKey()
	_, s := NewCustomTestBlockchain(5, 0, key)

	identities := []state.Identity{
		{State: state.Human, ShortFlipPoints: 98, QualifiedFlips: 100},
		{State: state.Verified, ShortFlipPoints: 83, QualifiedFlips: 100},
		{State: state.Verified, ShortFlipPoints: 85, QualifiedFlips: 100},
		{State: state.Verified, ShortFlipPoints: 88, QualifiedFlips: 100},
		{State: state.Human, ShortFlipPoints: 92, QualifiedFlips: 100},
		{State: state.Human, ShortFlipPoints: 94, QualifiedFlips: 100},
		{State: state.Human, ShortFlipPoints: 94, QualifiedFlips: 100},
		{State: state.Verified, ShortFlipPoints: 83, QualifiedFlips: 100},
		{State: state.Verified, ShortFlipPoints: 82, QualifiedFlips: 100},
		{State: state.Verified, ShortFlipPoints: 81, QualifiedFlips: 100},
	}

	for i, item := range identities {
		addr := common.Address{byte(i + 1)}
		s.State.SetState(addr, item.State)
		s.State.AddShortFlipPoints(addr, float32(item.ShortFlipPoints))
		s.State.AddQualifiedFlipsCount(addr, item.QualifiedFlips)
	}
	s.Commit(nil)

	setNewIdentitiesAttributes(s, 12, 100, false, &types.ValidationAuthors{}, nil)

	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x5}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x2}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x8}))

	s.Reset()
	setNewIdentitiesAttributes(s, 1, 100, false, &types.ValidationAuthors{}, nil)
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(0), s.State.GetInvites(common.Address{0x2}))

	s.Reset()
	setNewIdentitiesAttributes(s, 5, 100, false, &types.ValidationAuthors{}, nil)
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x6}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x7}))

	s.Reset()
	setNewIdentitiesAttributes(s, 14, 100, false, &types.ValidationAuthors{}, nil)
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xa}))

	s.Reset()
	setNewIdentitiesAttributes(s, 20, 100, false, &types.ValidationAuthors{}, nil)
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x6}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x4}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x9}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xa}))
}
