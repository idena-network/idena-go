package blockchain

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/blockchain/attachments"
	fee2 "github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/tests"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"testing"
	"time"
)

func Test_ApplyBlockRewards(t *testing.T) {
	chain, _, _, _ := NewTestBlockchain(false, nil)

	header := &types.ProposedHeader{
		Height:         2,
		ParentHash:     chain.Head.Hash(),
		Time:           time.Now().UTC().Unix(),
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

	context := &txExecutionContext{
		appState: chain.appState,
	}
	chain.applyTxOnState(signed, context)

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

	context := &txExecutionContext{
		appState: chain.appState,
	}
	chain.applyTxOnState(signed, context)
	require.Equal(t, state.Killed, appState.State.GetIdentityState(sender))
	require.Equal(t, 0, big.NewInt(0).Cmp(appState.State.GetBalance(sender)))

	require.Equal(t, state.Candidate, appState.State.GetIdentityState(receiver))
	require.Equal(t, -1, big.NewInt(0).Cmp(appState.State.GetBalance(receiver)))
}

func Test_ApplyKillTx(t *testing.T) {
	require := require.New(t)
	chain, appState, _, _ := NewTestBlockchain(true, nil)

	key, _ := crypto.GenerateKey()
	sender := crypto.PubkeyToAddress(key.PublicKey)

	balance := new(big.Int).Mul(big.NewInt(50), common.DnaBase)
	stake := new(big.Int).Mul(big.NewInt(10000), common.DnaBase)

	account := appState.State.GetOrNewAccountObject(sender)
	account.SetBalance(balance)

	id := appState.State.GetOrNewIdentityObject(sender)
	id.SetStake(stake)
	id.SetState(state.Invite)

	chain.appState.State.SetFeePerGas(new(big.Int).Div(big.NewInt(1e+18), big.NewInt(1000)))

	tx := &types.Transaction{
		Type:         types.KillTx,
		Amount:       big.NewInt(1),
		AccountNonce: 1,
	}

	signed, _ := types.SignTx(tx, key)

	fee := fee2.CalculateFee(chain.appState.ValidatorsCache.NetworkSize(), chain.appState.State.FeePerGas(), tx)
	require.Equal(big.NewInt(0), fee)
	require.Error(validation.ValidateTx(chain.appState, signed, fee2.MinFeePerGas, validation.InBlockTx), "should return error if amount is not zero")

	tx2 := &types.Transaction{
		Type:         types.KillTx,
		Amount:       big.NewInt(1),
		AccountNonce: 1,
		To:           &common.Address{0x1},
	}

	signed2, _ := types.SignTx(tx2, key)

	fee = fee2.CalculateFee(chain.appState.ValidatorsCache.NetworkSize(), chain.appState.State.FeePerGas(), tx2)
	require.Equal(big.NewInt(0), fee)
	require.Error(validation.ValidateTx(chain.appState, signed2, fee2.MinFeePerGas, validation.InBlockTx), "should return error if *to is filled")

	tx3 := &types.Transaction{
		Type:         types.KillTx,
		AccountNonce: 1,
	}

	signed3, _ := types.SignTx(tx3, key)

	fee = fee2.CalculateFee(chain.appState.ValidatorsCache.NetworkSize(), chain.appState.State.FeePerGas(), tx3)

	require.NoError(validation.ValidateTx(chain.appState, signed3, fee2.MinFeePerGas, validation.InBlockTx))
	context := &txExecutionContext{
		appState: chain.appState,
	}
	chain.applyTxOnState(signed3, context)

	require.Equal(big.NewInt(0), fee)
	require.Equal(state.Killed, appState.State.GetIdentityState(sender))
	require.Equal(new(big.Int).Add(balance, stake), appState.State.GetBalance(sender))
	require.True(common.ZeroOrNil(appState.State.GetStakeBalance(sender)))
}

func Test_ApplyDoubleKillTx(t *testing.T) {
	require := require.New(t)
	chain, appState, _, _ := NewTestBlockchain(true, nil)

	key, _ := crypto.GenerateKey()
	sender := crypto.PubkeyToAddress(key.PublicKey)

	stake := new(big.Int).Mul(big.NewInt(100), common.DnaBase)

	id := appState.State.GetOrNewIdentityObject(sender)
	id.SetStake(stake)
	id.SetState(state.Invite)

	tx1 := &types.Transaction{
		Type:         types.KillTx,
		AccountNonce: 1,
		MaxFee:       common.DnaBase,
	}
	tx2 := &types.Transaction{
		Type:         types.KillTx,
		AccountNonce: 2,
		MaxFee:       common.DnaBase,
	}

	signedTx1, _ := types.SignTx(tx1, key)
	signedTx2, _ := types.SignTx(tx2, key)

	chain.appState.State.SetFeePerGas(fee2.MinFeePerGas)
	require.Nil(validation.ValidateTx(chain.appState, signedTx1, fee2.MinFeePerGas, validation.InBlockTx))
	require.Nil(validation.ValidateTx(chain.appState, signedTx2, fee2.MinFeePerGas, validation.InBlockTx))

	context := &txExecutionContext{
		appState: chain.appState,
	}
	_, _, _, err := chain.applyTxOnState(signedTx1, context)

	require.Nil(err)
	require.Equal(validation.InvalidSender, validation.ValidateTx(chain.appState, signedTx2, fee2.MinFeePerGas, validation.InBlockTx))
}

func Test_ApplyKillInviteeTx(t *testing.T) {
	chain, appState, _, _ := NewTestBlockchain(true, nil)

	inviterKey, _ := crypto.GenerateKey()
	inviter := crypto.PubkeyToAddress(inviterKey.PublicKey)
	invitee := tests.GetRandAddr()
	anotherInvitee := tests.GetRandAddr()

	appState.State.SetInviter(invitee, inviter, common.Hash{}, 10)
	appState.State.SetInviter(anotherInvitee, inviter, common.Hash{}, 20)
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
	}
	signedTx, _ := types.SignTx(tx, inviterKey)
	require.Error(t, validation.ValidateTx(chain.appState, signedTx, fee2.MinFeePerGas, validation.InBlockTx), "should return error if *to is not filled")

	tx2 := &types.Transaction{
		Type:         types.KillInviteeTx,
		AccountNonce: 1,
		Amount:       new(big.Int).Mul(big.NewInt(9), common.DnaBase),
		To:           &invitee,
	}
	signedTx2, _ := types.SignTx(tx2, inviterKey)
	require.Error(t, validation.ValidateTx(chain.appState, signedTx2, fee2.MinFeePerGas, validation.InBlockTx), "should return error if amount is not zero")

	tx3 := &types.Transaction{
		Type:         types.KillInviteeTx,
		AccountNonce: 1,
		To:           &invitee,
		MaxFee:       new(big.Int).Mul(big.NewInt(2), common.DnaBase),
	}
	signedTx3, _ := types.SignTx(tx3, inviterKey)
	require.NoError(t, validation.ValidateTx(chain.appState, signedTx3, fee2.MinFeePerGas, validation.InBlockTx))

	chain.appState.State.SetFeePerGas(new(big.Int).Div(big.NewInt(1e+18), big.NewInt(1000)))
	fee := fee2.CalculateFee(chain.appState.ValidatorsCache.NetworkSize(), chain.appState.State.FeePerGas(), tx3)

	context := &txExecutionContext{
		appState: chain.appState,
	}
	chain.applyTxOnState(signedTx3, context)

	require.Equal(t, uint8(0), appState.State.GetInvites(inviter))
	require.Equal(t, 1, len(appState.State.GetInvitees(inviter)))
	require.Equal(t, anotherInvitee, appState.State.GetInvitees(inviter)[0].Address)
	newBalance := new(big.Int).Mul(big.NewInt(60), common.DnaBase)
	newBalance.Sub(newBalance, fee)
	require.Equal(t, newBalance, appState.State.GetBalance(inviter))
	require.True(t, common.ZeroOrNil(appState.State.GetStakeBalance(invitee)))

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
	chain, _, _, _ := NewTestBlockchain(true, nil)

	appState, _ := chain.appState.ForCheck(1)

	block := generateBlock(4, 4000)
	var usedGas uint64
	for _, tx := range block.Body.Transactions {
		usedGas += uint64(fee2.CalculateGas(tx))
	}
	chain.applyNextBlockFee(appState, block, usedGas)
	require.Equal(t, big.NewInt(10996093750000000), appState.State.FeePerGas())

	block = generateBlock(5, 1500)
	usedGas = 0
	for _, tx := range block.Body.Transactions {
		usedGas += uint64(fee2.CalculateGas(tx))
	}
	chain.applyNextBlockFee(appState, block, usedGas)
	require.Equal(t, big.NewInt(10547766685485839), appState.State.FeePerGas())

	block = generateBlock(6, 0)
	chain.applyNextBlockFee(appState, block, 0)
	// 0.01 / networkSize, where networkSize is 0, feePerGas = 0.01 DNA
	require.Equal(t, new(big.Int).Div(common.DnaBase, big.NewInt(100)), appState.State.FeePerGas())
}

