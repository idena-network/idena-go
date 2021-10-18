package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/blockchain/validation"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/core/upgrade"
	"github.com/idena-network/idena-go/core/validators"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/vrf/p256"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/keystore"
	"github.com/idena-network/idena-go/log"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/idena-network/idena-go/resources"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/idena-network/idena-go/subscriptions"
	"github.com/idena-network/idena-go/vm"
	cid2 "github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	dbm "github.com/tendermint/tm-db"
	math2 "math"
	"math/big"
	"math/rand"
	"sort"
	"time"
)

const (
	Mainnet types.Network = 0x0
	Testnet types.Network = 0x1
)

const (
	ProposerRole            uint8 = 0x1
	EmptyBlockTimeIncrement       = time.Second * 20
	MaxFutureBlockOffset          = time.Minute * 2
	MinBlockDelay                 = time.Second * 10
	StoreToIpfsThreshold          = 1 - fee.StoreToIpfsFeeCoef
)

var (
	ParentHashIsInvalid = errors.New("parentHash is invalid")
	BlockInsertionErr   = errors.New("can't insert block")
)

type Blockchain struct {
	repo            *database.Repo
	secStore        *secstore.SecStore
	Head            *types.Header
	PreliminaryHead *types.Header
	genesisInfo     *types.GenesisInfo
	config          *config.Config
	pubKey          []byte
	coinBaseAddress common.Address
	log             log.Logger
	txpool          *mempool.TxPool
	appState        *appstate.AppState
	offlineDetector *OfflineDetector
	indexer         *indexer
	secretKey       *ecdsa.PrivateKey
	ipfs            ipfs.Proxy
	timing          *timing
	bus             eventbus.Bus
	subManager      *subscriptions.Manager
	upgrader        *upgrade.Upgrader
	applyNewEpochFn func(height uint64, appState *appstate.AppState, collector collector.StatsCollector) (int, map[common.ShardId]*types.ValidationResults, bool)
	isSyncing       bool
	ipfsLoadQueue   chan *attachments.StoreToIpfsAttachment
}

type txsExecutionContext struct {
	appState       *appstate.AppState
	header         *types.Header
	blockInsertion bool
	statsCollector collector.StatsCollector
}

type txExecutionContext struct {
	appState       *appstate.AppState
	vm             vm.VM
	blockInsertion bool
	height         uint64
	statsCollector collector.StatsCollector
}

type blockInsertionResult struct {
	stateDiff         []*state.StateTreeDiff
	identityStateDiff *state.IdentityStateDiff
	txReceipts        types.TxReceipts
	txTasks           []task
}

func NewBlockchain(config *config.Config, db dbm.DB, txpool *mempool.TxPool, appState *appstate.AppState,
	ipfs ipfs.Proxy, secStore *secstore.SecStore, bus eventbus.Bus, offlineDetector *OfflineDetector, keyStore *keystore.KeyStore, subManager *subscriptions.Manager, upgrader *upgrade.Upgrader) *Blockchain {
	return &Blockchain{
		repo:            database.NewRepo(db),
		config:          config,
		log:             log.New(),
		txpool:          txpool,
		appState:        appState,
		ipfs:            ipfs,
		timing:          NewTiming(config.Validation),
		bus:             bus,
		secStore:        secStore,
		offlineDetector: offlineDetector,
		indexer:         newBlockchainIndexer(db, bus, config, keyStore),
		subManager:      subManager,
		upgrader:        upgrader,
		ipfsLoadQueue:   make(chan *attachments.StoreToIpfsAttachment, 100),
	}
}

func (chain *Blockchain) ProvideApplyNewEpochFunc(fn func(height uint64, appState *appstate.AppState, collector collector.StatsCollector) (int, map[common.ShardId]*types.ValidationResults, bool)) {
	chain.applyNewEpochFn = fn
}

func (chain *Blockchain) GetHead() *types.Header {
	head := chain.repo.ReadHead()
	return head
}

func (chain *Blockchain) Network() types.Network {
	return chain.config.Network
}

func (chain *Blockchain) Config() *config.Config {
	return chain.config
}

func (chain *Blockchain) Indexer() *indexer {
	return chain.indexer
}
func (chain *Blockchain) tryUpgrade(block *types.Header) {
	target := chain.upgrader.Target()
	if target == chain.config.Consensus.Version {
		return
	}
	if block.ProposedHeader != nil && block.ProposedHeader.Upgrade == uint32(chain.upgrader.Target()) {
		chain.log.Info("Detected upgrade block", "upgrade", block.ProposedHeader.Upgrade)
		chain.repo.WriteConsensusVersion(nil, block.ProposedHeader.Upgrade)
		chain.upgrader.CompleteMigration()
		diff := time.Unix(block.Time(), 0).Add(chain.config.Consensus.MigrationTimeout).Sub(time.Now().UTC())
		if diff > 0 {
			// pause block producing to allow weak machines process state migration on time
			chain.log.Info("Node goes to sleep", "duration", diff.String())
			time.Sleep(diff)
		}
	}
}
func (chain *Blockchain) InitializeChain() error {

	chain.coinBaseAddress = chain.secStore.GetAddress()
	chain.pubKey = chain.secStore.GetPubKey()
	head := chain.GetHead()
	if head != nil {
		chain.setCurrentHead(head)
		chain.tryUpgrade(head)

		genesisHeight := uint64(1)

		if chain.config.Network == Testnet {
			predefinedState, err := readPredefinedState()
			if err == nil {
				genesisHeight = predefinedState.Block
			} else {
				bindataGenesis, err := chain.readBindataGenesis()
				if err == nil {
					genesisHeight = bindataGenesis.Height()
				}
			}
		}

		predefinedGenesis := chain.GetBlockHeaderByHeight(genesisHeight)
		if predefinedGenesis == nil {
			return errors.New("genesis block is not found")
		}

		intermediateGenesisHeight := chain.repo.ReadIntermediateGenesis()
		if intermediateGenesisHeight == 0 || intermediateGenesisHeight == predefinedGenesis.Height() {
			chain.genesisInfo = &types.GenesisInfo{Genesis: predefinedGenesis}
		} else {
			genesis := chain.GetBlockHeaderByHeight(intermediateGenesisHeight)
			if genesis == nil {
				return errors.New("intermediate genesis block is not found")
			}
			chain.genesisInfo = &types.GenesisInfo{Genesis: genesis, OldGenesis: predefinedGenesis}
		}

	} else {
		_, err := chain.GenerateGenesis(chain.config.Network)
		if err != nil {
			return err
		}
	}
	chain.indexer.initialize(chain.coinBaseAddress)
	chain.PreliminaryHead = chain.repo.ReadPreliminaryHead()
	go chain.ipfsLoad()
	log.Info("Chain initialized", "block", chain.Head.Hash().Hex(), "height", chain.Head.Height())
	log.Info("Coinbase address", "addr", chain.coinBaseAddress.Hex())
	return nil
}

func (chain *Blockchain) setCurrentHead(head *types.Header) {
	chain.Head = head
}

func (chain *Blockchain) setHead(height uint64, batch dbm.Batch) {
	chain.repo.SetHead(batch, height)
	chain.setCurrentHead(chain.GetHead())
}

func (chain *Blockchain) readBindataGenesis() (*types.Header, error) {
	data, err := resources.IntermediateGenesisHeader()
	if err != nil {
		return nil, err
	}
	header := &types.Header{}
	if err = header.FromBytes(data); err != nil {
		return nil, err
	}
	return header, nil
}

func (chain *Blockchain) loadPredefinedGenesis(network types.Network) (*types.Block, error) {

	if network != Testnet {
		return nil, errors.New(fmt.Sprintf("predefined genesis for network=%v was not found", network))
	}

	header, err := chain.readBindataGenesis()
	if err != nil {
		return nil, err
	}
	stateDbData, err := resources.StateDb()
	if err != nil {
		return nil, err
	}
	defer stateDbData.Close()

	if err = chain.appState.State.RecoverSnapshot(header.Height(), header.Root(), stateDbData); err != nil {
		return nil, err
	}

	chain.appState.State.CommitSnapshot(header.Height(), nil)

	identityStateDbData, err := resources.IdentityStateDb()
	if err != nil {
		return nil, err
	}
	defer identityStateDbData.Close()

	if err = chain.appState.IdentityState.RecoverSnapshot(header.Height(), header.IdentityRoot(), identityStateDbData); err != nil {
		return nil, err
	}

	chain.appState.IdentityState.CommitSnapshot(header.Height())

	return &types.Block{Header: header, Body: &types.Body{}}, nil
}

func (chain *Blockchain) generateGenesis(network types.Network) (*types.Block, error) {
	for addr, alloc := range chain.config.GenesisConf.Alloc {
		if alloc.Balance != nil {
			chain.appState.State.SetBalance(addr, alloc.Balance)
		}
		if alloc.Stake != nil {
			chain.appState.State.AddStake(addr, alloc.Stake)
		}
		chain.appState.State.SetState(addr, state.IdentityState(alloc.State))
		if state.IdentityState(alloc.State).NewbieOrBetter() {
			chain.appState.IdentityState.Add(addr)
		}
	}

	chain.appState.State.SetGodAddress(chain.config.GenesisConf.GodAddress)

	seed := types.Seed(crypto.Keccak256Hash(append([]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6}, common.ToBytes(network)...)))
	blockNumber := uint64(1)
	var feePerGas *big.Int

	if network == Testnet {
		predefinedState, err := readPredefinedState()
		if err != nil {
			return nil, err
		}

		blockNumber = predefinedState.Block
		seed = types.BytesToSeed(predefinedState.Seed)
		feePerGas = common.BigIntOrNil(predefinedState.Global.FeePerGas)

		err = chain.appState.CommitAt(blockNumber - 1)
		if err != nil {
			return nil, err
		}

		chain.appState.SetPredefinedState(predefinedState)
	} else {
		nextValidationTimestamp := chain.config.GenesisConf.FirstCeremonyTime
		if nextValidationTimestamp == 0 {
			nextValidationTimestamp = time.Now().UTC().Unix()
		}
		chain.appState.State.SetNextValidationTime(time.Unix(nextValidationTimestamp, 0))
		chain.appState.State.SetFlipWordsSeed(seed)
		chain.appState.State.ClearStatusSwitchAddresses()
		if chain.config.GenesisConf.GodAddressInvites > 0 {
			chain.appState.State.SetGodAddressInvites(chain.config.GenesisConf.GodAddressInvites)
		} else {
			chain.appState.State.SetGodAddressInvites(common.GodAddressInvitesCount(0))
		}

		log.Info("Next validation time", "time", chain.appState.State.NextValidationTime().String(), "unix", nextValidationTimestamp)
	}

	if err := chain.appState.Commit(nil); err != nil {
		return nil, err
	}

	var emptyHash [32]byte

	return &types.Block{Header: &types.Header{
		ProposedHeader: &types.ProposedHeader{
			ParentHash:   emptyHash,
			Time:         0,
			Height:       blockNumber,
			Root:         chain.appState.State.Root(),
			IdentityRoot: chain.appState.IdentityState.Root(),
			BlockSeed:    seed,
			IpfsHash:     ipfs.EmptyCid.Bytes(),
			FeePerGas:    feePerGas,
		},
	}, Body: &types.Body{}}, nil
}

func (chain *Blockchain) GenerateGenesis(network types.Network) (*types.Block, error) {

	block, err := chain.loadPredefinedGenesis(network)
	if err != nil {
		block, err = chain.generateGenesis(network)
		if err != nil {
			return nil, err
		}
	}

	if err := chain.insertBlock(block, new(state.IdentityStateDiff), nil); err != nil {
		return nil, err
	}
	chain.genesisInfo = &types.GenesisInfo{Genesis: block.Header}
	return block, nil
}

func (chain *Blockchain) generateEmptyBlock(checkState *appstate.AppState, prevBlock *types.Header, statsCollector collector.StatsCollector) (*types.Block, *blockInsertionResult) {
	prevTimestamp := time.Unix(prevBlock.Time(), 0)

	block := &types.Block{
		Header: &types.Header{
			EmptyBlockHeader: &types.EmptyBlockHeader{
				ParentHash: prevBlock.Hash(),
				Height:     prevBlock.Height() + 1,
				Time:       prevTimestamp.Add(EmptyBlockTimeIncrement).Unix(),
			},
		},
		Body: &types.Body{},
	}

	block.Header.EmptyBlockHeader.BlockSeed = types.Seed(crypto.Keccak256Hash(getSeedData(prevBlock)))
	block.Header.EmptyBlockHeader.Flags = chain.calculateFlags(checkState, block, prevBlock)

	_, _, stateDiff, identityStateDiff := chain.applyEmptyBlockOnState(checkState, block, statsCollector)

	block.Header.EmptyBlockHeader.Root = checkState.State.Root()
	block.Header.EmptyBlockHeader.IdentityRoot = checkState.IdentityState.Root()
	return block, &blockInsertionResult{stateDiff: stateDiff, identityStateDiff: identityStateDiff}
}

