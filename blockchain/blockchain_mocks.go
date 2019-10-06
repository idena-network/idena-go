package blockchain

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/blockchain/types"
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
		MaxSteps:                          150,
		CommitteePercent:                  0.3,
		FinalCommitteePercent:             0.7,
		AgreementThreshold:                0.65,
		WaitBlockDelay:                    time.Minute,
		WaitSortitionProofDelay:           time.Second * 5,
		EstimatedBaVariance:               time.Second * 5,
		WaitForStepDelay:                  time.Second * 20,
		Automine:                          automine,
		BlockReward:                       big.NewInt(0).Mul(big.NewInt(1e+18), big.NewInt(15)),
		StakeRewardRate:                   0.2,
		FeeBurnRate:                       0.9,
		FinalCommitteeReward:              big.NewInt(6e+18),
		SnapshotRange:                     10000,
		OfflinePenaltyBlocksCount:         1800,
		SuccessfulValidationRewardPercent: 0.24,
		FlipRewardPercent:                 0.32,
		ValidInvitationRewardPercent:      0.32,
		FoundationPayoutsPercent:          0.1,
		ZeroWalletPercent:                 0.02,
		FirstInvitationRewardCoef:         1.0,
		SecondInvitationRewardCoef:        3.0,
		ThirdInvitationRewardCoef:         6.0,
		FeeSensitivityCoef:                0.25,
		MinFeePerByte:                     big.NewInt(1e+4),
		StatusSwitchRange:                 50,
	}
}

func NewTestBlockchainWithConfig(withIdentity bool, conf *config.ConsensusConf, valConf *config.ValidationConfig, alloc map[common.Address]config.GenesisAllocation, totalTxLimit int, addrTxLimit int) (*TestBlockchain, *appstate.AppState, *mempool.TxPool, *ecdsa.PrivateKey) {
	if alloc == nil {
		alloc = make(map[common.Address]config.GenesisAllocation)
	}
	key, _ := crypto.GenerateKey()
	secStore := secstore.NewSecStore()

	secStore.AddKey(crypto.FromECDSA(key))
	cfg := &config.Config{
		Network:   0x99,
		Consensus: conf,
		GenesisConf: &config.GenesisConf{
			Alloc:      alloc,
			GodAddress: secStore.GetAddress(),
		},
		Validation: valConf,
		Blockchain: &config.BlockchainConfig{},
	}

	db := db.NewMemDB()
	bus := eventbus.New()
	appState := appstate.NewAppState(db, bus)

	if withIdentity {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		cfg.GenesisConf.Alloc[addr] = config.GenesisAllocation{
			State: uint8(state.Verified),
		}
	}

	txPool := mempool.NewTxPool(appState, bus, totalTxLimit, addrTxLimit, cfg.Consensus.MinFeePerByte)
	offline := NewOfflineDetector(config.GetDefaultOfflineDetectionConfig(), db, appState, secStore, bus)

	chain := NewBlockchain(cfg, db, txPool, appState, ipfs.NewMemoryIpfsProxy(), secStore, bus, offline)

	chain.InitializeChain()
	appState.Initialize(chain.Head.Height())
	txPool.Initialize(chain.Head, secStore.GetAddress())

	return &TestBlockchain{db, chain}, appState, txPool, key
}

func NewTestBlockchain(withIdentity bool, alloc map[common.Address]config.GenesisAllocation) (*TestBlockchain, *appstate.AppState, *mempool.TxPool, *ecdsa.PrivateKey) {
	return NewTestBlockchainWithConfig(withIdentity, GetDefaultConsensusConfig(true), &config.ValidationConfig{}, alloc, -1, -1)
}

func NewTestBlockchainWithBlocks(blocksCount int, emptyBlocksCount int) (*TestBlockchain, *appstate.AppState) {
	key, _ := crypto.GenerateKey()
	return NewCustomTestBlockchain(blocksCount, emptyBlocksCount, key)
}

func NewCustomTestBlockchain(blocksCount int, emptyBlocksCount int, key *ecdsa.PrivateKey) (*TestBlockchain, *appstate.AppState) {
	addr := crypto.PubkeyToAddress(key.PublicKey)
	cfg := &config.Config{
		Network:   0x99,
		Consensus: GetDefaultConsensusConfig(true),
		GenesisConf: &config.GenesisConf{
			Alloc:             nil,
			GodAddress:        addr,
			FirstCeremonyTime: 4070908800, //01.01.2099
		},
		Validation: &config.ValidationConfig{},
		Blockchain: &config.BlockchainConfig{},
	}
	return NewCustomTestBlockchainWithConfig(blocksCount, emptyBlocksCount, key, cfg)
}