func Test_applyVrfProposerThreshold(t *testing.T) {
	chain, _ := NewTestBlockchainWithBlocks(100, 0)

	chain.GenerateEmptyBlocks(10)
	require.Equal(t, 10, chain.appState.State.EmptyBlocksCount())

	chain.GenerateBlocks(5, 0)
	require.Equal(t, 10, chain.appState.State.EmptyBlocksCount())

	chain.GenerateBlocks(100, 0)
	require.Equal(t, 0.5, chain.appState.State.VrfProposerThreshold())
}

type txWithTimestamp struct {
	tx        *types.Transaction
	timestamp int64
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
	appState.State.SetFeePerGas(new(big.Int).Div(common.DnaBase, big.NewInt(1000)))
	fee := fee2.CalculateFee(appState.ValidatorsCache.NetworkSize(), appState.State.FeePerGas(), tx)
	expectedBalance := new(big.Int).Mul(big.NewInt(89), common.DnaBase)
	expectedBalance.Sub(expectedBalance, fee)

	context := &txExecutionContext{
		appState: chain.appState,
	}
	chain.applyTxOnState(signedTx, context)

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
	appState.State.SetFeePerGas(big.NewInt(1))

	fee := fee2.CalculateFee(appState.ValidatorsCache.NetworkSize(), appState.State.FeePerGas(), tx)
	expectedBalance := big.NewInt(999_990)
	expectedBalance.Sub(expectedBalance, fee)

	context := &txExecutionContext{
		appState: chain.appState,
	}
	chain.applyTxOnState(signedTx, context)

	require.Equal(t, 1, fee.Sign())
	require.Equal(t, expectedBalance, appState.State.GetBalance(sender))
	identity := appState.State.GetIdentity(sender)
	require.Equal(t, 2, len(identity.Flips))
}