func (chain *Blockchain) GenerateEmptyBlock() *types.Block {
	appState, _ := chain.appState.ForCheck(chain.Head.Height())
	block, _ := chain.generateEmptyBlock(appState, chain.Head, nil)
	return block
}

func (chain *Blockchain) AddBlock(block *types.Block, checkState *appstate.AppState,
	statsCollector collector.StatsCollector) error {

	if err := validateBlockParentHash(block.Header, chain.Head); err != nil {
		return err
	}
	statsCollector.EnableCollecting()
	defer statsCollector.CompleteCollecting()
	if blockInsertionResult, err := chain.ValidateBlock(block, checkState, statsCollector); err != nil {
		return err
	} else {
		chain.appState.State.AddDiff(blockInsertionResult.stateDiff)
		chain.appState.IdentityState.AddDiff(block.Height(), blockInsertionResult.identityStateDiff)

		if chain.appState.State.Root() != block.Root() {
			chain.appState.Reset()
			return errors.New("invalid block root")
		}

		if chain.appState.IdentityState.Root() != block.IdentityRoot() {
			chain.appState.Reset()
			return errors.New("invalid block identity root")
		}

		if err := chain.appState.CommitTrees(block, blockInsertionResult.identityStateDiff); err != nil {
			chain.appState.Reset()
			return err
		}

		if err := chain.insertBlock(block, blockInsertionResult.identityStateDiff, blockInsertionResult.txReceipts); err != nil {
			return err
		}

		for _, task := range blockInsertionResult.txTasks {
			task()
		}

		if !chain.isSyncing {
			chain.txpool.ResetTo(block)
		}
		chain.tryUpgrade(block.Header)
		if block.Header.Flags().HasFlag(types.NewGenesis) {
			chain.repo.WriteIntermediateGenesis(nil, block.Header.Height())
			chain.genesisInfo.OldGenesis = chain.genesisInfo.Genesis
			chain.genesisInfo.Genesis = block.Header
		}
		chain.bus.Publish(&events.NewBlockEvent{
			Block: block,
		})
		if block.Header.Flags().HasFlag(types.ValidationFinished) {
			shardId, _ := chain.CoinbaseShard()
			log.Info("Coinbase shard", "shardId", shardId)
		}
		chain.RemovePreliminaryHead(nil)
		return nil
	}
}

func (chain *Blockchain) applyBlockOnState(appState *appstate.AppState, block *types.Block, prevBlock *types.Header, totalFee, totalTips *big.Int, usedGas uint64, statsCollector collector.StatsCollector) (root, identityRoot common.Hash, stateDiff []*state.StateTreeDiff, diff *state.IdentityStateDiff) {

	chain.applyStatusSwitch(appState, block)
	chain.applyDelayedOfflinePenalties(appState, block, statsCollector)
	undelegations := chain.applyDelegationSwitch(appState, block)
	chain.applyNewEpoch(appState, block, statsCollector)
	chain.applyBlockRewards(totalFee, totalTips, appState, block, prevBlock, statsCollector)
	chain.switchPoolsToOffline(appState, undelegations, block)
	chain.applyGlobalParams(appState, block, statsCollector)
	chain.applyNextBlockFee(appState, block, usedGas)
	chain.applyVrfProposerThreshold(appState, block)

	stateDiff, diff = appState.Precommit()

	return appState.State.Root(), appState.IdentityState.Root(), stateDiff, diff
}

func (chain *Blockchain) applyEmptyBlockOnState(
	appState *appstate.AppState,
	block *types.Block,
	statsCollector collector.StatsCollector,
) (root common.Hash, identityRoot common.Hash, stateDiff []*state.StateTreeDiff, identityStateDiff *state.IdentityStateDiff) {

	chain.applyStatusSwitch(appState, block)
	chain.applyDelayedOfflinePenalties(appState, block, statsCollector)
	undelegations := chain.applyDelegationSwitch(appState, block)
	chain.applyNewEpoch(appState, block, statsCollector)
	chain.switchPoolsToOffline(appState, undelegations, block)

	chain.applyGlobalParams(appState, block, statsCollector)
	chain.applyVrfProposerThreshold(appState, block)
	stateDiff, identityStateDiff = appState.Precommit()

	return appState.State.Root(), appState.IdentityState.Root(), stateDiff, identityStateDiff
}

func (chain *Blockchain) applyBlockRewards(totalFee *big.Int, totalTips *big.Int, appState *appstate.AppState,
	block *types.Block, prevBlock *types.Header, statsCollector collector.StatsCollector) {

	// calculate fee reward
	burnFee := decimal.NewFromBigInt(totalFee, 0)
	burnFee = burnFee.Mul(decimal.NewFromFloat32(chain.config.Consensus.FeeBurnRate))
	intBurn := math.ToInt(burnFee)
	intFeeReward := new(big.Int)
	intFeeReward.Sub(totalFee, intBurn)

	totalReward := big.NewInt(0).Add(chain.config.Consensus.BlockReward, intFeeReward)
	totalReward.Add(totalReward, totalTips)

	coinbase := block.Header.Coinbase()

	status := appState.State.GetIdentityState(coinbase)
	stakeDest := coinbase

	if appState.ValidatorsCache.IsPool(coinbase) {
		delegationNonce := appState.State.GetIdentity(coinbase).DelegationNonce
		subIdentity, newNonce := appState.ValidatorsCache.FindSubIdentity(coinbase, delegationNonce)
		appState.State.SetDelegationNonce(coinbase, newNonce)

		status = appState.State.GetIdentityState(subIdentity)
		stakeDest = subIdentity
	}

	reward, stake := splitReward(totalReward, status == state.Newbie, chain.config.Consensus)

	// calculate penalty
	balanceAdd, stakeAdd, penaltySub := calculatePenalty(reward, stake, appState.State.GetPenalty(coinbase))

	collector.BeginProposerRewardBalanceUpdate(statsCollector, coinbase, stakeDest, appState)
	// update state
	appState.State.AddBalance(coinbase, balanceAdd)
	appState.State.AddStake(stakeDest, stakeAdd)
	if penaltySub != nil {
		appState.State.SubPenalty(coinbase, penaltySub)
	}
	collector.CompleteBalanceUpdate(statsCollector, appState)
	collector.AddMintedCoins(statsCollector, chain.config.Consensus.BlockReward)
	collector.AfterAddStake(statsCollector, stakeDest, stake, appState)
	collector.AfterSubPenalty(statsCollector, coinbase, penaltySub, appState)
	collector.AddPenaltyBurntCoins(statsCollector, coinbase, penaltySub)
	collector.AddProposerReward(statsCollector, coinbase, stakeDest, reward, stake)

	chain.rewardFinalCommittee(appState, block, prevBlock, statsCollector)
}

func calculatePenalty(balanceAppend *big.Int, stakeAppend *big.Int, currentPenalty *big.Int) (balanceAdd *big.Int, stakeAdd *big.Int, penaltySub *big.Int) {

	if common.ZeroOrNil(currentPenalty) {
		return balanceAppend, stakeAppend, nil
	}

	// penalty is less than added balance
	if balanceAppend.Cmp(currentPenalty) >= 0 {
		return new(big.Int).Sub(balanceAppend, currentPenalty), stakeAppend, currentPenalty
	}

	remainPenalty := new(big.Int).Sub(currentPenalty, balanceAppend)

	// remain penalty is less than added stake
	if stakeAppend.Cmp(remainPenalty) >= 0 {
		return big.NewInt(0), new(big.Int).Sub(stakeAppend, remainPenalty), currentPenalty
	}

	return big.NewInt(0), big.NewInt(0), new(big.Int).Add(balanceAppend, stakeAppend)
}

func (chain *Blockchain) applyNewEpoch(appState *appstate.AppState, block *types.Block,
	statsCollector collector.StatsCollector) {

	if !block.Header.Flags().HasFlag(types.ValidationFinished) {
		return
	}
	networkSize, validationResults, failed := chain.applyNewEpochFn(block.Height(), appState, statsCollector)
	totalInvitesCount := float32(networkSize) * chain.config.Consensus.InvitesPercent
	totalNewbies, totalVerified, totalSuspended, newbiesByShard, verifiedByShard, suspendedByShard := setNewIdentitiesAttributes(appState, totalInvitesCount, networkSize, failed, validationResults, statsCollector)
	epochBlock := appState.State.EpochBlock()
	if !failed {
		var epochDurations []uint32
		prevEpochBlocks := appState.State.PrevEpochBlocks()
		epochBlocks := append(prevEpochBlocks, []uint64{epochBlock, block.Height()}...)
		epochDurationsLen := len(epochBlocks) - 1
		epochDurations = make([]uint32, 0, epochDurationsLen)
		for i := 0; i < epochDurationsLen; i++ {
			epochDurations = append(epochDurations, uint32(epochBlocks[i+1]-epochBlocks[i]))
		}
		rewardValidIdentities(appState, chain.config.Consensus, validationResults, epochDurations, block.Seed(),
			statsCollector)
		balanceShards(appState, totalNewbies, totalVerified, totalSuspended, newbiesByShard, verifiedByShard, suspendedByShard)
	}

	clearDustAccounts(appState, networkSize, statsCollector)

	appState.State.IncEpoch()

	validationTime := appState.State.NextValidationTime()
	nextValidationTime := chain.config.Validation.GetNextValidationTime(validationTime, networkSize)
	appState.State.SetNextValidationTime(nextValidationTime)

	appState.State.SetFlipWordsSeed(block.Seed())

	appState.State.AddPrevEpochBlock(epochBlock)
	appState.State.SetEpochBlock(block.Height())
	appState.State.ClearEmptyBlocksByShard()
	appState.State.SetGodAddressInvites(common.GodAddressInvitesCount(networkSize))
}

func calculateNewIdentityStatusFlags(validationResults map[common.ShardId]*types.ValidationResults) map[common.Address]state.ValidationStatusFlag {
	m := make(map[common.Address]state.ValidationStatusFlag)
	for _, validationResult := range validationResults {
		for addr, item := range validationResult.AuthorResults {
			var status state.ValidationStatusFlag
			if item.HasOneReportedFlip {
				status |= state.AtLeastOneFlipReported
			}
			if item.HasOneNotQualifiedFlip {
				status |= state.AtLeastOneFlipNotQualified
			}
			if item.AllFlipsNotQualified {
				status |= state.AllFlipsNotQualified
			}
			m[addr] = status
		}
	}
	return m
}

type identityWithInvite struct {
	address common.Address
	score   float32
	state   state.IdentityState
}

func setInvites(appState *appstate.AppState, identitiesWithInvites []identityWithInvite, totalInvitesCount float32,
	statsCollector collector.StatsCollector) {

	if len(identitiesWithInvites) == 0 {
		return
	}

	currentInvitesCount := int(totalInvitesCount)

	var index int
	var identity identityWithInvite
	// first step, give 1 invite to humans
	for index, identity = range identitiesWithInvites {
		if identity.score < common.MinHumanTotalScore {
			break
		}
		if identity.state != state.Human {
			continue
		}
		appState.State.AddInvite(identity.address, 1)
		currentInvitesCount -= 1

		if currentInvitesCount <= 0 {
			break
		}
	}

	if currentInvitesCount == 0 {
		lastScore := identitiesWithInvites[index].score
		for i := index + 1; i < len(identitiesWithInvites) && identitiesWithInvites[i].score == lastScore; i++ {
			if identitiesWithInvites[i].state == state.Human {
				appState.State.AddInvite(identitiesWithInvites[i].address, 1)
			}
		}
		collector.SetMinScoreForInvite(statsCollector, lastScore)
		return
	}

	//  second step, distribute remained invites
	for index, identity = range identitiesWithInvites {
		appState.State.AddInvite(identity.address, 1)
		currentInvitesCount -= 1
		if currentInvitesCount <= 0 {
			break
		}
	}

	lastScore := identitiesWithInvites[index].score
	for i := index + 1; i < len(identitiesWithInvites) && identitiesWithInvites[i].score == lastScore; i++ {
		appState.State.AddInvite(identitiesWithInvites[i].address, 1)
	}
	collector.SetMinScoreForInvite(statsCollector, lastScore)
}

