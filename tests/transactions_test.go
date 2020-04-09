package tests

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

func TestTransactions_EpochChanging(t *testing.T) {
	//TODO: remove skip
	t.SkipNow()

	require := require.New(t)
	key1, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()

	addr1 := crypto.PubkeyToAddress(key1.PublicKey)
	addr2 := crypto.PubkeyToAddress(key2.PublicKey)
	balance := getAmount(100000)

	alloc := make(map[common.Address]config.GenesisAllocation)
	alloc[addr1] = config.GenesisAllocation{
		Balance: balance,
		State:   uint8(state.Candidate),
	}
	alloc[addr2] = config.GenesisAllocation{
		Balance: balance,
		State:   uint8(state.Invite),
	}

	conf := config.GetDefaultConsensusConfig()
	conf.FinalCommitteeReward = big.NewInt(0)
	valConf := &config.ValidationConfig{}
	valConf.ValidationInterval = time.Minute * 1

	chain, appState, pool, _ := blockchain.NewTestBlockchainWithConfig(true, conf, valConf, alloc, -1, -1, 0, 0)

	tx1 := generateTx(getAmount(12), addr2, 1, 0, key1)
	tx2 := generateTx(getAmount(88), addr1, 1, 0, key2)
	tx3 := generateTx(getAmount(32), addr2, 2, 0, key1)

	feePerByte := big.NewInt(0).Div(big.NewInt(1e+18), big.NewInt(1000))
	feeTx1 := fee.CalculateCost(1, feePerByte, tx1)
	feeTx2 := fee.CalculateCost(1, feePerByte, tx2)
	feeTx3 := fee.CalculateCost(1, feePerByte, tx3)

	spend1 := new(big.Int).Add(feeTx1, feeTx3)
	receive1 := new(big.Int).Add(balance, getAmount(88))

	spend2 := feeTx2
	receive2 := new(big.Int).Add(balance, getAmount(44))

	require.NoError(pool.Add(tx1))
	require.NoError(pool.Add(tx2))
	require.NoError(pool.Add(tx3))

	block := chain.ProposeBlock()
	require.NoError(chain.AddBlock(block.Block, nil, nil))
	require.Equal(appState.State.GetBalance(addr1), new(big.Int).Sub(receive1, spend1))
	require.Equal(appState.State.GetBalance(addr2), new(big.Int).Sub(receive2, spend2))

	//new epoch
	block = chain.ProposeBlock()
	require.NoError(chain.AddBlock(block.Block, nil, nil))

	// new epoch started
	tx1 = generateTx(getAmount(15), addr2, 1, 1, key1)
	tx2 = generateTx(getAmount(10), addr1, 2, 1, key2) // wont be mined, nonce from future

	spend1 = fee.CalculateCost(2, feePerByte, tx1)
	receive1 = appState.State.GetBalance(addr1)

	receive2 = new(big.Int).Add(appState.State.GetBalance(addr2), getAmount(15))

	require.NoError(pool.Add(tx1))
	require.NoError(pool.Add(tx2))

	block = chain.ProposeBlock()
	require.NoError(chain.AddBlock(block.Block, nil, nil))

	require.Equal(1, len(block.Body.Transactions))

	require.Equal(uint16(1), appState.State.Epoch())

	require.Equal(new(big.Int).Sub(receive1, spend1), appState.State.GetBalance(addr1))
	require.Equal(receive2, appState.State.GetBalance(addr2))

	require.True(appState.IdentityState.IsApproved(addr1))
	require.False(appState.IdentityState.IsApproved(addr2))
}

func generateTx(amount *big.Int, to common.Address, nonce uint32, epoch uint16, key *ecdsa.PrivateKey) *types.Transaction {

	tx := types.Transaction{
		AccountNonce: nonce,
		Type:         types.SendTx,
		To:           &to,
		Amount:       amount,
		Epoch:        epoch,
	}

	signedTx, _ := types.SignTx(&tx, key)

	return signedTx
}