func Test_Blockchain_OnlineStatusSwitch(t *testing.T) {
	require := require.New(t)
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)
	consensusCfg := config.ConsensusVersions[config.ConsensusV5]
	consensusCfg.Automine = true
	consensusCfg.StatusSwitchRange = 10
	cfg := &config.Config{
		Network:   0x99,
		Consensus: consensusCfg,
		GenesisConf: &config.GenesisConf{
			Alloc: map[common.Address]config.GenesisAllocation{
				addr: {
					State:   uint8(state.Verified),
					Balance: new(big.Int).Mul(big.NewInt(1e+18), big.NewInt(100)),
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
	tx, _ := chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(20, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true)))
	err := chain.txpool.AddInternalTx(tx)
	require.NoError(err)

	// apply pending status switch
	chain.GenerateBlocks(1, 0)
	require.Equal(1, len(state.State.StatusSwitchAddresses()))
	require.False(state.IdentityState.IsOnline(addr))

	// fail to switch online again
	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(20, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true)))
	err = chain.txpool.AddInternalTx(tx)
	require.Error(err, "should not validate tx if switch is already pending")

	// switch status to online
	chain.GenerateBlocks(3, 0)
	require.Equal(uint64(10), chain.Head.Height())
	require.Zero(len(state.State.StatusSwitchAddresses()))
	require.True(state.IdentityState.IsOnline(addr))
	require.True(chain.Head.Flags().HasFlag(types.IdentityUpdate))

	// fail to switch online again
	chain.GenerateBlocks(5, 0)
	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(20, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true)))
	err = chain.txpool.AddInternalTx(tx)
	require.Error(err, "should not validate tx if identity already has online status")

	// add pending request to switch offline
	chain.GenerateBlocks(4, 0)
	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(20, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(false)))
	err = chain.txpool.AddInternalTx(tx)
	require.NoError(err)

	// switch status to offline
	chain.GenerateBlocks(1, 0)
	require.Equal(uint64(20), chain.Head.Height())
	require.Zero(len(state.State.StatusSwitchAddresses()))
	require.False(state.IdentityState.IsOnline(addr))
	require.True(chain.Head.Flags().HasFlag(types.IdentityUpdate))

	// add pending request to switch offline
	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(20, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true)))
	err = chain.txpool.AddInternalTx(tx)
	require.NoError(err)
	chain.GenerateBlocks(1, 0)

	require.Equal(1, len(state.State.StatusSwitchAddresses()))

	// remove pending request to switch online
	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(20, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(false)))
	err = chain.txpool.AddInternalTx(tx)
	require.NoError(err)
	chain.GenerateBlocks(1, 0)

	require.Zero(len(state.State.StatusSwitchAddresses()))

	// 30th block should not update identity statuses, no pending requests
	chain.GenerateBlocks(8, 0)
	require.Equal(uint64(30), chain.Head.Height())
	require.False(state.IdentityState.IsOnline(addr))
	require.False(chain.Head.Flags().HasFlag(types.IdentityUpdate))

	chain.GenerateBlocks(70, 0)
	require.Equal(uint64(100), chain.Head.Height())

	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(20, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true)))
	err = chain.txpool.AddInternalTx(tx)
	require.Nil(err)

	// switch status to online
	chain.GenerateBlocks(10, 0)

	require.True(state.IdentityState.IsOnline(addr))
	state.State.AddDelayedPenalty(common.Address{0x2})
	state.State.AddDelayedPenalty(addr)
	state.State.AddDelayedPenalty(common.Address{0x3})
	state.Commit(nil)
	chain.CommitState()

	tx, _ = chain.secStore.SignTx(BuildTx(state, addr, nil, types.OnlineStatusTx, decimal.Zero, decimal.New(20, 0), decimal.Zero, 0, 0, attachments.CreateOnlineStatusAttachment(true)))
	err = chain.txpool.AddInternalTx(tx)
	require.Nil(err)
	chain.GenerateBlocks(1, 0)

	require.Equal([]common.Address{{0x2}, {0x3}}, state.State.DelayedOfflinePenalties())
	require.True(state.IdentityState.IsOnline(addr))
	require.Zero(len(state.State.StatusSwitchAddresses()))

	state.State.AddDelayedPenalty(addr)
	state.Commit(nil)
	chain.CommitState()
	chain.GenerateBlocks(10, 0)

	require.False(state.IdentityState.IsOnline(addr))
	require.True(state.State.GetPenalty(addr).Sign() > 0)
	require.Len(state.State.DelayedOfflinePenalties(), 0)
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
	err = chain.txpool.AddInternalTx(signed)
	require.NoError(t, err)

	chain.GenerateBlocks(3, 0)

	require.True(t, app.State.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.False(t, app.State.HasValidationTx(addr, types.SubmitShortAnswersTx))

	tx = &types.Transaction{
		Type:         types.EvidenceTx,
		AccountNonce: 2,
		Payload:      []byte{0x1},
	}
	signed, _ = types.SignTx(tx, key)
	err = chain.txpool.AddInternalTx(signed)
	require.NoError(t, err)

	chain.GenerateBlocks(1, 0)
	require.True(t, app.State.HasValidationTx(addr, types.SubmitAnswersHashTx))
	require.True(t, app.State.HasValidationTx(addr, types.EvidenceTx))

	tx = &types.Transaction{
		Type:         types.EvidenceTx,
		AccountNonce: 3,
		Payload:      []byte{0x1},
	}
	signed, _ = types.SignTx(tx, key)

	err = chain.txpool.AddInternalTx(signed)
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
		require.NoError(chain.txpool.AddInternalTx(tx))
		chain.GenerateBlocks(1, 0)
	}

	keyReceiver, _ := crypto.GenerateKey()
	receiver := crypto.PubkeyToAddress(keyReceiver.PublicKey)
	tx, _ := chain.secStore.SignTx(BuildTx(state, addr, &receiver, types.InviteTx, decimal.Zero, decimal.New(2, 0), decimal.Zero, 0, 0, nil))
	require.Equal(validation.InsufficientInvites, chain.txpool.AddInternalTx(tx), "we should not issue invite if we exceed limit")
}