func setNewIdentitiesAttributes(appState *appstate.AppState, totalInvitesCount float32, networkSize int, validationFailed bool, validationResults map[common.ShardId]*types.ValidationResults, statsCollector collector.StatsCollector) (int, int, int, map[common.ShardId]int, map[common.ShardId]int, map[common.ShardId]int) {
	_, flips := common.NetworkParams(networkSize)
	identityFlags := calculateNewIdentityStatusFlags(validationResults)

	identitiesWithInvites := make([]identityWithInvite, 0)
	addIdentityWithInvite := func(elem identityWithInvite) {
		index := sort.Search(len(identitiesWithInvites), func(i int) bool {
			return identitiesWithInvites[i].score < elem.score
		})
		identitiesWithInvites = append(identitiesWithInvites, identityWithInvite{})
		copy(identitiesWithInvites[index+1:], identitiesWithInvites[index:])
		identitiesWithInvites[index] = elem
	}

	newbiesByShard := map[common.ShardId]int{}
	verifiedByShard := map[common.ShardId]int{}
	suspendedByShard := map[common.ShardId]int{}

	var totalNewbies, totalVerified, totalSuspended int

	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		if !validationFailed {
			switch identity.State {
			case state.Verified, state.Human:
				removeLinkWithInviter(appState.State, addr)
				totalPoints, totalFlips := common.CalculateIdentityScores(identity.Scores, identity.GetShortFlipPoints(), identity.QualifiedFlips)
				addIdentityWithInvite(identityWithInvite{
					address: addr,
					score:   totalPoints / float32(totalFlips),
					state:   identity.State,
				})
				appState.State.SetRequiredFlips(addr, uint8(flips))
				appState.IdentityState.Add(addr)
				if identity.Delegatee != nil {
					appState.IdentityState.SetDelegatee(addr, *identity.Delegatee)
				}
				verifiedByShard[identity.ShiftedShardId()]++
				totalVerified++
			case state.Newbie:
				appState.State.SetRequiredFlips(addr, uint8(flips))
				appState.IdentityState.Add(addr)
				if identity.Delegatee != nil {
					appState.IdentityState.SetDelegatee(addr, *identity.Delegatee)
				}
				newbiesByShard[identity.ShiftedShardId()]++
				totalNewbies++
			case state.Killed, state.Undefined:
				removeLinksWithInviterAndInvitees(appState.State, addr)
				appState.State.SetRequiredFlips(addr, 0)
				appState.IdentityState.Remove(addr)
			case state.Suspended, state.Zombie:
				appState.State.SetRequiredFlips(addr, 0)
				appState.IdentityState.Remove(addr)
				suspendedByShard[identity.ShiftedShardId()]++
				totalSuspended++
			default:
				appState.State.SetRequiredFlips(addr, 0)
				appState.IdentityState.Remove(addr)
			}
			appState.State.SetInvites(addr, 0)
		}

		collector.BeforeClearPenalty(statsCollector, addr, appState)
		collector.BeginEpochPenaltyResetBalanceUpdate(statsCollector, addr, appState)
		appState.State.ClearPenalty(addr)
		collector.CompleteBalanceUpdate(statsCollector, appState)
		appState.State.ClearFlips(addr)
		appState.State.ResetValidationTxBits(addr)

		if status, ok := identityFlags[addr]; ok && !validationFailed {
			appState.State.SetValidationStatus(addr, status)
		} else {
			appState.State.SetValidationStatus(addr, 0)
		}
	})

	if !validationFailed {
		setInvites(appState, identitiesWithInvites, totalInvitesCount, statsCollector)
	}
	return totalNewbies, totalVerified, totalSuspended, newbiesByShard, verifiedByShard, suspendedByShard
}

func balanceShards(appState *appstate.AppState, totalNewbies, totalVerified, totalSuspended int, newbiesByShard, verifiedByShard, suspendedByShard map[common.ShardId]int) {
	prevShardsNum := appState.State.ShardsNum()
	newShardsNum := common.CalculateShardsNumber(common.MinShardSize, common.MaxShardSize, totalNewbies+totalVerified+totalSuspended, int(prevShardsNum))

	for i := common.ShardId(1); i <= common.ShardId(prevShardsNum); i++ {
		log.Info("Shard distribution before balancing", "shardId", i, "newbies", newbiesByShard[i], "verified,human", verifiedByShard[i], "suspended,zombie", suspendedByShard[i])
	}

	desiredNewbiesInShard := totalNewbies / newShardsNum
	desiredVerifiedInShard := totalVerified / newShardsNum
	desiredSuspendedInShard := totalSuspended / newShardsNum

	var verifiedForRelocation []common.Address
	var newbiesForRelocation []common.Address
	var suspendedForRelocation []common.Address

	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		switch identity.State {
		case state.Verified, state.Human:
			if verifiedByShard[identity.ShiftedShardId()] > desiredVerifiedInShard || identity.ShiftedShardId() >= common.ShardId(newShardsNum) {
				verifiedForRelocation = append(verifiedForRelocation, addr)
				verifiedByShard[identity.ShiftedShardId()]--
			}
		case state.Newbie:
			if newbiesByShard[identity.ShiftedShardId()] > desiredNewbiesInShard || identity.ShiftedShardId() >= common.ShardId(newShardsNum) {
				newbiesForRelocation = append(newbiesForRelocation, addr)
				newbiesByShard[identity.ShiftedShardId()]--
			}
		case state.Suspended, state.Zombie:
			if suspendedByShard[identity.ShiftedShardId()] > desiredSuspendedInShard || identity.ShiftedShardId() >= common.ShardId(newShardsNum) {
				suspendedForRelocation = append(suspendedForRelocation, addr)
				suspendedByShard[identity.ShiftedShardId()]--
			}
		}
	})

	rnd := rand.New(rand.NewSource(int64(totalNewbies + totalVerified + totalSuspended)))
	shuffledVerifiedIndexes := rnd.Perm(len(verifiedForRelocation))
	shuffledNewbiesIndexes := rnd.Perm(len(newbiesForRelocation))
	shuffledSuspendedIndexes := rnd.Perm(len(suspendedForRelocation))

	verifiedIdx := 0
	newbiesIdx := 0
	suspendedIdx := 0

	for shardId := common.ShardId(1); shardId <= common.ShardId(newShardsNum); shardId++ {
		for verifiedByShard[shardId] < desiredVerifiedInShard && verifiedIdx < len(shuffledVerifiedIndexes) {
			addr := verifiedForRelocation[shuffledVerifiedIndexes[verifiedIdx]]
			appState.State.SetShardId(addr, shardId)
			verifiedIdx++
			verifiedByShard[shardId]++
		}

		for newbiesByShard[shardId] < desiredNewbiesInShard && newbiesIdx < len(shuffledNewbiesIndexes) {
			addr := newbiesForRelocation[shuffledNewbiesIndexes[newbiesIdx]]
			appState.State.SetShardId(addr, shardId)
			newbiesIdx++
			newbiesByShard[shardId]++
		}
		for suspendedByShard[shardId] < desiredSuspendedInShard && suspendedIdx < len(shuffledSuspendedIndexes) {
			addr := suspendedForRelocation[shuffledSuspendedIndexes[suspendedIdx]]
			appState.State.SetShardId(addr, shardId)
			suspendedIdx++
			suspendedByShard[shardId]++
		}
	}
	shardId := common.ShardId(1)
	for verifiedIdx < len(shuffledVerifiedIndexes) {
		addr := verifiedForRelocation[shuffledVerifiedIndexes[verifiedIdx]]
		appState.State.SetShardId(addr, shardId)
		verifiedByShard[shardId]++
		shardId++
		if shardId > common.ShardId(newShardsNum) {
			shardId = 1
		}
		verifiedIdx++
	}
	for newbiesIdx < len(shuffledNewbiesIndexes) {
		addr := newbiesForRelocation[shuffledNewbiesIndexes[newbiesIdx]]
		appState.State.SetShardId(addr, shardId)
		newbiesByShard[shardId]++
		shardId++
		if shardId > common.ShardId(newShardsNum) {
			shardId = 1
		}
		newbiesIdx++
	}
	for suspendedIdx < len(shuffledSuspendedIndexes) {
		addr := suspendedForRelocation[shuffledSuspendedIndexes[suspendedIdx]]
		appState.State.SetShardId(addr, shardId)
		suspendedByShard[shardId]++
		shardId++
		if shardId > common.ShardId(newShardsNum) {
			shardId = 1
		}
		suspendedIdx++
	}

	for i := common.ShardId(1); i <= common.ShardId(newShardsNum); i++ {
		appState.State.SetShardSize(i, uint32(newbiesByShard[i]+verifiedByShard[i]+suspendedByShard[i]))
		log.Info("Shard distribution after balancing", "shardId", i, "newbies", newbiesByShard[i], "verified,human", verifiedByShard[i], "suspended,zombie", suspendedByShard[i])
	}

	appState.State.SetShardsNum(uint32(newShardsNum))

	log.Info("Sharding info", "prev shards count", prevShardsNum, "new shards count", newShardsNum)
}

func clearDustAccounts(appState *appstate.AppState, networkSize int, statsCollector collector.StatsCollector) {
	commonTxSize := big.NewInt(1000)
	minFeePerGas := fee.GetFeePerGasForNetwork(networkSize)
	commonTxCost := new(big.Int).Mul(commonTxSize, minFeePerGas)

	appState.State.IterateOverAccounts(func(addr common.Address, account state.Account) {
		if account.Balance == nil || account.Balance.Cmp(commonTxCost) == -1 {
			collector.BeginDustClearingBalanceUpdate(statsCollector, addr, appState)
			appState.State.ClearAccount(addr)
			collector.CompleteBalanceUpdate(statsCollector, appState)
		}
	})
}

func removeLinksWithInviterAndInvitees(stateDB *state.StateDB, addr common.Address) {
	removeLinkWithInviter(stateDB, addr)
	removeLinkWithInvitees(stateDB, addr)
}

func switchOnePoolToOffline(appState *appstate.AppState, pool common.Address, lostIdentities []common.Address) {
	if appState.ValidatorsCache.IsPool(pool) && appState.ValidatorsCache.IsOnlineIdentity(pool) &&
		appState.ValidatorsCache.PoolSizeExceptNodes(pool, lostIdentities) <= 0 {
		appState.IdentityState.SetOnline(pool, false)
	}
}

func removeLinkWithInviter(stateDB *state.StateDB, inviteeAddr common.Address) {
	inviter := stateDB.GetInviter(inviteeAddr)
	if inviter == nil {
		return
	}
	stateDB.ResetInviter(inviteeAddr)
	stateDB.RemoveInvitee(inviter.Address, inviteeAddr)
}

func removeLinkWithInvitees(stateDB *state.StateDB, inviterAddr common.Address) {
	for len(stateDB.GetInvitees(inviterAddr)) > 0 {
		invitee := stateDB.GetInvitees(inviterAddr)[0]
		stateDB.RemoveInvitee(inviterAddr, invitee.Address)
		stateDB.ResetInviter(invitee.Address)
	}
}

func (chain *Blockchain) applyGlobalParams(appState *appstate.AppState, block *types.Block,
	statsCollector collector.StatsCollector) {

	if appState.State.ValidationPeriod() == state.AfterLongSessionPeriod && !block.IsEmpty() {

		proposerAddr, _ := crypto.PubKeyBytesToAddress(block.Header.ProposedHeader.ProposerPubKey)

		proposer := appState.State.GetIdentity(proposerAddr)
		proposerShardId := proposer.ShiftedShardId()

		ceremonialTxsByShard := map[common.ShardId]bool{}

		for _, tx := range block.Body.Transactions {
			if _, ok := types.CeremonialTxs[tx.Type]; ok {
				sender, _ := types.Sender(tx)
				senderIdentity := appState.State.GetIdentity(sender)
				ceremonialTxsByShard[senderIdentity.ShiftedShardId()] = true
			}
		}
		if len(ceremonialTxsByShard) == 0 {
			if !proposer.State.NewbieOrBetter() {
				randSeed := binary.LittleEndian.Uint64(block.Seed().Bytes())
				random := rand.New(rand.NewSource(int64(randSeed)*77 + 55))
				proposerShardId = common.ShardId(1 + random.Intn(int(appState.State.ShardsNum())))
			}
			appState.State.AddEmptyBlockByShard(appState.ValidatorsCache.OnlineSize(), proposerShardId, proposerAddr)
			appState.State.IncBlocksCntWithoutCeremonialTxs()
		} else {
			appState.State.ResetBlocksCntWithoutCeremonialTxs()
			if proposer.State.NewbieOrBetter() && !ceremonialTxsByShard[proposerShardId] {
				appState.State.AddEmptyBlockByShard(appState.ValidatorsCache.OnlineSize(), proposerShardId, proposerAddr)
			}
			for shardId := range ceremonialTxsByShard {
				appState.State.ResetEmptyBlockByShard(shardId)
			}
		}
	}

	flags := block.Header.Flags()
	if flags.HasFlag(types.FlipLotteryStarted) {
		appState.State.SetValidationPeriod(state.FlipLotteryPeriod)
	}

	if flags.HasFlag(types.ShortSessionStarted) {
		appState.State.SetValidationPeriod(state.ShortSessionPeriod)
	}

	if flags.HasFlag(types.LongSessionStarted) {
		appState.State.SetValidationPeriod(state.LongSessionPeriod)
	}

	if flags.HasFlag(types.AfterLongSessionStarted) {
		appState.State.SetValidationPeriod(state.AfterLongSessionPeriod)
	}

	if flags.HasFlag(types.ValidationFinished) {
		appState.State.ResetBlocksCntWithoutCeremonialTxs()
		appState.State.SetValidationPeriod(state.NonePeriod)
	}

	if flags.HasFlag(types.Snapshot) {
		appState.State.SetLastSnapshot(block.Height())
	}

	if flags.HasFlag(types.OfflineCommit) {
		addr := block.Header.OfflineAddr()
		chain.applyOfflinePenalty(appState, *addr)
	}
}

