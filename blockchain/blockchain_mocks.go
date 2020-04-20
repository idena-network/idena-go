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
	"github.com/idena-network/idena-go/keystore"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/tendermint/tm-db"
	"math/big"
)

func NewTestBlockchainWithConfig(withIdentity bool, conf *config.ConsensusConf, valConf *config.ValidationConfig, alloc map[common.Address]config.GenesisAllocation, queueSlots int, executableSlots int, executableLimit int, queueLimit int) (*TestBlockchain, *appstate.AppState, *mempool.TxPool, *ecdsa.PrivateKey) {
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
		Validation:       valConf,
		Blockchain:       &config.BlockchainConfig{},
		OfflineDetection: config.GetDefaultOfflineDetectionConfig(),
		Mempool: &config.Mempool{
			TxPoolQueueSlots:          queueSlots,
			TxPoolExecutableSlots:     executableSlots,
			TxPoolAddrExecutableLimit: executableLimit,
			TxPoolAddrQueueLimit:      queueLimit,
		},
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

	txPool := mempool.NewTxPool(appState, bus, cfg.Mempool, cfg.Consensus.MinFeePerByte)
	offline := NewOfflineDetector(cfg, db, appState, secStore, bus)
	keyStore := keystore.NewKeyStore("./testdata", keystore.StandardScryptN, keystore.StandardScryptP)

	chain := NewBlockchain(cfg, db, txPool, appState, ipfs.NewMemoryIpfsProxy(), secStore, bus, offline, keyStore)

	chain.InitializeChain()
	appState.Initialize(chain.Head.Height())
	txPool.Initialize(chain.Head, secStore.GetAddress())

	return &TestBlockchain{db, chain}, appState, txPool, key
}

func NewTestBlockchain(withIdentity bool, alloc map[common.Address]config.GenesisAllocation) (*TestBlockchain, *appstate.AppState, *mempool.TxPool, *ecdsa.PrivateKey) {
	cfg := config.GetDefaultConsensusConfig()
	cfg.Automine = true
	return NewTestBlockchainWithConfig(withIdentity, cfg, &config.ValidationConfig{}, alloc, -1, -1, 0, 0)
}

func NewTestBlockchainWithBlocks(blocksCount int, emptyBlocksCount int) (*TestBlockchain, *appstate.AppState) {
	key, _ := crypto.GenerateKey()
	return NewCustomTestBlockchain(blocksCount, emptyBlocksCount, key)
}

func NewCustomTestBlockchain(blocksCount int, emptyBlocksCount int, key *ecdsa.PrivateKey) (*TestBlockchain, *appstate.AppState) {
	addr := crypto.PubkeyToAddress(key.PublicKey)
	consensusCfg := config.GetDefaultConsensusConfig()
	consensusCfg.Automine = true
	cfg := &config.Config{
		Network:   0x99,
		Consensus: consensusCfg,
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
	if cfg.OfflineDetection == nil {
		cfg.OfflineDetection = config.GetDefaultOfflineDetectionConfig()
	}
	txPool := mempool.NewTxPool(appState, bus, config.GetDefaultMempoolConfig(), cfg.Consensus.MinFeePerByte)
	offline := NewOfflineDetector(cfg, db, appState, secStore, bus)
	keyStore := keystore.NewKeyStore("./testdata", keystore.StandardScryptN, keystore.StandardScryptP)

	chain := NewBlockchain(cfg, db, txPool, appState, ipfs.NewMemoryIpfsProxy(), secStore, bus, offline, keyStore)
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
	consensusCfg := config.GetDefaultConsensusConfig()
	consensusCfg.Automine = true
	cfg := &config.Config{
		Network:   0x99,
		Consensus: consensusCfg,
		GenesisConf: &config.GenesisConf{
			Alloc:             nil,
			GodAddress:        chain.secStore.GetAddress(),
			FirstCeremonyTime: 4070908800, //01.01.2099
		},
		Validation:       &config.ValidationConfig{},
		Blockchain:       &config.BlockchainConfig{},
		OfflineDetection: config.GetDefaultOfflineDetectionConfig(),
	}
	txPool := mempool.NewTxPool(appState, bus, config.GetDefaultMempoolConfig(), cfg.Consensus.MinFeePerByte)
	offline := NewOfflineDetector(cfg, db, appState, chain.secStore, bus)
	keyStore := keystore.NewKeyStore("./testdata", keystore.StandardScryptN, keystore.StandardScryptP)

	copy := NewBlockchain(cfg, db, txPool, appState, ipfs.NewMemoryIpfsProxy(), chain.secStore, bus, offline, keyStore)
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
	cert := types.FullBlockCert{Votes: []*types.Vote{vote}}
	chain.WriteCertificate(block.Header.Hash(), cert.Compress(), true)
}

func (chain *TestBlockchain) GenerateBlocks(count int) *TestBlockchain {
	for i := 0; i < count; i++ {
		block := chain.ProposeBlock()
		block.Block.Header.ProposedHeader.Time = big.NewInt(0).Add(chain.Head.Time(), big.NewInt(20))
		err := chain.AddBlock(block.Block, nil, collector.NewStatsCollector())
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
		err := chain.AddBlock(block, nil, collector.NewStatsCollector())
		if err != nil {
			panic(err)
		}
		chain.addCert(block)
	}
	return chain
}