func Test_setNewIdentitiesAttributes(t *testing.T) {
	require := require.New(t)
	key, _ := crypto.GenerateKey()
	_, s := NewCustomTestBlockchain(5, 0, key)

	identities := []state.Identity{
		// 98%
		{Code: []byte{0x1}, State: state.Human, Scores: []byte{common.EncodeScore(6, 6), common.EncodeScore(6, 6), common.EncodeScore(6, 6), common.EncodeScore(5.5, 6)}},
		// 83%
		{Code: []byte{0x2}, State: state.Verified, Scores: []byte{common.EncodeScore(5, 6)}},
		// 85%
		{Code: []byte{0x3}, State: state.Verified, Scores: []byte{common.EncodeScore(5, 6), common.EncodeScore(5.5, 6), common.EncodeScore(4, 5)}},
		// 89%
		{Code: []byte{0x4}, State: state.Verified, Scores: []byte{common.EncodeScore(5, 6), common.EncodeScore(5, 6), common.EncodeScore(6, 6)}},
		// 92%
		{Code: []byte{0x5}, State: state.Human, Scores: []byte{common.EncodeScore(5, 5), common.EncodeScore(5.5, 6), common.EncodeScore(5.5, 6), common.EncodeScore(5.5, 6), common.EncodeScore(4.5, 5)}},
		// 81%
		{Code: []byte{0x6}, State: state.Verified, Scores: []byte{common.EncodeScore(5, 6), common.EncodeScore(4.5, 6), common.EncodeScore(4.5, 5), common.EncodeScore(4.5, 6), common.EncodeScore(5, 6)}},
		// 94%
		{Code: []byte{0x7}, State: state.Human, Scores: []byte{common.EncodeScore(5, 6), common.EncodeScore(6, 6), common.EncodeScore(6, 6)}},
		// 94%
		{Code: []byte{0x8}, State: state.Human, Scores: []byte{common.EncodeScore(5, 6), common.EncodeScore(6, 6), common.EncodeScore(6, 6)}},
		// 83%
		{Code: []byte{0x9}, State: state.Verified, Scores: []byte{common.EncodeScore(5, 6)}},
		// 82%
		{Code: []byte{0xa}, State: state.Verified, Scores: []byte{common.EncodeScore(5, 6), common.EncodeScore(4.5, 6), common.EncodeScore(4.5, 5)}},
		// 81%
		{Code: []byte{0xb}, State: state.Verified, Scores: []byte{common.EncodeScore(5, 6), common.EncodeScore(4.5, 6), common.EncodeScore(4.5, 5), common.EncodeScore(4.5, 6), common.EncodeScore(5, 6)}},
		// 81%
		{Code: []byte{0xc}, State: state.Verified, Scores: []byte{common.EncodeScore(5, 6), common.EncodeScore(4.5, 6), common.EncodeScore(4.5, 5), common.EncodeScore(4.5, 6), common.EncodeScore(5, 6)}},
		// 94%
		{Code: []byte{0xd}, State: state.Verified, Scores: []byte{common.EncodeScore(5, 6), common.EncodeScore(6, 6), common.EncodeScore(6, 6)}},
	}

	for _, item := range identities {
		var addr common.Address
		copy(addr[:], item.Code)
		s.State.SetState(addr, item.State)
		for _, score := range item.Scores {
			s.State.AddNewScore(addr, score)
		}
	}
	s.Commit(nil)

	setNewIdentitiesAttributes(s, 12, 100, false, map[common.ShardId]*types.ValidationResults{}, nil)

	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x5}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x7}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x8}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xd}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x2}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x3}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x4}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x9}))

	s.Reset()
	setNewIdentitiesAttributes(s, 1, 100, false, map[common.ShardId]*types.ValidationResults{}, nil)
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(0), s.State.GetInvites(common.Address{0x7}))
	require.Equal(uint8(0), s.State.GetInvites(common.Address{0x8}))

	s.Reset()
	setNewIdentitiesAttributes(s, 5, 100, false, map[common.ShardId]*types.ValidationResults{}, nil)
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x5}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x7}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x8}))
	require.Equal(uint8(0), s.State.GetInvites(common.Address{0xd}))
	require.Equal(uint8(0), s.State.GetInvites(common.Address{0x4}))

	s.Reset()
	setNewIdentitiesAttributes(s, 15, 100, false, map[common.ShardId]*types.ValidationResults{}, nil)
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x5}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x4}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x6}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xa}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xb}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xc}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xd}))

	s.Reset()
	setNewIdentitiesAttributes(s, 20, 100, false, map[common.ShardId]*types.ValidationResults{}, nil)
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x5}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x4}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x6}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xa}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xb}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xc}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xd}))

	s.Reset()
	setNewIdentitiesAttributes(s, 2, 100, false, map[common.ShardId]*types.ValidationResults{}, nil)
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x7}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x8}))
	require.Equal(uint8(0), s.State.GetInvites(common.Address{0xd}))
	require.Equal(uint8(0), s.State.GetInvites(common.Address{0x5}))

	s.Reset()
	setNewIdentitiesAttributes(s, 6, 100, false, map[common.ShardId]*types.ValidationResults{}, nil)
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x1}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x7}))
	require.Equal(uint8(2), s.State.GetInvites(common.Address{0x8}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0x5}))
	require.Equal(uint8(1), s.State.GetInvites(common.Address{0xd}))
	require.Equal(uint8(0), s.State.GetInvites(common.Address{0x4}))
}