func (chain *Blockchain) calculatePenalty(appState *appstate.AppState, addr common.Address) *big.Int {
	networkSize := appState.ValidatorsCache.NetworkSize()
	totalBlockReward := new(big.Int).Add(chain.config.Consensus.FinalCommitteeReward, chain.config.Consensus.BlockReward)
	totalPenalty := new(big.Int).Mul(totalBlockReward, big.NewInt(chain.config.Consensus.OfflinePenaltyBlocksCount))
	coins := decimal.NewFromBigInt(totalPenalty, 0)
	res := coins.Div(decimal.New(int64(networkSize), 0))

	if appState.ValidatorsCache.IsPool(addr) {
		res = res.Mul(decimal.New(int64(appState.ValidatorsCache.PoolSize(addr)), 0))
	}
	return math.ToInt(res)
}

func (chain *Blockchain) applyOfflinePenalty(appState *appstate.AppState, addr common.Address) {
	networkSize := appState.ValidatorsCache.NetworkSize()

	if networkSize > 0 {
		appState.State.AddDelayedPenalty(addr)
		return
	}
	appState.IdentityState.SetOnline(addr, false)
}

func (chain *Blockchain) applyDelayedOfflinePenalties(appState *appstate.AppState, block *types.Block, statsCollector collector.StatsCollector) {
	if !block.Header.Flags().HasFlag(types.IdentityUpdate) {
		return
	}
	identities := appState.State.DelayedOfflinePenalties()
	for _, addr := range identities {
		amount := chain.calculatePenalty(appState, addr)
		collector.BeforeSetPenalty(statsCollector, addr, amount, appState)
		collector.BeginPenaltyBalanceUpdate(statsCollector, addr, appState)
		appState.State.SetPenalty(addr, amount)
		collector.CompleteBalanceUpdate(statsCollector, appState)
		appState.IdentityState.SetOnline(addr, false)
	}
	appState.State.ClearDelayedOfflinePenalties()
}

func (chain *Blockchain) rewardFinalCommittee(appState *appstate.AppState, block *types.Block, prevBlock *types.Header,
	statsCollector collector.StatsCollector) {
	if block.IsEmpty() {
		return
	}
	identities := appState.ValidatorsCache.GetOnlineValidators(prevBlock.Seed(), block.Height(), types.Final, chain.GetCommitteeSize(appState.ValidatorsCache, true))
	if identities == nil || identities.Addresses.Cardinality() == 0 {
		return
	}
	totalReward := big.NewInt(0)
	totalReward.Div(chain.config.Consensus.FinalCommitteeReward, big.NewInt(int64(identities.Original.Cardinality())))
	collector.SetCommitteeRewardShare(statsCollector, totalReward)

	reward, stake := splitReward(totalReward, false, chain.config.Consensus)
	newbieReward, newbieStake := splitReward(totalReward, true, chain.config.Consensus)

	for _, item := range identities.Original.ToSlice() {
		addr := item.(common.Address)

		identityState := appState.State.GetIdentityState(addr)
		r, s := reward, stake
		if identityState == state.Newbie {
			r, s = newbieReward, newbieStake
		}

		penaltySource := addr
		balanceDest := addr
		if delegator := appState.ValidatorsCache.Delegator(addr); !delegator.IsEmpty() {
			penaltySource = delegator
			balanceDest = delegator
		}
		// calculate penalty
		balanceAdd, stakeAdd, penaltySub := calculatePenalty(r, s, appState.State.GetPenalty(penaltySource))

		collector.BeginCommitteeRewardBalanceUpdate(statsCollector, balanceDest, addr, appState)
		// update state
		appState.State.AddBalance(balanceDest, balanceAdd)
		appState.State.AddStake(addr, stakeAdd)
		if penaltySub != nil {
			appState.State.SubPenalty(penaltySource, penaltySub)
		}
		collector.CompleteBalanceUpdate(statsCollector, appState)
		collector.AddMintedCoins(statsCollector, r)
		collector.AddMintedCoins(statsCollector, s)
		collector.AfterAddStake(statsCollector, addr, s, appState)
		collector.AfterSubPenalty(statsCollector, penaltySource, penaltySub, appState)
		collector.AddPenaltyBurntCoins(statsCollector, penaltySource, penaltySub)
		collector.AddFinalCommitteeReward(statsCollector, balanceDest, addr, r, s)
	}
}

func (chain *Blockchain) processTxs(txs []*types.Transaction, context *txsExecutionContext) (totalFee *big.Int, totalTips *big.Int, receipts types.TxReceipts, tasks []task, usedGas uint64, err error) {
	totalFee = new(big.Int)
	totalTips = new(big.Int)
	appState := context.appState
	header := context.header
	minFeePerGas := fee.GetFeePerGasForNetwork(appState.ValidatorsCache.NetworkSize())

	vm := vm.NewVmImpl(appState, header, chain.secStore, context.statsCollector, chain.config)

	for i := 0; i < len(txs); i++ {
		tx := txs[i]
		if err := validation.ValidateTx(appState, tx, minFeePerGas, validation.InBlockTx); err != nil {
			return nil, nil, nil, nil, 0, err
		}
		txContext := &txExecutionContext{
			appState:       appState,
			vm:             vm,
			blockInsertion: context.blockInsertion,
			height:         header.Height(),
			statsCollector: context.statsCollector,
		}
		if usedFee, receipt, task, err := chain.applyTxOnState(tx, txContext); err != nil {
			return nil, nil, nil, nil, 0, err
		} else {
			gas := uint64(fee.CalculateGas(tx))
			if receipt != nil {
				receipts = append(receipts, receipt)
				gas += receipt.GasUsed
			}
			if usedGas+gas > types.MaxBlockGas {
				return nil, nil, nil, nil, 0, errors.New("block exceeds gas limit")
			}
			usedGas += gas
			totalFee.Add(totalFee, usedFee)
			totalTips.Add(totalTips, tx.TipsOrZero())
			if task != nil {
				tasks = append(tasks, task)
			}
		}
	}
	return totalFee, totalTips, receipts, tasks, usedGas, nil
}

type task = func()

func (chain *Blockchain) applyTxOnState(tx *types.Transaction, context *txExecutionContext) (*big.Int, *types.TxReceipt, task, error) {

	statsCollector := context.statsCollector
	appState := context.appState

	collector.BeginApplyingTx(statsCollector, tx, appState)
	defer collector.CompleteApplyingTx(statsCollector, appState)

	stateDB := appState.State

	sender, _ := types.Sender(tx)

	globalState := stateDB.GetOrNewGlobalObject()
	senderAccount := stateDB.GetOrNewAccountObject(sender)

	if tx.Epoch != globalState.Epoch() {
		return nil, nil, nil, errors.New(fmt.Sprintf("invalid tx epoch. Tx=%v expectedEpoch=%v actualEpoch=%v", tx.Hash().Hex(),
			globalState.Epoch(), tx.Epoch))
	}

	currentNonce := senderAccount.Nonce()
	// if epoch was increased, we should reset nonce to 1
	if senderAccount.Epoch() < globalState.Epoch() {
		currentNonce = 0
	}

	if currentNonce+1 != tx.AccountNonce {
		return nil, nil, nil, errors.New(fmt.Sprintf("invalid tx nonce. Tx=%v expectedNonce=%v actualNonce=%v", tx.Hash().Hex(),
			currentNonce+1, tx.AccountNonce))
	}

	feePerGas := appState.State.FeePerGas()
	fee := chain.getTxFee(feePerGas, tx)
	totalCost := chain.getTxCost(feePerGas, tx)
	var receipt *types.TxReceipt
	var task task

	switch tx.Type {
	case types.ActivationTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)

		balance := stateDB.GetBalance(sender)
		generation, code := stateDB.GeneticCode(sender)
		balanceToTransfer := new(big.Int).Sub(balance, totalCost)

		// sub balance, which should be transferred, and kill temp identity
		stateDB.SubBalance(sender, balanceToTransfer)
		stateDB.SetState(sender, state.Killed)

		// verify identity and add transferred balance
		recipient := *tx.To
		stateDB.SetState(recipient, state.Candidate)
		stateDB.AddBalance(recipient, balanceToTransfer)
		stateDB.SetPubKey(recipient, tx.Payload)
		stateDB.SetGeneticCode(recipient, generation, code)
		candidateShard := chain.MinimalShard(appState)
		stateDB.SetShardId(recipient, candidateShard)
		stateDB.IncreaseShardSize(candidateShard)
		inviter := stateDB.GetInviter(sender)
		if inviter != nil {
			removeLinkWithInviter(appState.State, sender)
			if inviter.Address == stateDB.GodAddress() || stateDB.GetIdentityState(inviter.Address).VerifiedOrBetter() {
				var epochBlock uint32
				epochBlock = uint32(context.height - appState.State.EpochBlock())
				stateDB.AddInvitee(inviter.Address, recipient, inviter.TxHash)
				stateDB.SetInviter(recipient, inviter.Address, inviter.TxHash, epochBlock)
			}
		}

		collector.AddActivationTxBalanceTransfer(statsCollector, tx, balanceToTransfer)
		if sender != *tx.To {
			collector.AddInviteBurntCoins(statsCollector, sender, appState.State.GetStakeBalance(sender), tx)
		}
	case types.SendTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		stateDB.SubBalance(sender, tx.AmountOrZero())
		stateDB.AddBalance(*tx.To, tx.AmountOrZero())
	case types.BurnTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		stateDB.SubBalance(sender, tx.AmountOrZero())
		collector.AddBurnTxBurntCoins(statsCollector, sender, tx)
	case types.InviteTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		if sender == stateDB.GodAddress() {
			stateDB.SubGodAddressInvite()
		} else {
			stateDB.SubInvite(sender, 1)
		}

		stateDB.SubBalance(sender, tx.AmountOrZero())
		generation, code := stateDB.GeneticCode(sender)

		stateDB.SetState(*tx.To, state.Invite)
		stateDB.AddBalance(*tx.To, tx.AmountOrZero())
		stateDB.SetGeneticCode(*tx.To, generation+1, append(code[1:], sender[0]))

		stateDB.SetInviter(*tx.To, sender, tx.Hash(), 0)
	case types.KillTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		removeLinksWithInviterAndInvitees(stateDB, sender)
		if stateDB.GetIdentityState(sender).IsInShard() {
			stateDB.DecreaseShardSize(stateDB.ShardId(sender))
		}
		stateDB.SetState(sender, state.Killed)

		appState.IdentityState.Remove(sender)
		stake := stateDB.GetStakeBalance(sender)
		stateDB.SubStake(sender, stake)
		stateDB.AddBalance(sender, stake)
		collector.AddKillTxStakeTransfer(statsCollector, tx, stake)
	case types.KillInviteeTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		removeLinksWithInviterAndInvitees(stateDB, *tx.To)
		inviteePrevState := stateDB.GetIdentityState(*tx.To)
		if stateDB.GetIdentityState(*tx.To).IsInShard() {
			stateDB.DecreaseShardSize(stateDB.ShardId(*tx.To))
		}
		stateDB.SetState(*tx.To, state.Killed)
		appState.IdentityState.Remove(*tx.To)
		if inviteePrevState == state.Newbie {
			stake := stateDB.GetStakeBalance(*tx.To)
			stakeToTransfer := big.NewInt(0).Set(stake)

			// 1/6 of stake moves to balance, rest burns
			stakeToTransfer.Div(stake, big.NewInt(6))

			stateDB.AddBalance(sender, stakeToTransfer)
			stateDB.SubStake(*tx.To, stake)
			collector.AddKillInviteeTxStakeTransfer(statsCollector, tx, stake, stakeToTransfer)
		}
		if sender != stateDB.GodAddress() && stateDB.GetIdentityState(sender).VerifiedOrBetter() &&
			(inviteePrevState == state.Invite || inviteePrevState == state.Candidate) {
			stateDB.AddInvite(sender, 1)
		}
	case types.KillDelegatorTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		removeLinksWithInviterAndInvitees(stateDB, *tx.To)
		delegatorPrevState := stateDB.GetIdentityState(*tx.To)
		if stateDB.GetIdentityState(*tx.To).IsInShard() {
			stateDB.DecreaseShardSize(stateDB.ShardId(*tx.To))
		}
		stateDB.SetState(*tx.To, state.Killed)
		appState.IdentityState.Remove(*tx.To)
		stake := stateDB.GetStakeBalance(*tx.To)
		stateDB.SubStake(*tx.To, stake)
		if delegatorPrevState.VerifiedOrBetter() {
			stateDB.AddBalance(sender, stake)
		} else {
			collector.AddKilledBurntCoins(statsCollector, *tx.To, stake)
		}
	case types.SubmitFlipTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		attachment := attachments.ParseFlipSubmitAttachment(tx)
		stateDB.AddFlip(sender, attachment.Cid, attachment.Pair)
	case types.OnlineStatusTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		if stateDB.HasDelayedOfflinePenalty(sender) {
			stateDB.RemoveDelayedOfflinePenalty(sender)
		} else {
			stateDB.ToggleStatusSwitchAddress(sender)
		}
	case types.ChangeGodAddressTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		appState.State.SetGodAddress(*tx.To)
	case types.ChangeProfileTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		attachment := attachments.ParseChangeProfileAttachment(tx)
		stateDB.SetProfileHash(sender, attachment.Hash)
	case types.DeleteFlipTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		attachment := attachments.ParseDeleteFlipAttachment(tx)
		stateDB.DeleteFlip(sender, attachment.Cid)
	case types.SubmitAnswersHashTx, types.SubmitShortAnswersTx, types.EvidenceTx, types.SubmitLongAnswersTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		stateDB.SetValidationTxBit(sender, tx.Type)
	case types.DeployContractTx, types.CallContractTx, types.TerminateContractTx:
		amount := tx.AmountOrZero()
		if amount.Sign() > 0 && tx.Type == types.CallContractTx {
			collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
			stateDB.SubBalance(sender, amount)
			stateDB.AddBalance(*tx.To, amount)
			collector.CompleteBalanceUpdate(statsCollector, appState)
		}
		receipt = context.vm.Run(tx, chain.getGasLimit(appState, tx))
		if receipt.Error != nil {
			chain.log.Error("contract err", "err", receipt.Error)
		}
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)

		if !receipt.Success && tx.Type == types.CallContractTx {
			stateDB.AddBalance(sender, amount)
			stateDB.SubBalance(*tx.To, amount)
		}
		if receipt.Success && tx.Type == types.DeployContractTx {
			stateDB.SubBalance(sender, amount)
		}
		receipt.GasCost = chain.GetGasCost(appState, receipt.GasUsed)
		fee = fee.Add(fee, receipt.GasCost)
		collector.AddTxReceipt(statsCollector, receipt, appState)
	case types.DelegateTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		stateDB.ToggleDelegationAddress(sender, *tx.To)
	case types.UndelegateTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		stateDB.ToggleDelegationAddress(sender, common.EmptyAddress)
	case types.StoreToIpfsTx:
		collector.BeginTxBalanceUpdate(statsCollector, tx, appState)
		defer collector.CompleteBalanceUpdate(statsCollector, appState)
		task = func() {
			if rand.Float32() > StoreToIpfsThreshold {
				attachment := attachments.ParseStoreToIpfsAttachment(tx)
				select {
				case chain.ipfsLoadQueue <- attachment:
				default:
				}
			}
		}
	}

	stateDB.SubBalance(sender, fee)
	stateDB.SubBalance(sender, tx.TipsOrZero())
	stateDB.SetNonce(sender, tx.AccountNonce)

	if senderAccount.Epoch() != tx.Epoch {
		stateDB.SetEpoch(sender, tx.Epoch)
	}
	collector.AddTxFee(statsCollector, fee)
	collector.AddFeeBurntCoins(statsCollector, sender, fee, chain.config.Consensus.FeeBurnRate, tx)

	return fee, receipt, task, nil
}