func NewCustomTestBlockchainWithConfig(blocksCount int, emptyBlocksCount int, key *ecdsa.PrivateKey, cfg *config.Config) (*TestBlockchain, *appstate.AppState) {
	db := db.NewMemDB()
	bus := eventbus.New()
	appState := appstate.NewAppState(db, bus)
	secStore := secstore.NewSecStore()
	secStore.AddKey(crypto.FromECDSA(key))

	txPool := mempool.NewTxPool(appState, bus, -1, -1, cfg.Consensus.MinFeePerByte)
	offline := NewOfflineDetector(config.GetDefaultOfflineDetectionConfig(), db, appState, secStore, bus)

	chain := NewBlockchain(cfg, db, txPool, appState, ipfs.NewMemoryIpfsProxy(), secStore, bus, offline, collector.NewBlockStatsCollector())
	chain.InitializeChain()
	appState.Initialize(chain.Head.Height())

	result := &TestBlockchain{db, chain}
	result.GenerateBlocks(blocksCount).GenerateEmptyBlocks(emptyBlocksCount)
	txPool.Initialize(chain.Head, secStore.GetAddress())
	return result, appState
}

type TestBlockchain struct {
	db db.DB
	*Blockchain
}

func (chain *TestBlockchain) Copy() (*TestBlockchain, *appstate.AppState) {
	db := db.NewMemDB()
	bus := eventbus.New()

	it, _ := chain.db.Iterator(nil, nil)
	defer it.Close()
	for ; it.Valid(); it.Next() {
		db.Set(it.Key(), it.Value())
	}
	appState := appstate.NewAppState(db, bus)
	cfg := &config.Config{
		Network:   0x99,
		Consensus: GetDefaultConsensusConfig(true),
		GenesisConf: &config.GenesisConf{
			Alloc:             nil,
			GodAddress:        chain.secStore.GetAddress(),
			FirstCeremonyTime: 4070908800, //01.01.2099
		},
		Validation: &config.ValidationConfig{},
		Blockchain: &config.BlockchainConfig{},
	}
	txPool := mempool.NewTxPool(appState, bus, -1, -1, cfg.Consensus.MinFeePerByte)
	offline := NewOfflineDetector(config.GetDefaultOfflineDetectionConfig(), db, appState, chain.secStore, bus)

	copy := NewBlockchain(cfg, db, txPool, appState, ipfs.NewMemoryIpfsProxy(), chain.secStore, bus, offline, collector.NewBlockStatsCollector())
	copy.InitializeChain()
	appState.Initialize(copy.Head.Height())
	return &TestBlockchain{db, copy}, appState
}

func (chain *TestBlockchain) addCert(block *types.Block) {
	vote := &types.Vote{
		Header: &types.VoteHeader{
			Round:       block.Height(),
			Step:        1,
			ParentHash:  block.Header.ParentHash(),
			VotedHash:   block.Header.Hash(),
			TurnOffline: false,
		},
	}
	vote.Signature = chain.secStore.Sign(vote.Header.SignatureHash().Bytes())
	chain.WriteCertificate(block.Header.Hash(), &types.BlockCert{Votes: []*types.Vote{vote}}, true)
}

func (chain *TestBlockchain) GenerateBlocks(count int) *TestBlockchain {
	for i := 0; i < count; i++ {
		block := chain.ProposeBlock()
		block.Block.Header.ProposedHeader.Time = big.NewInt(0).Add(chain.Head.Time(), big.NewInt(20))
		err := chain.AddBlock(block.Block, nil)
		if err != nil {
			panic(err)
		}
		chain.addCert(block.Block)
	}
	return chain
}

func (chain *TestBlockchain) GenerateEmptyBlocks(count int) *TestBlockchain {
	for i := 0; i < count; i++ {
		block := chain.GenerateEmptyBlock()
		err := chain.AddBlock(block, nil)
		if err != nil {
			panic(err)
		}
		chain.addCert(block)
	}
	return chain
}