func Test_ClearDustAccounts(t *testing.T) {
	require := require.New(t)
	key, _ := crypto.GenerateKey()
	_, s := NewCustomTestBlockchain(1, 0, key)

	s.State.AddBalance(common.Address{0x1}, big.NewInt(1))
	s.State.AddBalance(common.Address{0x2}, common.DnaBase)
	s.State.AddBalance(common.Address{0x3}, new(big.Int).Div(common.DnaBase, big.NewInt(100)))
	s.State.AddBalance(common.Address{0x4}, new(big.Int).Mul(common.DnaBase, big.NewInt(100)))
	s.State.AddBalance(common.Address{0x5}, new(big.Int).Mul(common.DnaBase, big.NewInt(5000)))
	s.State.AddBalance(common.Address{0x6}, big.NewInt(999_999_999_999))

	s.State.Commit(true)

	// accounts with balance less than 0.1 DNA should be removed (1, 3, 6)
	clearDustAccounts(s, 100, nil)

	s.State.Commit(true)

	require.False(s.State.AccountExists(common.Address{0x1}))
	require.True(s.State.AccountExists(common.Address{0x2}))
	require.False(s.State.AccountExists(common.Address{0x3}))
	require.True(s.State.AccountExists(common.Address{0x4}))
	require.True(s.State.AccountExists(common.Address{0x5}))
	require.False(s.State.AccountExists(common.Address{0x6}))

	s.State.Clear()

	s.State.SetBalance(common.Address{0x4}, big.NewInt(1))
	s.State.SetBalance(common.Address{0x7}, big.NewInt(100))
	s.State.SetBalance(common.Address{0x8}, new(big.Int).Mul(common.DnaBase, big.NewInt(100)))

	// accounts with balance less than 2 DNA should be removed (2, 4)
	clearDustAccounts(s, 5, nil)

	s.State.Commit(true)

	require.False(s.State.AccountExists(common.Address{0x2}))
	require.False(s.State.AccountExists(common.Address{0x4}))
	require.True(s.State.AccountExists(common.Address{0x5}))
	require.False(s.State.AccountExists(common.Address{0x7}))
	require.True(s.State.AccountExists(common.Address{0x8}))
}