func (chain *Blockchain) getTxFee(feePerGas *big.Int, tx *types.Transaction) *big.Int {
	return fee.CalculateFee(chain.appState.ValidatorsCache.NetworkSize(), feePerGas, tx)
}

func (chain *Blockchain) GetGasCost(appState *appstate.AppState, gasUsed uint64) *big.Int {
	feePerGas := appState.State.FeePerGas()
	if common.ZeroOrNil(feePerGas) {
		return common.Big0
	}
	return new(big.Int).Mul(feePerGas, big.NewInt(int64(gasUsed)))
}

func (chain *Blockchain) getGasLimit(appState *appstate.AppState, tx *types.Transaction) int64 {
	feePerGas := appState.State.FeePerGas()
	txFee := chain.getTxFee(feePerGas, tx)
	diff := new(big.Int).Sub(tx.MaxFeeOrZero(), txFee)

	oneGasCost := chain.GetGasCost(appState, 1)
	if oneGasCost.Sign() == 0 {
		return 0
	}
	return math.ToInt(decimal.NewFromBigInt(diff, 0).Div(decimal.NewFromBigInt(oneGasCost, 0))).Int64()
}

func (chain *Blockchain) applyNextBlockFee(appState *appstate.AppState, block *types.Block, usedGas uint64) {
	feePerGas := chain.calculateNextBlockFeePerGas(appState, block, usedGas)
	appState.State.SetFeePerGas(feePerGas)
}

func (chain *Blockchain) calculateNextBlockFeePerGas(appState *appstate.AppState, block *types.Block, usedGas uint64) *big.Int {

	minFeePerGas := fee.GetFeePerGasForNetwork(appState.ValidatorsCache.NetworkSize())

	feePerGas := appState.State.FeePerGas()
	if common.ZeroOrNil(feePerGas) || feePerGas.Cmp(minFeePerGas) == -1 {
		feePerGas = new(big.Int).Set(minFeePerGas)
	}

	k := chain.config.Consensus.FeeSensitivityCoef
	maxBlockGas := types.MaxBlockGas

	// curBlockFee = prevBlockFee * (1 + k * (prevBlockGas / maxBlockGas - 0.5))
	newFeePerGasD := decimal.New(int64(usedGas), 0).
		Div(decimal.New(int64(maxBlockGas), 0)).
		Sub(decimal.NewFromFloat(0.5)).
		Mul(decimal.NewFromFloat32(k)).
		Add(decimal.New(1, 0)).
		Mul(decimal.NewFromBigInt(feePerGas, 0))

	newFeePerGas := math.ToInt(newFeePerGasD)
	if newFeePerGas.Cmp(minFeePerGas) == -1 {
		newFeePerGas = new(big.Int).Set(minFeePerGas)
	}
	return newFeePerGas
}

func (chain *Blockchain) applyVrfProposerThreshold(appState *appstate.AppState, block *types.Block) {
	appState.State.AddBlockBit(block.IsEmpty())
	currentThreshold := appState.State.VrfProposerThreshold()

	online := float64(appState.ValidatorsCache.ValidatorsSize())
	if online == 0 {
		online = 1
	}

	emptyBlocks := appState.State.EmptyBlocksCount()

	minVrf := math2.Max(chain.config.Consensus.MinProposerThreshold, 1.0-5.0/online)
	maxVrf := math2.Max(chain.config.Consensus.MinProposerThreshold, 1.0-1.0/online)

	step := (maxVrf - minVrf) / 60
	switch emptyBlocks {
	case 0:
		currentThreshold += step
	case 1, 2:
	default:
		currentThreshold -= step
	}

	currentThreshold = math2.Max(minVrf, math2.Min(currentThreshold, maxVrf))

	appState.State.SetVrfProposerThreshold(currentThreshold)
}

func (chain *Blockchain) applyStatusSwitch(appState *appstate.AppState, block *types.Block) {
	if !block.Header.Flags().HasFlag(types.IdentityUpdate) {
		return
	}
	addrs := appState.State.StatusSwitchAddresses()
	for _, addr := range addrs {
		isOnline := appState.IdentityState.IsOnline(addr)
		if isOnline {
			appState.IdentityState.SetOnline(addr, false)
			continue
		}
		if appState.IdentityState.IsApproved(addr) || appState.ValidatorsCache.IsPool(addr) {
			appState.IdentityState.SetOnline(addr, true)
		}
	}
	appState.State.ClearStatusSwitchAddresses()
}

func (chain *Blockchain) applyDelegationSwitch(appState *appstate.AppState, block *types.Block) (undelegations []*state.Delegation) {
	if !block.Header.Flags().HasFlag(types.IdentityUpdate) {
		return
	}
	delegations := appState.State.Delegations()

	newPools := map[common.Address]struct{}{}

	for idx := range delegations {
		delegation := delegations[idx]
		if delegation.Delegatee.IsEmpty() {
			delegatee := appState.State.Delegatee(delegation.Delegator)
			appState.IdentityState.RemoveDelegatee(delegation.Delegator)
			appState.State.RemoveDelegatee(delegation.Delegator)
			if delegatee != nil {
				undelegations = append(undelegations, &state.Delegation{Delegator: delegation.Delegator, Delegatee: *delegatee})
			}
		} else {
			_, becamePool := newPools[delegation.Delegator]
			if appState.State.Delegatee(delegation.Delegatee) == nil && !becamePool && !appState.ValidatorsCache.IsPool(delegation.Delegator) {
				appState.IdentityState.SetDelegatee(delegation.Delegator, delegation.Delegatee)
				appState.State.SetDelegatee(delegation.Delegator, delegation.Delegatee)
				appState.State.SetDelegationEpoch(delegation.Delegator, appState.State.Epoch())
				appState.IdentityState.SetOnline(delegation.Delegator, false)
				newPools[delegation.Delegatee] = struct{}{}
			}
		}
	}
	appState.State.ClearDelegations()
	return undelegations
}

func (chain *Blockchain) switchPoolsToOffline(appState *appstate.AppState, undelegations []*state.Delegation, block *types.Block) {
	if !block.Header.Flags().HasFlag(types.IdentityUpdate) {
		return
	}
	lostPoolNodes := map[common.Address][]common.Address{}

	addToLostPoolNode := func(pool, node common.Address) {
		list := lostPoolNodes[pool]
		lostPoolNodes[pool] = append(list, node)
	}

	for _, addr := range appState.State.CollectKilledDelegators() {
		identity := appState.State.GetIdentity(addr)
		addToLostPoolNode(*identity.Delegatee, addr)
	}

	for _, delegation := range undelegations {
		addToLostPoolNode(delegation.Delegatee, delegation.Delegator)
	}
	for pool, nodes := range lostPoolNodes {
		switchOnePoolToOffline(appState, pool, nodes)
	}
}

func (chain *Blockchain) getTxCost(feePerGas *big.Int, tx *types.Transaction) *big.Int {
	return fee.CalculateCost(chain.appState.ValidatorsCache.NetworkSize(), feePerGas, tx)
}

func getSeedData(prevBlock *types.Header) []byte {
	result := prevBlock.Seed().Bytes()
	result = append(result, common.ToBytes(prevBlock.Height()+1)...)
	return result
}

func (chain *Blockchain) GetProposerSortition() (bool, []byte) {

	if checkIfProposer(chain.coinBaseAddress, chain.appState) {

		modifier := int64(1)
		if chain.appState.ValidatorsCache.IsPool(chain.coinBaseAddress) {
			modifier = int64(chain.appState.ValidatorsCache.PoolSize(chain.coinBaseAddress))
		}
		return chain.getSortition(chain.getProposerData(), chain.appState.State.VrfProposerThreshold(), modifier)
	}

	return false, nil
}

