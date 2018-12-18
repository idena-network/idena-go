package blockchain

import (
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	db2 "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/common/math"
	"idena-go/config"
	"idena-go/core/appstate"
	"idena-go/core/mempool"
	"idena-go/core/state"
	"idena-go/crypto"
	"math/big"
	"testing"
	"time"
)

func getDefaultConsensusConfig(automine bool) *config.ConsensusConf {
	return &config.ConsensusConf{
		MaxSteps:                       150,
		CommitteePercent:               0.3,
		FinalCommitteeConsensusPercent: 0.7,
		ThesholdBa:                     0.65,
		ProposerTheshold:               0.5,
		WaitBlockDelay:                 time.Minute,
		WaitSortitionProofDelay:        time.Second * 5,
		EstimatedBaVariance:            time.Second * 5,
		WaitForStepDelay:               time.Second * 20,
		Automine:                       automine,
		BlockReward:                    big.NewInt(0).Mul(big.NewInt(1e+18), big.NewInt(15)),
		StakeRewardRate:                0.2,
		FeeBurnRate:                    0.9,
		FinalCommitteeReward:           big.NewInt(6e+18),
	}
}

func newBlockchain() *Blockchain {

	cfg := &config.Config{
		Network:   Testnet,
		Consensus: getDefaultConsensusConfig(true),
	}

	db := db2.NewMemDB()

	stateDb, _ := state.NewLatest(db)

	appState := appstate.NewAppState(stateDb)

	txPool := mempool.NewTxPool(appState)

	chain := NewBlockchain(cfg, db, txPool, appState)
	key, _ := crypto.GenerateKey()
	chain.InitializeChain(key)
	return chain
}

func Test_ApplyBlockRewards(t *testing.T) {
	chain := newBlockchain()

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

	s := state.NewForCheck(chain.appState.State)
	chain.applyBlockRewards(fee, s, block)

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

	require.Equal(t, 0, expectedBalance.Cmp(s.GetBalance(chain.coinBaseAddress)))
	require.Equal(t, 0, intStake.Cmp(s.GetStakeBalance(chain.coinBaseAddress)))
}
