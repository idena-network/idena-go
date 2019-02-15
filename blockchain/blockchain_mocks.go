package blockchain

import (
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/common"
	"idena-go/config"
	"idena-go/core/appstate"
	"idena-go/core/mempool"
	"idena-go/core/state"
	"idena-go/crypto"
	"math/big"
	"time"
)

func GetDefaultConsensusConfig(automine bool) *config.ConsensusConf {
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

func NewTestBlockchainWithConfig(withIdentity bool, conf *config.ConsensusConf, alloc map[common.Address]config.GenesisAllocation) (*Blockchain, *appstate.AppState, *mempool.TxPool) {
	if alloc == nil {
		alloc = make(map[common.Address]config.GenesisAllocation)
	}

	cfg := &config.Config{
		Network:   Testnet,
		Consensus: conf,
		GenesisConf: &config.GenesisConf{
			Alloc: alloc,
		},
	}

	db := db.NewMemDB()
	appState := appstate.NewAppState(db)

	key, _ := crypto.GenerateKey()

	if withIdentity {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		cfg.GenesisConf.Alloc[addr] = config.GenesisAllocation{
			State: state.Verified,
		}
	}

	txPool := mempool.NewTxPool(appState)

	chain := NewBlockchain(cfg, db, txPool, appState)

	chain.InitializeChain(key)
	appState.Initialize(chain.Head.Height())

	return chain, appState, txPool
}

func NewTestBlockchain(withIdentity bool, alloc map[common.Address]config.GenesisAllocation) (*Blockchain, *appstate.AppState, *mempool.TxPool) {
	return NewTestBlockchainWithConfig(withIdentity, GetDefaultConsensusConfig(true), alloc)
}