func (chain *Blockchain) ProposeBlock(proof []byte) *types.BlockProposal {
	head := chain.Head

	txs := chain.txpool.BuildBlockTransactions()
	checkState, _ := chain.appState.ForCheck(chain.Head.Height())

	prevBlockTime := time.Unix(chain.Head.Time(), 0)
	newBlockTime := prevBlockTime.Add(MinBlockDelay).Unix()
	if localTime := time.Now().UTC().Unix(); localTime > newBlockTime {
		newBlockTime = localTime
	}

	header := &types.ProposedHeader{
		Height:         head.Height() + 1,
		ParentHash:     head.Hash(),
		Time:           newBlockTime,
		ProposerPubKey: chain.pubKey,
		FeePerGas:      chain.appState.State.FeePerGas(),
	}

	header.BlockSeed, header.SeedProof = chain.secStore.VrfEvaluate(getSeedData(head))
	addr, flag := chain.offlineDetector.ProposeOffline(head)
	if addr != nil {
		header.OfflineAddr = addr
		header.Flags |= flag
	}

	filteredTxs, totalFee, totalTips, receipts, usedGas := chain.filterTxs(checkState, txs, header)
	body := &types.Body{
		Transactions: filteredTxs,
	}
	var bodyCid cid2.Cid
	var bodyCidBytes []byte

	bodyCid, _ = chain.ipfs.Cid(body.ToBytes())
	if bodyCid != ipfs.EmptyCid {
		bodyCidBytes = bodyCid.Bytes()
	}

	receiptsBytes, _ := receipts.ToBytes()

	var receiptsCid cid2.Cid
	var receiptsCidBytes []byte
	receiptsCid, _ = chain.ipfs.Cid(receiptsBytes)

	if receiptsCid != ipfs.EmptyCid {
		receiptsCidBytes = receiptsCid.Bytes()
	}

	header.IpfsHash = bodyCidBytes
	header.TxHash = types.DeriveSha(types.Transactions(filteredTxs))
	header.TxReceiptsCid = receiptsCidBytes

	block := &types.Block{
		Header: &types.Header{
			ProposedHeader: header,
		},
		Body: body,
	}

	block.Header.ProposedHeader.TxBloom = calculateTxBloom(block, receipts)

	block.Header.ProposedHeader.Flags |= chain.calculateFlags(checkState, block, head)

	if chain.upgrader.CanUpgrade() && !block.Header.ProposedHeader.Flags.HasFlag(types.NewGenesis) {
		header.Upgrade = chain.upgrader.UpgradeBits()
	}

	block.Header.ProposedHeader.Root, block.Header.ProposedHeader.IdentityRoot, _, _ = chain.applyBlockOnState(checkState, block, chain.Head, totalFee, totalTips, usedGas, nil)

	proposal := &types.BlockProposal{Block: block, Proof: proof}
	hash := crypto.SignatureHash(proposal)
	proposal.Signature = chain.secStore.Sign(hash[:])
	return proposal
}

func calculateTxBloom(block *types.Block, receipts types.TxReceipts) []byte {
	if block.IsEmpty() {
		return []byte{}
	}
	if len(block.Body.Transactions) == 0 {
		return []byte{}
	}

	values := make(map[string]struct{})

	for _, tx := range block.Body.Transactions {
		sender, _ := types.Sender(tx)
		values[string(sender.Bytes())] = struct{}{}
		if tx.To != nil {
			values[string(tx.To.Bytes())] = struct{}{}
		}
	}

	for _, r := range receipts {
		for _, e := range r.Events {
			values[string(append(r.ContractAddress.Bytes(), []byte(e.EventName)...))] = struct{}{}
		}
	}

	bloom := common.NewSerializableBF(len(values))
	for addr := range values {
		bloom.Add([]byte(addr))
	}
	data, _ := bloom.Serialize()
	return data
}

func (chain *Blockchain) calculateFlags(appState *appstate.AppState, block *types.Block, prevBlock *types.Header) types.BlockFlag {

	var flags types.BlockFlag

	for _, tx := range block.Body.Transactions {
		if tx.Type == types.KillTx || tx.Type == types.KillInviteeTx || tx.Type == types.KillDelegatorTx {
			flags |= types.IdentityUpdate
		}
	}
	stateDb := appState.State
	if stateDb.ValidationPeriod() == state.NonePeriod &&
		chain.timing.isFlipLotteryStarted(stateDb.NextValidationTime(), block.Header.Time()) {
		flags |= types.FlipLotteryStarted
	}

	if stateDb.ValidationPeriod() == state.FlipLotteryPeriod &&
		chain.timing.isShortSessionStarted(stateDb.NextValidationTime(), block.Header.Time()) {
		flags |= types.ShortSessionStarted
	}

	if stateDb.ValidationPeriod() == state.ShortSessionPeriod &&
		chain.timing.isLongSessionStarted(stateDb.NextValidationTime(), block.Header.Time()) {
		flags |= types.LongSessionStarted
	}

	if stateDb.ValidationPeriod() == state.LongSessionPeriod &&
		chain.timing.isAfterLongSessionStarted(stateDb.NextValidationTime(), block.Header.Time(), appState.ValidatorsCache.NetworkSize()) {
		flags |= types.AfterLongSessionStarted
	}

	if stateDb.ValidationPeriod() == state.AfterLongSessionPeriod && stateDb.CanCompleteEpoch() {
		flags |= types.ValidationFinished
		flags |= types.IdentityUpdate
	}

	if block.Height()-appState.State.LastSnapshot() >= chain.config.Consensus.SnapshotRange && appState.State.ValidationPeriod() == state.NonePeriod &&
		!flags.HasFlag(types.ValidationFinished) && !flags.HasFlag(types.FlipLotteryStarted) {
		flags |= types.Snapshot
	}

	if (flags.HasFlag(types.Snapshot) || block.Height()%chain.config.Consensus.StatusSwitchRange == 0) && (len(appState.State.StatusSwitchAddresses()) > 0 ||
		len(appState.State.DelayedOfflinePenalties()) > 0) {
		flags |= types.IdentityUpdate
	}

	if block.Height()%chain.config.Consensus.DelegationSwitchRange == 0 && len(appState.State.Delegations()) > 0 {
		flags |= types.IdentityUpdate
	}

	if prevBlock.ProposedHeader != nil && prevBlock.ProposedHeader.Upgrade > 0 && chain.config.Consensus.GenerateGenesisAfterUpgrade {
		flags |= types.NewGenesis
	}
	return flags
}

func (chain *Blockchain) filterTxs(appState *appstate.AppState, txs []*types.Transaction, header *types.ProposedHeader) ([]*types.Transaction, *big.Int, *big.Int, types.TxReceipts, uint64) {
	var result []*types.Transaction

	minFeePerGas := fee.GetFeePerGasForNetwork(appState.ValidatorsCache.NetworkSize())

	totalFee := new(big.Int)
	totalTips := new(big.Int)
	vm := vm.NewVmImpl(appState, &types.Header{ProposedHeader: header}, chain.secStore, nil, chain.config)
	var receipts []*types.TxReceipt
	var usedGas uint64
	for _, tx := range txs {
		if err := validation.ValidateTx(appState, tx, minFeePerGas, validation.InBlockTx); err != nil {
			continue
		}
		context := &txExecutionContext{appState: appState, vm: vm, height: header.Height}
		if f, r, _, err := chain.applyTxOnState(tx, context); err == nil {
			gas := uint64(fee.CalculateGas(tx))
			if r != nil {
				receipts = append(receipts, r)
				gas += r.GasUsed
			}
			if usedGas+gas > types.MaxBlockGas {
				break
			}
			usedGas += gas
			totalFee.Add(totalFee, f)
			totalTips.Add(totalTips, tx.TipsOrZero())
			result = append(result, tx)
		}
	}
	return result, totalFee, totalTips, receipts, usedGas
}

func (chain *Blockchain) insertHeader(header *types.Header) {
	chain.repo.WriteBlockHeader(header)
	chain.repo.WriteHead(nil, header)
	chain.repo.WriteCanonicalHash(header.Height(), header.Hash())
}

func (chain *Blockchain) insertBlock(block *types.Block, diff *state.IdentityStateDiff, receipts types.TxReceipts) error {
	_, err := chain.ipfs.Add(block.Body.ToBytes(), chain.ipfs.ShouldPin(ipfs.Block))
	if err != nil {
		return errors.Wrap(BlockInsertionErr, err.Error())
	}
	if receipts != nil {
		data, _ := receipts.ToBytes()
		_, err := chain.ipfs.Add(data, chain.ipfs.ShouldPin(ipfs.TxReceipt))
		if err != nil {
			return errors.Wrap(BlockInsertionErr, err.Error())
		}
	}

	chain.insertHeader(block.Header)
	chain.WriteIdentityStateDiff(block.Height(), diff)
	chain.WriteTxIndex(block.Hash(), block.Body.Transactions)
	if receipts != nil {
		chain.WriteTxReceipts(block.Header.ProposedHeader.TxReceiptsCid, receipts)
	}
	chain.indexer.HandleBlockTransactions(block.Header, block.Body.Transactions)
	chain.setCurrentHead(block.Header)
	return nil
}

func (chain *Blockchain) WriteTxIndex(hash common.Hash, txs types.Transactions) {
	for i, tx := range txs {
		idx := &types.TransactionIndex{
			BlockHash: hash,
			Idx:       uint32(i),
		}
		chain.repo.WriteTxIndex(tx.Hash(), idx)
	}
}

func (chain *Blockchain) WriteTxReceipts(cid []byte, receipts types.TxReceipts) {
	m := make(map[common.Address]map[string]struct{})
	for _, s := range chain.subManager.Subscriptions() {
		eventMap, ok := m[s.Contract]
		if !ok {
			eventMap = make(map[string]struct{})
			m[s.Contract] = eventMap
		}
		eventMap[s.Event] = struct{}{}
	}

	for i, r := range receipts {
		idx := &types.TxReceiptIndex{
			Idx:        uint32(i),
			ReceiptCid: cid,
		}
		chain.repo.WriteReceiptIndex(r.TxHash, idx)
		if eventMap, ok := m[r.ContractAddress]; ok {
			for idx, event := range r.Events {
				if _, ok := eventMap[event.EventName]; ok {
					chain.repo.WriteEvent(r.ContractAddress, r.TxHash, uint32(idx), event)
				}
			}
		}
	}
}

func (chain *Blockchain) getProposerData() []byte {
	head := chain.Head
	result := head.Seed().Bytes()
	result = append(result, common.ToBytes(ProposerRole)...)
	result = append(result, common.ToBytes(head.Height()+1)...)
	return result
}

func (chain *Blockchain) getSortition(data []byte, threshold float64, modifier int64) (bool, []byte) {

	hash, proof := chain.secStore.VrfEvaluate(data)
	q := common.HashToFloat(hash, modifier)

	vrfThreshold := new(big.Float).SetFloat64(threshold)

	if q.Cmp(vrfThreshold) >= 0 {
		return true, proof
	}
	return false, nil
}

func (chain *Blockchain) validateBlock(checkState *appstate.AppState, block *types.Block, prevBlock *types.Header, statsCollector collector.StatsCollector) (*blockInsertionResult, error) {

	if block.IsEmpty() {
		emptyBlock, blockInsertionRes := chain.generateEmptyBlock(checkState, prevBlock, statsCollector)
		if emptyBlock.Hash() == block.Hash() {
			return blockInsertionRes, nil
		}
		return nil, errors.New("empty blocks' hashes mismatch")
	}

	if err := chain.ValidateHeader(block.Header, prevBlock); err != nil {
		return nil, err
	}

	if !common.ZeroOrNil(block.Header.ProposedHeader.FeePerGas) && checkState.State.FeePerGas().Cmp(block.Header.ProposedHeader.FeePerGas) != 0 {
		return nil, errors.New("fee rate is invalid")
	}

	proposerAddr, _ := crypto.PubKeyBytesToAddress(block.Header.ProposedHeader.ProposerPubKey)

	if !checkIfProposer(proposerAddr, checkState) {
		return nil, errors.New("proposer is not identity")
	}

	var txs = types.Transactions(block.Body.Transactions)

	if types.DeriveSha(txs) != block.Header.ProposedHeader.TxHash {
		return nil, errors.New("txHash is invalid")
	}

	var totalFee, totalTips *big.Int
	var err error
	var receipts types.TxReceipts
	var usedGas uint64
	txsContext := &txsExecutionContext{
		appState:       checkState,
		header:         block.Header,
		statsCollector: statsCollector,
	}
	var tasks []task

	if totalFee, totalTips, receipts, tasks, usedGas, err = chain.processTxs(block.Body.Transactions, txsContext); err != nil {
		return nil, err
	}

	if bytes.Compare(calculateTxBloom(block, receipts), block.Header.ProposedHeader.TxBloom) != 0 {
		return nil, errors.New("tx bloom is invalid")
	}

	persistentFlags := block.Header.ProposedHeader.Flags.UnsetFlag(types.OfflinePropose).UnsetFlag(types.OfflineCommit)

	if expected := chain.calculateFlags(checkState, block, prevBlock); expected != persistentFlags {
		return nil, errors.Errorf("flags are invalid, expected=%v, actual=%v", expected, persistentFlags)
	}

	var root, identityRoot common.Hash
	var stateDiff []*state.StateTreeDiff
	var identityStateDiff *state.IdentityStateDiff
	if root, identityRoot, stateDiff, identityStateDiff = chain.applyBlockOnState(checkState, block, prevBlock, totalFee, totalTips, usedGas, statsCollector); root != block.Root() || identityRoot != block.IdentityRoot() {
		return nil, errors.Errorf("invalid block roots. Expected=%x & %x, actual=%x & %x", root, identityRoot, block.Root(), block.IdentityRoot())
	}

	cid, _ := chain.ipfs.Cid(block.Body.ToBytes())

	var cidBytes []byte
	if cid != ipfs.EmptyCid {
		cidBytes = cid.Bytes()
	}
	if bytes.Compare(cidBytes, block.Header.ProposedHeader.IpfsHash) != 0 {
		return nil, errors.New("invalid block cid")
	}
	var receiptCidBytes []byte
	if receipts != nil {
		data, _ := receipts.ToBytes()
		cidReceipt, _ := chain.ipfs.Cid(data)
		if cidReceipt != ipfs.EmptyCid {
			receiptCidBytes = cidReceipt.Bytes()
		}
	}
	if bytes.Compare(receiptCidBytes, block.Header.ProposedHeader.TxReceiptsCid) != 0 {
		return nil, errors.New("invalid receipt cid")
	}
	return &blockInsertionResult{stateDiff: stateDiff, identityStateDiff: identityStateDiff, txReceipts: receipts, txTasks: tasks}, nil
}