func TestBlockchain_applyOfflinePenalty(t *testing.T) {
	chain := &Blockchain{
		config: &config.Config{
			Consensus: config.GetDefaultConsensusConfig(),
		},
	}

	chain.config.Consensus.FinalCommitteeReward = big.NewInt(1)
	chain.config.Consensus.BlockReward = big.NewInt(3)

	db := dbm.NewMemDB()
	bus := eventbus.New()
	appState, _ := appstate.NewAppState(db, bus)

	count := byte(10)
	pool1 := common.Address{0x11}

	for i := byte(1); i <= count; i++ {
		addr := common.Address{i}
		appState.IdentityState.Add(addr)
		appState.IdentityState.SetOnline(addr, true)
		if i%3 == 0 { // 3, 6, 9
			appState.IdentityState.SetDelegatee(addr, pool1)
		}
	}
	appState.Commit(nil)
	appState.Initialize(1)

	require.True(t, appState.ValidatorsCache.IsPool(pool1))

	chain.applyOfflinePenalty(appState, pool1)

	require.Equal(t, big.NewInt(4*3*1800/int64(count)).Bytes(), appState.State.GetPenalty(pool1).Bytes())
}

func Test_Delegation(t *testing.T) {
	chain, appState, txpool, coinbaseKey := NewTestBlockchainWithConfig(true, config.ConsensusVersions[config.ConsensusV4], &config.ValidationConfig{}, nil, -1, -1, 0, 0)

	coinbase := crypto.PubkeyToAddress(coinbaseKey.PublicKey)

	appState.State.SetState(coinbase, state.Verified)
	appState.State.SetNextValidationTime(time.Date(2099, 01, 01, 00, 00, 00, 0, time.UTC))
	appState.IdentityState.Add(coinbase)
	appState.IdentityState.SetOnline(coinbase, true)

	keys := []*ecdsa.PrivateKey{}
	addrs := []common.Address{}
	for i := 0; i < 5; i++ {
		key, _ := crypto.GenerateKey()
		keys = append(keys, key)

		addr := crypto.PubkeyToAddress(key.PublicKey)

		addrs = append(addrs, addr)
		appState.State.SetState(addr, state.Newbie)
		appState.IdentityState.Add(addr)
		appState.IdentityState.SetOnline(addr, true)
		appState.State.SetBalance(addr, big.NewInt(0).Mul(big.NewInt(1000), common.DnaBase))
	}

	appState.IdentityState.SetOnline(addrs[3], false)

	validation.SetAppConfig(chain.config)

	pool1 := common.Address{0x1}
	pool2 := common.Address{0x2}

	pool3Key, _ := crypto.GenerateKey()
	pool3 := crypto.PubkeyToAddress(pool3Key.PublicKey)

	appState.State.SetBalance(pool3, big.NewInt(0).Mul(big.NewInt(1000), common.DnaBase))

	appState.State.SetState(pool3, state.Newbie)
	appState.IdentityState.Add(pool3)
	appState.IdentityState.SetOnline(pool3, true)

	appState.Commit(nil)
	appState.ValidatorsCache.Load()
	chain.CommitState()

	addTx := func(key *ecdsa.PrivateKey, txType types.TxType, nonce uint32, epoch uint16, to *common.Address, payload []byte) error {
		tx := &types.Transaction{
			Type:         txType,
			AccountNonce: nonce,
			To:           to,
			Epoch:        epoch,
			MaxFee:       new(big.Int).Mul(big.NewInt(50), common.DnaBase),
			Payload:      payload,
		}
		signedTx, _ := types.SignTx(tx, key)
		return txpool.AddInternalTx(signedTx)
	}

	require.NoError(t, addTx(keys[0], types.DelegateTx, 1, 0, &pool1, nil))
	require.NoError(t, addTx(keys[1], types.DelegateTx, 1, 0, &pool2, nil))
	require.NoError(t, addTx(keys[2], types.DelegateTx, 1, 0, &pool2, nil))
	require.NoError(t, addTx(keys[3], types.DelegateTx, 1, 0, &pool3, nil))

	chain.GenerateBlocks(1, 0)
	require.ErrorIs(t, validation.InvalidRecipient, addTx(pool3Key, types.DelegateTx, 1, 0, &pool3, nil))
	require.NoError(t, addTx(pool3Key, types.DelegateTx, 1, 0, &pool2, nil))

	chain.GenerateBlocks(1, 0)

	require.Len(t, appState.State.Delegations(), 5)

	require.NoError(t, addTx(keys[0], types.OnlineStatusTx, 2, 0, nil, attachments.CreateOnlineStatusAttachment(false)))
	require.NoError(t, addTx(keys[3], types.OnlineStatusTx, 2, 0, nil, attachments.CreateOnlineStatusAttachment(true)))

	chain.GenerateBlocks(1, 0)
	chain.GenerateEmptyBlocks(50)

	require.False(t, appState.ValidatorsCache.IsOnlineIdentity(addrs[0]))
	require.False(t, appState.ValidatorsCache.IsOnlineIdentity(addrs[1]))
	require.False(t, appState.ValidatorsCache.IsOnlineIdentity(addrs[2]))
	require.False(t, appState.ValidatorsCache.IsOnlineIdentity(addrs[3]))

	require.Len(t, appState.State.Delegations(), 0)
	require.Nil(t, appState.State.Delegatee(pool3))
	require.Equal(t, 1, appState.ValidatorsCache.PoolSize(pool1))
	require.Equal(t, 2, appState.ValidatorsCache.PoolSize(pool2))
	require.Equal(t, 2, appState.ValidatorsCache.PoolSize(pool3))

	attachments.CreateOnlineStatusAttachment(false)

	addr3 := crypto.PubkeyToAddress(keys[3].PublicKey)
	require.NoError(t, addTx(pool3Key, types.KillDelegatorTx, 2, 0, &addr3, nil))
	chain.GenerateBlocks(1, 0)

	require.Equal(t, 0, appState.ValidatorsCache.PoolSize(pool3))
	require.False(t, appState.ValidatorsCache.IsPool(pool3))
	require.True(t, appState.ValidatorsCache.IsOnlineIdentity(pool3))

	require.ErrorIs(t, validation.WrongEpoch, addTx(keys[1], types.UndelegateTx, 2, 0, nil, nil))

	appState.State.SetGlobalEpoch(1)
	appState.Commit(nil)
	chain.CommitState()

	require.NoError(t, addTx(keys[1], types.UndelegateTx, 1, 1, nil, nil))
	require.NoError(t, addTx(keys[2], types.UndelegateTx, 1, 1, nil, nil))

	chain.GenerateBlocks(1, 0)
	require.Len(t, appState.State.Delegations(), 2)

	chain.GenerateBlocks(50, 0)
	require.Len(t, appState.State.Delegations(), 0)

	require.Equal(t, 0, appState.ValidatorsCache.PoolSize(pool2))
	require.False(t, appState.ValidatorsCache.IsPool(pool2))
	require.False(t, appState.ValidatorsCache.IsOnlineIdentity(pool2))
}

