package blockchain

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/secstore"
	"github.com/tendermint/tm-db"
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

func NewTestBlockchainWithConfig(withIdentity bool, conf *config.ConsensusConf, valConf *config.ValidationConfig, alloc map[common.Address]config.GenesisAllocation, totalTxLimit int, addrTxLimit int) (*Blockchain, *appstate.AppState, *mempool.TxPool) {
	if alloc == nil {
		alloc = make(map[common.Address]config.GenesisAllocation)
	}

	cfg := &config.Config{
		Network:   0x99,
		Consensus: conf,
		GenesisConf: &config.GenesisConf{
			Alloc: alloc,
		},
		Validation: valConf,
	}

	db := db.NewMemDB()
	appState := appstate.NewAppState(db, eventbus.New())

	key, _ := crypto.GenerateKey()
	secStore := secstore.NewSecStore()
	secStore.AddKey(crypto.FromECDSA(key))
	if withIdentity {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		cfg.GenesisConf.Alloc[addr] = config.GenesisAllocation{
			State: uint8(state.Verified),
		}
	}

	bus := eventbus.New()
	txPool := mempool.NewTxPool(appState, bus, totalTxLimit, addrTxLimit)
	offline := NewOfflineDetector(config.GetDefaultOfflineDetectionConfig(), db, appState, secStore, bus)

	chain := NewBlockchain(cfg, db, txPool, appState, ipfs.NewMemoryIpfsProxy(), secStore, bus, offline)

	chain.InitializeChain()
	appState.Initialize(chain.Head.Height())
	txPool.Initialize(chain.Head, secStore.GetAddress())

	return chain, appState, txPool
}

func NewTestBlockchain(withIdentity bool, alloc map[common.Address]config.GenesisAllocation) (*Blockchain, *appstate.AppState, *mempool.TxPool) {
	return NewTestBlockchainWithConfig(withIdentity, GetDefaultConsensusConfig(true), &config.ValidationConfig{}, alloc, -1, -1)
}

func NewTestBlockchainWithTxLimits(withIdentity bool, alloc map[common.Address]config.GenesisAllocation, totalTxLimit int, addrTxLimit int) (*Blockchain, *appstate.AppState, *mempool.TxPool) {
	return NewTestBlockchainWithConfig(withIdentity, GetDefaultConsensusConfig(true), &config.ValidationConfig{}, alloc, totalTxLimit, addrTxLimit)
}