func (chain *Blockchain) ValidateBlockCertOnHead(block *types.Header, cert *types.BlockCert) error {
	return chain.ValidateBlockCert(chain.Head, block, cert, chain.appState.ValidatorsCache, nil)
}

func (chain *Blockchain) ValidateBlockCert(prevBlock *types.Header, block *types.Header, cert *types.BlockCert, validatorsCache *validators.ValidatorsCache, pubKeyToAddrCache map[string]common.Address) (err error) {

	step := cert.Step
	validators := validatorsCache.GetOnlineValidators(prevBlock.Seed(), block.Height(), step, chain.GetCommitteeSize(validatorsCache, step == types.Final))

	voters := mapset.NewSet()

	for _, signature := range cert.Signatures {

		vote := types.Vote{
			Header: &types.VoteHeader{
				Step:        step,
				Round:       cert.Round,
				TurnOffline: signature.TurnOffline,
				Upgrade:     signature.Upgrade,
				VotedHash:   cert.VotedHash,
				ParentHash:  prevBlock.Hash(),
			},
			Signature: signature.Signature,
		}

		var addr common.Address
		if pubKeyToAddrCache != nil {
			pubKey, err := vote.PubKey()
			if err != nil {
				continue
			}
			var ok bool
			if addr, ok = pubKeyToAddrCache[string(pubKey)]; !ok {
				addr = vote.VoterAddr()
				pubKeyToAddrCache[string(pubKey)] = addr
			}
		} else {
			addr = vote.VoterAddr()
		}

		if !validators.Contains(addr) {
			return errors.Errorf("invalid voter %v", addr.String())
		}
		if vote.Header.Round != block.Height() {
			return errors.New("invalid vote header")
		}
		if vote.Header.VotedHash != block.Hash() {
			return errors.New("invalid voted hash")
		}

		if vote.Header.ParentHash != prevBlock.Hash() {
			return errors.New("invalid parent hash")
		}
		voters.Add(addr)
	}

	if voters.Cardinality() < chain.GetCommitteeVotesThreshold(validatorsCache, step == types.Final)-validators.VotesCountSubtrahend(chain.config.Consensus.AgreementThreshold) {
		return errors.New("not enough votes")
	}
	return nil
}

func (chain *Blockchain) ValidateBlock(block *types.Block, checkState *appstate.AppState, statsCollector collector.StatsCollector) (*blockInsertionResult, error) {
	if checkState == nil {
		var err error
		checkState, err = chain.appState.ForCheck(chain.Head.Height())
		if err != nil {
			return nil, err
		}
	}
	return chain.validateBlock(checkState, block, chain.Head, statsCollector)
}

func validateBlockParentHash(block *types.Header, prevBlock *types.Header) error {
	if prevBlock.Height()+1 != (block.Height()) {
		return errors.New(fmt.Sprintf("Height is invalid. Expected=%v but received=%v", prevBlock.Height()+1, block.Height()))
	}
	if prevBlock.Hash() != block.ParentHash() {
		return ParentHashIsInvalid
	}
	return nil
}

func validateBlockTimestamp(block *types.Header, prevBlock *types.Header) error {
	blockTime := time.Unix(block.Time(), 0)

	if blockTime.Sub(time.Now().UTC()) > MaxFutureBlockOffset {
		return errors.New("block from future")
	}
	prevBlockTime := time.Unix(prevBlock.Time(), 0)

	if blockTime.Sub(prevBlockTime) < MinBlockDelay {
		return errors.Errorf("block is too close to previous one, prev: %v, current: %v", prevBlockTime.Unix(), blockTime.Unix())
	}

	return nil
}

func (chain *Blockchain) ValidateProposerProof(proof []byte, pubKeyData []byte) error {
	pubKey, err := crypto.UnmarshalPubkey(pubKeyData)
	if err != nil {
		return err
	}
	verifier, err := p256.NewVRFVerifier(pubKey)
	if err != nil {
		return err
	}

	h, err := verifier.ProofToHash(chain.getProposerData(), proof)

	vrfThreshold := new(big.Float).SetFloat64(chain.appState.State.VrfProposerThreshold())
	proposerAddr := crypto.PubkeyToAddress(*pubKey)
	modifier := 1
	if chain.appState.ValidatorsCache.IsPool(proposerAddr) {
		modifier = chain.appState.ValidatorsCache.PoolSize(proposerAddr)
	}

	q := common.HashToFloat(h, int64(modifier))

	if q.Cmp(vrfThreshold) == -1 {
		return errors.New("Proposer is invalid")
	}

	if !checkIfProposer(proposerAddr, chain.appState) {
		return errors.New("Proposer is not identity")
	}
	return nil
}

func (chain *Blockchain) Round() uint64 {
	head := chain.Head
	return head.Height() + 1
}
func (chain *Blockchain) WriteFinalConsensus(hash common.Hash) {
	chain.repo.WriteFinalConsensus(hash)
}

func (chain *Blockchain) WriteCertificate(hash common.Hash, cert *types.BlockCert, persistent bool) {
	chain.repo.WriteCertificate(hash, cert)
	if !persistent {
		chain.repo.WriteWeakCertificate(hash)
	}
}

func (chain *Blockchain) GetBlock(hash common.Hash) *types.Block {
	header := chain.repo.ReadBlockHeader(hash)
	if header == nil {
		return nil
	}
	if header.EmptyBlockHeader != nil {
		return &types.Block{
			Header: header,
			Body:   &types.Body{},
		}
	}
	if bodyBytes, err := chain.ipfs.Get(header.ProposedHeader.IpfsHash, ipfs.Block); err != nil {
		return nil
	} else {
		body := &types.Body{}
		body.FromBytes(bodyBytes)
		return &types.Block{
			Header: header,
			Body:   body,
		}
	}
}

func (chain *Blockchain) GetBlockWithRetry(hash common.Hash) *types.Block {
	tryCount := 0
	for {
		if block := chain.GetBlock(hash); block != nil {
			return block
		}
		tryCount++
		if tryCount == 10 {
			panic(fmt.Sprintf("Failed to get block %s", hash.Hex()))
		}
		time.Sleep(time.Second)
		chain.log.Warn("Retrying to get block", "hash", hash.Hex())
	}
}

func (chain *Blockchain) GetBlockByHeight(height uint64) *types.Block {
	hash := chain.repo.ReadCanonicalHash(height)
	if hash == (common.Hash{}) {
		return nil
	}
	return chain.GetBlock(hash)
}

func (chain *Blockchain) GetBlockHeaderByHeight(height uint64) *types.Header {
	hash := chain.repo.ReadCanonicalHash(height)
	if hash == (common.Hash{}) {
		return nil
	}
	return chain.repo.ReadBlockHeader(hash)
}

func (chain *Blockchain) GetTxIndex(hash common.Hash) *types.TransactionIndex {
	return chain.repo.ReadTxIndex(hash)
}

func (chain *Blockchain) GetReceipt(hash common.Hash) *types.TxReceipt {
	idx := chain.repo.ReadReceiptIndex(hash)
	if idx == nil {
		return nil
	}
	data, err := chain.ipfs.Get(idx.ReceiptCid, ipfs.TxReceipt)
	if err != nil {
		return nil
	}
	r := types.TxReceipts{}
	r = r.FromBytes(data)
	if len(r) < int(idx.Idx) {
		return nil
	}
	return r[idx.Idx]
}

func (chain *Blockchain) GetTx(hash common.Hash) (*types.Transaction, *types.TransactionIndex) {
	idx := chain.repo.ReadTxIndex(hash)
	if idx == nil {
		return nil, nil
	}
	header := chain.repo.ReadBlockHeader(idx.BlockHash)
	if header == nil || header.ProposedHeader == nil {
		return nil, nil
	}

	data, err := chain.ipfs.Get(header.ProposedHeader.IpfsHash, ipfs.Block)
	if err != nil {
		return nil, nil
	}
	body := &types.Body{}
	body.FromBytes(data)

	if uint32(len(body.Transactions)) < idx.Idx {
		return nil, nil
	}
	tx := body.Transactions[idx.Idx]

	if tx.Hash() != hash {
		return nil, nil
	}
	return tx, idx
}

func (chain *Blockchain) GetCommitteeSize(vc *validators.ValidatorsCache, final bool) int {
	var cnt = vc.OnlineSize()
	percent := chain.config.Consensus.CommitteePercent
	if final {
		percent = chain.config.Consensus.FinalCommitteePercent
	}
	if cnt <= 8 {
		return cnt
	}

	size := int(math2.Round(float64(cnt) * percent))
	if size > chain.config.Consensus.MaxCommitteeSize {
		return chain.config.Consensus.MaxCommitteeSize
	}
	return size
}

func (chain *Blockchain) GetCommitteeVotesThreshold(vc *validators.ValidatorsCache, final bool) int {

	var cnt = vc.OnlineSize()
	switch cnt {
	case 0, 1:
		return 1
	case 2, 3:
		return 2
	case 4, 5:
		return 3
	case 6, 7:
		return 4
	case 8:
		return 5
	}
	size := chain.GetCommitteeSize(vc, final)
	return int(math2.Round(float64(size) * chain.config.Consensus.AgreementThreshold))
}

func (chain *Blockchain) GenesisInfo() *types.GenesisInfo {
	return chain.genesisInfo
}

func (chain *Blockchain) ValidateSubChain(startHeight uint64, blocks []types.BlockBundle) error {
	checkState, err := chain.appState.ForCheckWithOverwrite(startHeight)
	if err != nil {
		return err
	}
	prevBlock := chain.GetBlockHeaderByHeight(startHeight)

	for _, b := range blocks {
		if _, err := chain.validateBlock(checkState, b.Block, prevBlock, nil); err != nil {
			return err
		}
		if b.Block.Header.Flags().HasFlag(types.IdentityUpdate) {
			if b.Cert.Empty() {
				return errors.New("Block cert is missing")
			}
		}
		if !b.Cert.Empty() {
			if err := chain.ValidateBlockCert(prevBlock, b.Block.Header, b.Cert, checkState.ValidatorsCache, nil); err != nil {
				return err
			}
		}
		if err := checkState.Commit(b.Block); err != nil {
			return err
		}
		prevBlock = b.Block.Header
	}

	if blocks[len(blocks)-1].Cert == nil {
		return errors.New("last block of the fork should have a certificate")
	}

	return nil
}

func (chain *Blockchain) ResetTo(height uint64) (revertedTxs []*types.Transaction, err error) {
	prevHead := chain.Head.Height()
	if err := chain.appState.ResetTo(height); err != nil {
		return nil, errors.WithMessage(err, "state is corrupted, try to resync from scratch")
	}
	chain.setHead(height, nil)

	for h := height + 1; h <= prevHead; h++ {
		hash := chain.repo.ReadCanonicalHash(h)
		if hash == (common.Hash{}) {
			continue
		}
		block := chain.GetBlock(hash)
		if block != nil {
			for _, tx := range block.Body.Transactions {
				revertedTxs = append(revertedTxs, tx)
			}
		}
		chain.repo.RemoveHeader(hash)
		chain.repo.RemoveCanonicalHash(h)
	}
	chain.bus.Publish(&events.BlockchainResetEvent{Header: chain.Head, RevertedTxs: revertedTxs})
	return revertedTxs, nil
}