func TestBalance_shards_reducing(t *testing.T) {
	db := dbm.NewMemDB()
	bus := eventbus.New()
	appState, _ := appstate.NewAppState(db, bus)

	var totalNewbies, totalVerified, totalSuspended int

	statuses := []state.IdentityState{state.Suspended, state.Zombie, state.Newbie, state.Verified, state.Human}

	shardsNum := 2

	newbiesByShard := map[common.ShardId]int{}
	verifiedByShard := map[common.ShardId]int{}
	suspendedByShard := map[common.ShardId]int{}

	for shardId := common.ShardId(1); shardId <= common.ShardId(shardsNum); shardId++ {
		statusIdx := 0
		for i := 0; i < common.MinShardSize-100; i++ {
			key, _ := crypto.GenerateKey()
			addr := crypto.PubkeyToAddress(key.PublicKey)
			status := statuses[statusIdx]
			appState.State.SetState(addr, status)
			appState.State.SetShardId(addr, shardId)
			switch status {
			case state.Suspended, state.Zombie:
				totalSuspended++
				suspendedByShard[shardId]++
			case state.Newbie:
				totalNewbies++
				newbiesByShard[shardId]++
			case state.Verified, state.Human:
				totalVerified++
				verifiedByShard[shardId]++
			}
			statusIdx++
			if statusIdx >= len(statuses) {
				statusIdx = 0
			}
		}
	}
	appState.State.SetShardsNum(uint32(shardsNum))
	appState.Commit(nil)
	balanceShards(appState, totalNewbies, totalVerified, totalSuspended, newbiesByShard, verifiedByShard, suspendedByShard)

	require.Equal(t, uint32(1), appState.State.ShardsNum())

	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		require.Equal(t, common.ShardId(1), identity.ShardId)
	})
}