func (chain *Blockchain) EnsureIntegrity() error {
	wasReset := false
	for chain.Head.Root() != chain.appState.State.Root() ||
		chain.Head.IdentityRoot() != chain.appState.IdentityState.Root() {
		wasReset = true
		resetTo := uint64(0)
		for h, tryCnt := chain.Head.Height()-1, 0; h >= 1 && tryCnt < state.MaxSavedStatesCount+1; h, tryCnt = h-1, tryCnt+1 {
			if chain.appState.State.HasVersion(h) && chain.appState.IdentityState.HasVersion(h) {
				resetTo = h
				break
			}
		}
		if resetTo == 0 {
			return errors.New("state db is corrupted, try to delete idenachain.db folder from your data directory and sync from scratch")
		}
		if _, err := chain.ResetTo(resetTo); err != nil {
			return err
		}
	}
	if wasReset {
		chain.log.Warn("Blockchain was reset", "new head", chain.Head.Height())
	}
	return nil
}

func (chain *Blockchain) StartSync() {
	chain.isSyncing = true
	chain.txpool.StartSync()
}

func (chain *Blockchain) StopSync() {
	chain.isSyncing = false
	chain.txpool.StopSync(chain.GetBlockWithRetry(chain.Head.Hash()))
}

func checkIfProposer(addr common.Address, appState *appstate.AppState) bool {
	return appState.ValidatorsCache.IsOnlineIdentity(addr) ||
		appState.State.GodAddress() == addr && appState.ValidatorsCache.OnlineSize() == 0
}

func (chain *Blockchain) AddHeaderUnsafe(header *types.Header) error {
	chain.repo.WriteBlockHeader(header)
	chain.repo.WriteCanonicalHash(header.Height(), header.Hash())
	chain.repo.WritePreliminaryHead(header)
	chain.PreliminaryHead = header
	return nil
}

func (chain *Blockchain) ValidateHeader(header, prevBlock *types.Header) error {
	if err := validateBlockParentHash(header, prevBlock); err != nil {
		return err
	}

	if err := validateBlockTimestamp(header, prevBlock); err != nil {
		return err
	}

	if header.EmptyBlockHeader != nil {
		//TODO: validate empty block hash
		return nil
	}

	coinbase := header.Coinbase()
	if coinbase == (common.Address{}) {
		return errors.New("invalid coinbase")
	}

	var seedData = getSeedData(prevBlock)
	pubKey, err := crypto.UnmarshalPubkey(header.ProposedHeader.ProposerPubKey)
	if err != nil {
		return err
	}
	verifier, err := p256.NewVRFVerifier(pubKey)
	if err != nil {
		return err
	}

	hash, err := verifier.ProofToHash(seedData, header.ProposedHeader.SeedProof)
	if err != nil {
		return err
	}
	if hash != header.Seed() {
		return errors.New("seed is invalid")
	}
	if header.Flags().HasFlag(types.NewGenesis) {
		if header.ProposedHeader.Upgrade != 0 || prevBlock.ProposedHeader == nil ||
			prevBlock.ProposedHeader.Upgrade == 0 ||
			!chain.config.Consensus.GenerateGenesisAfterUpgrade {
			return errors.New("flag NewGenesis is invalid")
		}
	}
	if prevBlock.ProposedHeader != nil && prevBlock.ProposedHeader.Upgrade > 0 && chain.config.Consensus.GenerateGenesisAfterUpgrade {
		if !header.Flags().HasFlag(types.NewGenesis) {
			return errors.New("flag NewGenesis is required")
		}
	}
	if header.ProposedHeader != nil && header.ProposedHeader.Upgrade > 0 && header.ProposedHeader.Upgrade != uint32(chain.upgrader.Target()) {
		return errors.New("unknown consensus upgrade")
	}
	//TODO: add proposer's check??

	return nil
}

func (chain *Blockchain) GetCertificate(hash common.Hash) *types.BlockCert {
	return chain.repo.ReadCertificate(hash)
}

func (chain *Blockchain) GetIdentityDiff(height uint64) *state.IdentityStateDiff {
	data := chain.repo.ReadIdentityStateDiff(height)
	if data == nil {
		return nil
	}
	diff := new(state.IdentityStateDiff)
	diff.FromBytes(data)
	return diff
}

func (chain *Blockchain) ReadSnapshotManifest() *snapshot.Manifest {
	cid, cidV2, root, height, _ := chain.repo.LastSnapshotManifest()
	if cid == nil {
		return nil
	}
	return &snapshot.Manifest{
		Cid:    cid,
		CidV2:  cidV2,
		Root:   root,
		Height: height,
	}
}

func (chain *Blockchain) ReadPreliminaryHead() *types.Header {
	return chain.repo.ReadPreliminaryHead()
}

func (chain *Blockchain) WriteIdentityStateDiff(height uint64, diff *state.IdentityStateDiff) {
	if !diff.Empty() {
		b, _ := diff.ToBytes()
		chain.repo.WriteIdentityStateDiff(height, b)
	}
}

func (chain *Blockchain) RemovePreliminaryHead(batch dbm.Batch) {
	if chain.PreliminaryHead != nil {
		chain.repo.RemovePreliminaryHead(batch)
	}
	chain.PreliminaryHead = nil
}

func (chain *Blockchain) IsPermanentCert(header *types.Header) bool {
	return header.Flags().HasFlag(types.IdentityUpdate|types.Snapshot|types.NewGenesis) ||
		header.Height()%chain.config.Blockchain.StoreCertRange == 0 || header.ProposedHeader != nil && header.ProposedHeader.Upgrade > 0
}

func (chain *Blockchain) ReadTxs(address common.Address, count int, token []byte) ([]*types.SavedTransaction, []byte) {
	return chain.repo.GetSavedTxs(address, count, token)
}

func (chain *Blockchain) ReadTotalBurntCoins() []*types.BurntCoins {
	return chain.repo.GetTotalBurntCoins()
}

func readPredefinedState() (*models.ProtoPredefinedState, error) {
	data, err := resources.PredefinedState()
	if err != nil {
		return nil, err
	}
	predefinedState := new(models.ProtoPredefinedState)
	if err := proto.Unmarshal(data, predefinedState); err != nil {
		return nil, err
	}
	return predefinedState, nil
}

func (chain *Blockchain) ReadBlockForForkedPeer(blocks []common.Hash) []types.BlockBundle {
	commonHeight := uint64(1)
	needBlocks := uint64(0)
	for i := 0; i < len(blocks); i++ {
		header := chain.repo.ReadBlockHeader(blocks[i])
		needBlocks++
		if header != nil {
			commonHeight = header.Height()
			break
		}
	}
	result := make([]types.BlockBundle, 0)
	if commonHeight == 1 {
		return result
	}
	for h := commonHeight + 1; h < commonHeight+needBlocks+1 && h <= chain.Head.Height(); h++ {
		block := chain.GetBlockByHeight(h)
		if block == nil {
			break
		}
		result = append(result, types.BlockBundle{
			Block: block,
			Cert:  chain.GetCertificate(block.Hash()),
		})
	}
	if len(result) == 0 {
		return result
	}
	lastBlock := result[len(result)-1]
	if lastBlock.Cert != nil {
		return result
	}
	for i := uint64(1); i <= chain.config.Blockchain.StoreCertRange; i++ {
		block := chain.GetBlockByHeight(lastBlock.Block.Height() + i)
		if block == nil {
			result = make([]types.BlockBundle, 0)
			break
		}
		cert := chain.GetCertificate(block.Hash())
		result = append(result, types.BlockBundle{
			Block: block,
			Cert:  cert,
		})
		if cert != nil {
			break
		}
	}
	return result
}

func (chain *Blockchain) GetTopBlockHashes(count int) []common.Hash {
	result := make([]common.Hash, 0, count)
	head := chain.Head.Height()
	for i := uint64(0); i < uint64(count); i++ {
		hash := chain.repo.ReadCanonicalHash(head - i)
		if hash == (common.Hash{}) {
			break
		}
		result = append(result, hash)
	}
	return result
}

func (chain *Blockchain) AtomicSwitchToPreliminary(manifest *snapshot.Manifest) error {
	batch, oldIdentityStateDb, err := chain.appState.IdentityState.SwitchToPreliminary(manifest.Height)

	if err != nil {
		chain.appState.State.DropSnapshot(manifest)
		return err
	}
	defer batch.Close()

	oldStateDb := chain.appState.State.CommitSnapshot(manifest.Height, batch)
	chain.appState.ValidatorsCache.Load()

	chain.setHead(chain.PreliminaryHead.Height(), batch)
	newHead := chain.PreliminaryHead
	chain.RemovePreliminaryHead(batch)

	consensusVersion := chain.ReadPreliminaryConsensusVersion()
	if consensusVersion > 0 {
		chain.repo.WriteConsensusVersion(batch, consensusVersion)
		chain.repo.RemovePreliminaryConsensusVersion(batch)
	}
	preliminaryIntermediateGenesis := chain.repo.ReadPreliminaryIntermediateGenesis()
	if preliminaryIntermediateGenesis > 0 {
		chain.repo.WriteIntermediateGenesis(batch, preliminaryIntermediateGenesis)
		chain.repo.RemovePreliminaryIntermediateGenesis(batch)
	}
	if err := batch.WriteSync(); err != nil {
		return err
	}
	if preliminaryIntermediateGenesis > 0 {
		hash := chain.repo.ReadCanonicalHash(preliminaryIntermediateGenesis)
		chain.genesisInfo.OldGenesis = chain.genesisInfo.Genesis
		chain.genesisInfo.Genesis = chain.repo.ReadBlockHeader(hash)
	}
	chain.setCurrentHead(newHead)
	go func() {
		common.ClearDb(oldIdentityStateDb)
		common.ClearDb(oldStateDb)
	}()
	return nil
}

func (chain *Blockchain) ReadEvents(contract common.Address) []*types.SavedEvent {
	return chain.repo.GetSavedEvents(contract)
}

func (chain *Blockchain) WritePreliminaryConsensusVersion(ver uint32) {
	chain.repo.WritePreliminaryConsensusVersion(ver)
}

func (chain *Blockchain) ReadPreliminaryConsensusVersion() uint32 {
	return chain.repo.ReadPreliminaryConsensusVersion()
}

func (chain *Blockchain) RemovePreliminaryConsensusVersion() {
	chain.repo.RemovePreliminaryConsensusVersion(nil)
}

func (chain *Blockchain) WritePreliminaryIntermediateGenesis(height uint64) {
	chain.repo.WritePreliminaryIntermediateGenesis(height)
}

func (chain *Blockchain) RemovePreliminaryIntermediateGenesis() {
	chain.repo.RemovePreliminaryIntermediateGenesis(nil)
}

func (chain *Blockchain) ipfsLoad() {
	for {
		item := <-chain.ipfsLoadQueue
		_, err := chain.ipfs.GetWithSizeLimit(item.Cid, ipfs.CustomData, int64(item.Size))
		if err == nil {
			c, _ := cid2.Cast(item.Cid)
			chain.log.Debug("content loaded to local ipfs", "item", c.String())
		} else {
			chain.log.Debug("error while loading ipfs content", "err", err)
		}
	}
}

func (chain *Blockchain) CoinbaseShard() (common.ShardId, error) {
	stateDb, err := chain.appState.Readonly(chain.Head.Height())
	if err != nil {
		return common.MultiShard, err
	}
	identity := stateDb.State.GetIdentity(chain.coinBaseAddress)
	return identity.ShiftedShardId(), nil
}

func (chain *Blockchain) ModifiedCoinbaseShard() (common.ShardId, error) {
	stateDb, err := chain.appState.Readonly(chain.Head.Height())
	if err != nil {
		return common.MultiShard, err
	}
	identity := stateDb.State.GetIdentity(chain.coinBaseAddress)
	if identity.State == state.Undefined || identity.State == state.Invite || identity.State == state.Killed {
		return common.MultiShard, nil
	}
	return identity.ShiftedShardId(), nil
}

func (chain *Blockchain) MinimalShard(appState *appstate.AppState) common.ShardId {
	if appState.State.ShardsNum() == 1 {
		return common.ShardId(1)
	}
	sizes := appState.State.ShardSizes()

	minSize := uint32(math.MaxUint32)
	var minShard common.ShardId

	for shardId := common.ShardId(1); shardId < common.ShardId(appState.State.ShardsNum()); shardId++ {
		size, ok := sizes[shardId]
		if ok && size < minSize {
			minSize = size
			minShard = shardId
		}
	}
	return minShard
}

func (chain *Blockchain) ShardsNum() uint32 {
	stateDb, err := chain.appState.Readonly(chain.Head.Height())
	if err != nil {
		return 0
	}
	return stateDb.State.ShardsNum()
}