func TestBalance_shards_increasing(t *testing.T) {
	db := dbm.NewMemDB()
	bus := eventbus.New()
	appState, _ := appstate.NewAppState(db, bus)

	var totalNewbies, totalVerified, totalSuspended int

	statuses := []state.IdentityState{state.Suspended, state.Zombie, state.Newbie, state.Verified, state.Human}

	shardsNum := 2

	newbiesByShard := map[common.ShardId]int{}
	verifiedByShard := map[common.ShardId]int{}
	suspendedByShard := map[common.ShardId]int{}

	for shardId := common.ShardId(1); shardId <= common.ShardId(shardsNum); shardId++ {
		statusIdx := 0
		for i := 0; i < common.MaxShardSize+100; i++ {
			key, _ := crypto.GenerateKey()
			addr := crypto.PubkeyToAddress(key.PublicKey)
			status := statuses[statusIdx]
			appState.State.SetState(addr, status)
			appState.State.SetShardId(addr, shardId)
			switch status {
			case state.Suspended, state.Zombie:
				totalSuspended++
				suspendedByShard[shardId]++
			case state.Newbie:
				totalNewbies++
				newbiesByShard[shardId]++
			case state.Verified, state.Human:
				totalVerified++
				verifiedByShard[shardId]++
			}
			statusIdx++
			if statusIdx >= len(statuses) {
				statusIdx = 0
			}
		}
	}
	appState.State.SetShardsNum(uint32(shardsNum))
	appState.Commit(nil)
	balanceShards(appState, totalNewbies, totalVerified, totalSuspended, newbiesByShard, verifiedByShard, suspendedByShard)

	require.Equal(t, uint32(4), appState.State.ShardsNum())

	shardSizes := map[common.ShardId]int{}

	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		shardSizes[identity.ShiftedShardId()]++
	})
	for _, s := range shardSizes {
		require.Less(t, s, common.MaxShardSize)
		require.Greater(t, s, common.MinShardSize)
	}
}
