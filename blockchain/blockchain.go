package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	mapset "github.com/deckarep/golang-set"
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
	"github.com/idena-network/idena-go/core/validators"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/vrf/p256"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/keystore"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/stats/collector"
	cid2 "github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	dbm "github.com/tendermint/tm-db"
	math2 "math"
	"math/big"
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
)

var (
	MaxHash             *big.Float
	ParentHashIsInvalid = errors.New("parentHash is invalid")
	BlockInsertionErr   = errors.New("can't insert block")
)

type Blockchain struct {
	repo            *database.Repo
	secStore        *secstore.SecStore
	Head            *types.Header
	PreliminaryHead *types.Header
	genesis         *types.Header
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
	applyNewEpochFn func(height uint64, appState *appstate.AppState, collector collector.StatsCollector) (int, *types.ValidationAuthors, bool)
	isSyncing       bool
}

func init() {
	var max [32]byte
	for i := range max {
		max[i] = 0xFF
	}
	i := new(big.Int)
	i.SetBytes(max[:])
	MaxHash = new(big.Float).SetInt(i)
}

func NewBlockchain(config *config.Config, db dbm.DB, txpool *mempool.TxPool, appState *appstate.AppState,
	ipfs ipfs.Proxy, secStore *secstore.SecStore, bus eventbus.Bus, offlineDetector *OfflineDetector, keyStore *keystore.KeyStore) *Blockchain {
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
	}
}

func (chain *Blockchain) ProvideApplyNewEpochFunc(fn func(height uint64, appState *appstate.AppState, collector collector.StatsCollector) (int, *types.ValidationAuthors, bool)) {
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

func (chain *Blockchain) InitializeChain() error {

	chain.coinBaseAddress = chain.secStore.GetAddress()
	chain.pubKey = chain.secStore.GetPubKey()
	head := chain.GetHead()
	if head != nil {
		chain.setCurrentHead(head)
		genesisHeight := uint64(1)

		if chain.config.Network == Testnet {
			predefinedState, err := readPredefinedState()
			if err != nil {
				return err
			}
			genesisHeight = predefinedState.Block
		}

		if chain.genesis = chain.GetBlockHeaderByHeight(genesisHeight); chain.genesis == nil {
			return errors.New("genesis block is not found")
		}
	} else {
		_, err := chain.GenerateGenesis(chain.config.Network)
		if err != nil {
			return err
		}
	}
	chain.indexer.initialize(chain.coinBaseAddress)
	chain.PreliminaryHead = chain.repo.ReadPreliminaryHead()
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

func (chain *Blockchain) GenerateGenesis(network types.Network) (*types.Block, error) {

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
	var feePerByte *big.Int

	if network == Testnet {
		predefinedState, err := readPredefinedState()
		if err != nil {
			return nil, err
		}

		blockNumber = predefinedState.Block
		seed = predefinedState.Seed
		feePerByte = predefinedState.Global.FeePerByte

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

	block := &types.Block{Header: &types.Header{
		ProposedHeader: &types.ProposedHeader{
			ParentHash:   emptyHash,
			Time:         big.NewInt(0),
			Height:       blockNumber,
			Root:         chain.appState.State.Root(),
			IdentityRoot: chain.appState.IdentityState.Root(),
			BlockSeed:    seed,
			IpfsHash:     ipfs.EmptyCid.Bytes(),
			FeePerByte:   feePerByte,
		},
	}, Body: &types.Body{}}

	if err := chain.insertBlock(block, new(state.IdentityStateDiff)); err != nil {
		return nil, err
	}
	chain.genesis = block.Header
	return block, nil
}

func (chain *Blockchain) generateEmptyBlock(checkState *appstate.AppState, prevBlock *types.Header) *types.Block {
	prevTimestamp := time.Unix(prevBlock.Time().Int64(), 0)

	block := &types.Block{
		Header: &types.Header{
			EmptyBlockHeader: &types.EmptyBlockHeader{
				ParentHash: prevBlock.Hash(),
				Height:     prevBlock.Height() + 1,
				Time:       new(big.Int).SetInt64(prevTimestamp.Add(EmptyBlockTimeIncrement).Unix()),
			},
		},
		Body: &types.Body{},
	}

	block.Header.EmptyBlockHeader.BlockSeed = types.Seed(crypto.Keccak256Hash(getSeedData(prevBlock)))
	block.Header.EmptyBlockHeader.Flags = chain.calculateFlags(checkState, block)

	chain.applyEmptyBlockOnState(checkState, block, nil)

	block.Header.EmptyBlockHeader.Root = checkState.State.Root()
	block.Header.EmptyBlockHeader.IdentityRoot = checkState.IdentityState.Root()
	return block
}

func (chain *Blockchain) GenerateEmptyBlock() *types.Block {
	appState, _ := chain.appState.ForCheck(chain.Head.Height())
	return chain.generateEmptyBlock(appState, chain.Head)
}

func (chain *Blockchain) AddBlock(block *types.Block, checkState *appstate.AppState,
	statsCollector collector.StatsCollector) error {

	if err := validateBlockParentHash(block.Header, chain.Head); err != nil {
		return err
	}
	if err := chain.ValidateBlock(block, checkState); err != nil {
		return err
	}
	statsCollector.EnableCollecting()
	defer statsCollector.CompleteCollecting()
	diff, err := chain.processBlock(block, statsCollector)
	if err != nil {
		return err
	}
	if err := chain.insertBlock(block, diff); err != nil {
		return err
	}
	if !chain.isSyncing {
		chain.txpool.ResetTo(block)
	}

	chain.bus.Publish(&events.NewBlockEvent{
		Block: block,
	})
	chain.RemovePreliminaryHead(nil)
	return nil
}

func (chain *Blockchain) processBlock(block *types.Block,
	statsCollector collector.StatsCollector) (diff *state.IdentityStateDiff, err error) {

	var root, identityRoot common.Hash
	if block.IsEmpty() {
		root, identityRoot, diff = chain.applyEmptyBlockOnState(chain.appState, block, statsCollector)
	} else {
		if root, identityRoot, diff, err = chain.applyBlockAndTxsOnState(chain.appState, block, chain.Head, statsCollector); err != nil {
			chain.appState.Reset()
			return nil, err
		}
	}

	if root != block.Root() || identityRoot != block.IdentityRoot() {
		chain.appState.Reset()
		return nil, errors.Errorf("Process block. Invalid block roots. Expected=%x & %x, actual=%x & %x", root, identityRoot, block.Root(), block.IdentityRoot())
	}

	if err := chain.appState.Commit(block); err != nil {
		return nil, err
	}

	chain.log.Trace("Applied block", "root", fmt.Sprintf("0x%x", block.Root()), "height", block.Height())

	return diff, nil
}

func (chain *Blockchain) applyBlockAndTxsOnState(
	appState *appstate.AppState,
	block *types.Block,
	prevBlock *types.Header,
	statsCollector collector.StatsCollector,
) (root common.Hash, identityRoot common.Hash, diff *state.IdentityStateDiff, err error) {
	var totalFee, totalTips *big.Int
	if totalFee, totalTips, err = chain.processTxs(appState, block, statsCollector); err != nil {
		return
	}

	root, identityRoot, diff = chain.applyBlockOnState(appState, block, prevBlock, totalFee, totalTips, statsCollector)
	return root, identityRoot, diff, nil
}

func (chain *Blockchain) applyBlockOnState(appState *appstate.AppState, block *types.Block, prevBlock *types.Header, totalFee, totalTips *big.Int, statsCollector collector.StatsCollector) (root common.Hash, identityRoot common.Hash, diff *state.IdentityStateDiff) {

	chain.applyNewEpoch(appState, block, statsCollector)
	chain.applyBlockRewards(totalFee, totalTips, appState, block, prevBlock, statsCollector)
	chain.applyStatusSwitch(appState, block)
	chain.applyGlobalParams(appState, block, statsCollector)
	chain.applyNextBlockFee(appState, block)
	chain.applyVrfProposerThreshold(appState, block)
	diff = appState.Precommit()

	return appState.State.Root(), appState.IdentityState.Root(), diff
}

func (chain *Blockchain) applyEmptyBlockOnState(
	appState *appstate.AppState,
	block *types.Block,
	statsCollector collector.StatsCollector,
) (root common.Hash, identityRoot common.Hash, diff *state.IdentityStateDiff) {

	chain.applyNewEpoch(appState, block, statsCollector)
	chain.applyStatusSwitch(appState, block)
	chain.applyGlobalParams(appState, block, statsCollector)
	chain.applyVrfProposerThreshold(appState, block)
	diff = appState.Precommit()

	return appState.State.Root(), appState.IdentityState.Root(), diff
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

	reward, stake := splitReward(totalReward, appState.State.GetIdentityState(coinbase) == state.Newbie, chain.config.Consensus)

	// calculate penalty
	balanceAdd, stakeAdd, penaltySub := calculatePenalty(reward, stake, appState.State.GetPenalty(coinbase))

	// update state
	appState.State.AddBalance(coinbase, balanceAdd)
	appState.State.AddStake(coinbase, stakeAdd)
	if penaltySub != nil {
		appState.State.SubPenalty(coinbase, penaltySub)
	}
	collector.AfterBalanceUpdate(statsCollector, coinbase, appState)
	collector.AddMintedCoins(statsCollector, chain.config.Consensus.BlockReward)
	collector.AfterAddStake(statsCollector, coinbase, stake)
	collector.AfterSubPenalty(statsCollector, coinbase, penaltySub, appState)
	collector.AddPenaltyBurntCoins(statsCollector, coinbase, penaltySub)
	collector.AddProposerReward(statsCollector, coinbase, reward, stake)

	chain.rewardFinalCommittee(appState, block, prevBlock, statsCollector)
}

func calculatePenalty(balanceAppend *big.Int, stakeAppend *big.Int, currentPenalty *big.Int) (balanceAdd *big.Int, stakeAdd *big.Int, penaltySub *big.Int) {

	if currentPenalty == nil {
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
	networkSize, authors, failed := chain.applyNewEpochFn(block.Height(), appState, statsCollector)
	totalInvitesCount := float32(networkSize) * chain.config.Consensus.InvitesPercent
	setNewIdentitiesAttributes(appState, totalInvitesCount, networkSize, failed, authors, statsCollector)

	if !failed {
		rewardValidIdentities(appState, chain.config.Consensus, authors, block.Height()-appState.State.EpochBlock(), block.Seed(),
			statsCollector)
	}

	appState.State.IncEpoch()

	validationTime := appState.State.NextValidationTime()
	nextValidationTime := chain.config.Validation.GetNextValidationTime(validationTime, networkSize)
	appState.State.SetNextValidationTime(nextValidationTime)

	appState.State.SetFlipWordsSeed(block.Seed())

	appState.State.SetEpochBlock(block.Height())

	appState.State.SetGodAddressInvites(common.GodAddressInvitesCount(networkSize))
}

func calculateNewIdentityStatusFlags(authors *types.ValidationAuthors) map[common.Address]state.ValidationStatusFlag {
	m := make(map[common.Address]state.ValidationStatusFlag)
	for addr, item := range authors.AuthorResults {
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
	return m
}

type identityWithInvite struct {
	state   state.IdentityState
	address common.Address
	score   float32
}

func setInvites(appState *appstate.AppState, identitiesWithInvites []identityWithInvite, totalInvitesCount float32,
	statsCollector collector.StatsCollector) {

	getInvitesCount := func(s state.IdentityState) uint8 {
		if s == state.Human {
			return 2
		}
		if s == state.Verified {
			return 1
		}
		return 0
	}

	currentInvitesCount := int(totalInvitesCount)
	var index int
	var identity identityWithInvite
	for index, identity = range identitiesWithInvites {
		if currentInvitesCount <= 0 {
			break
		}
		invitesForIdentity := getInvitesCount(identity.state)
		appState.State.SetInvites(identity.address, invitesForIdentity)
		currentInvitesCount -= int(invitesForIdentity)
	}
	if index == 0 {
		return
	}
	lastScore := identitiesWithInvites[index-1].score
	for i := index; i < len(identitiesWithInvites) && identitiesWithInvites[i].score == lastScore; i++ {
		appState.State.SetInvites(identity.address, getInvitesCount(identitiesWithInvites[i].state))
	}
	collector.SetMinScoreForInvite(statsCollector, lastScore)
}

func setNewIdentitiesAttributes(appState *appstate.AppState, totalInvitesCount float32, networkSize int, validationFailed bool,
	authors *types.ValidationAuthors, statsCollector collector.StatsCollector) {
	_, flips := common.NetworkParams(networkSize)
	identityFlags := calculateNewIdentityStatusFlags(authors)

	identitiesWithInvites := make([]identityWithInvite, 0)
	addIdentityWithInvite := func(elem identityWithInvite) {
		index := sort.Search(len(identitiesWithInvites), func(i int) bool {
			return identitiesWithInvites[i].score < elem.score
		})
		identitiesWithInvites = append(identitiesWithInvites, identityWithInvite{})
		copy(identitiesWithInvites[index+1:], identitiesWithInvites[index:])
		identitiesWithInvites[index] = elem
	}

	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {
		if !validationFailed {
			switch identity.State {
			case state.Verified, state.Human:
				removeLinkWithInviter(appState.State, addr)
				addIdentityWithInvite(identityWithInvite{
					address: addr,
					state:   identity.State,
					score:   identity.GetTotalScore(),
				})
				appState.State.SetRequiredFlips(addr, uint8(flips))
				appState.IdentityState.Add(addr)
			case state.Newbie:
				appState.State.SetRequiredFlips(addr, uint8(flips))
				appState.IdentityState.Add(addr)
			case state.Killed, state.Undefined:
				removeLinksWithInviterAndInvitees(appState.State, addr)
				appState.State.SetRequiredFlips(addr, 0)
				appState.IdentityState.Remove(addr)
			default:
				appState.State.SetRequiredFlips(addr, 0)
				appState.IdentityState.Remove(addr)
			}
			appState.State.SetInvites(addr, 0)
		}
		collector.BeforeClearPenalty(statsCollector, addr, appState)
		appState.State.ClearPenalty(addr)
		appState.State.ClearFlips(addr)
		appState.State.ResetValidationTxBits(addr)

		if status, ok := identityFlags[addr]; ok && !validationFailed {
			appState.State.SetValidationStatus(addr, status)
		} else {
			appState.State.SetValidationStatus(addr, 0)
		}
	})

	setInvites(appState, identitiesWithInvites, totalInvitesCount, statsCollector)
}

func removeLinksWithInviterAndInvitees(stateDB *state.StateDB, addr common.Address) {
	removeLinkWithInviter(stateDB, addr)
	removeLinkWithInvitees(stateDB, addr)
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
		appState.State.SetValidationPeriod(state.NonePeriod)
	}

	if flags.HasFlag(types.Snapshot) {
		appState.State.SetLastSnapshot(block.Height())
	}

	if flags.HasFlag(types.OfflineCommit) {
		addr := block.Header.OfflineAddr()
		chain.applyOfflinePenalty(appState, *addr, statsCollector)
	}
}

func (chain *Blockchain) applyOfflinePenalty(appState *appstate.AppState, addr common.Address,
	statsCollector collector.StatsCollector) {
	networkSize := appState.ValidatorsCache.NetworkSize()

	if networkSize > 0 {
		totalBlockReward := new(big.Int).Add(chain.config.Consensus.FinalCommitteeReward, chain.config.Consensus.BlockReward)
		totalPenalty := new(big.Int).Mul(totalBlockReward, big.NewInt(chain.config.Consensus.OfflinePenaltyBlocksCount))
		coins := decimal.NewFromBigInt(totalPenalty, 0)
		res := coins.Div(decimal.New(int64(networkSize), 0))
		collector.BeforeSetPenalty(statsCollector, addr, appState)
		appState.State.SetPenalty(addr, math.ToInt(res))
	}

	appState.IdentityState.SetOnline(addr, false)
}

func (chain *Blockchain) rewardFinalCommittee(appState *appstate.AppState, block *types.Block, prevBlock *types.Header,
	statsCollector collector.StatsCollector) {
	if block.IsEmpty() {
		return
	}
	identities := appState.ValidatorsCache.GetOnlineValidators(prevBlock.Seed(), block.Height(), types.Final, chain.GetCommitteeSize(appState.ValidatorsCache, true))
	if identities == nil || identities.Cardinality() == 0 {
		return
	}
	totalReward := big.NewInt(0)
	totalReward.Div(chain.config.Consensus.FinalCommitteeReward, big.NewInt(int64(identities.Cardinality())))

	reward, stake := splitReward(totalReward, false, chain.config.Consensus)
	newbieReward, newbieStake := splitReward(totalReward, true, chain.config.Consensus)

	for _, item := range identities.ToSlice() {
		addr := item.(common.Address)

		identityState := appState.State.GetIdentityState(addr)
		r, s := reward, stake
		if identityState == state.Newbie {
			r, s = newbieReward, newbieStake
		}

		// calculate penalty
		balanceAdd, stakeAdd, penaltySub := calculatePenalty(r, s, appState.State.GetPenalty(addr))

		// update state
		appState.State.AddBalance(addr, balanceAdd)
		appState.State.AddStake(addr, stakeAdd)
		if penaltySub != nil {
			appState.State.SubPenalty(addr, penaltySub)
		}
		collector.AfterBalanceUpdate(statsCollector, addr, appState)
		collector.AddMintedCoins(statsCollector, r)
		collector.AddMintedCoins(statsCollector, s)
		collector.AfterAddStake(statsCollector, addr, s)
		collector.AfterSubPenalty(statsCollector, addr, penaltySub, appState)
		collector.AddPenaltyBurntCoins(statsCollector, addr, penaltySub)
		collector.AddFinalCommitteeReward(statsCollector, addr, r, s)
	}
}

func (chain *Blockchain) processTxs(appState *appstate.AppState, block *types.Block,
	statsCollector collector.StatsCollector) (totalFee *big.Int, totalTips *big.Int, err error) {
	totalFee = new(big.Int)
	totalTips = new(big.Int)
	fee := new(big.Int)
	for i := 0; i < len(block.Body.Transactions); i++ {
		tx := block.Body.Transactions[i]
		if err := validation.ValidateTx(appState, tx, chain.config.Consensus.MinFeePerByte, validation.InBlockTx); err != nil {
			return nil, nil, err
		}
		if fee, err = chain.ApplyTxOnState(appState, tx, statsCollector); err != nil {
			return nil, nil, err
		}

		totalFee.Add(totalFee, fee)
		totalTips.Add(totalTips, tx.TipsOrZero())
	}

	return totalFee, totalTips, nil
}

func (chain *Blockchain) ApplyTxOnState(appState *appstate.AppState, tx *types.Transaction,
	statsCollector collector.StatsCollector) (*big.Int, error) {

	stateDB := appState.State

	sender, _ := types.Sender(tx)

	globalState := stateDB.GetOrNewGlobalObject()
	senderAccount := stateDB.GetOrNewAccountObject(sender)

	if tx.Epoch != globalState.Epoch() {
		return nil, errors.New(fmt.Sprintf("invalid tx epoch. Tx=%v expectedEpoch=%v actualEpoch=%v", tx.Hash().Hex(),
			globalState.Epoch(), tx.Epoch))
	}

	currentNonce := senderAccount.Nonce()
	// if epoch was increased, we should reset nonce to 1
	if senderAccount.Epoch() < globalState.Epoch() {
		currentNonce = 0
	}

	if currentNonce+1 != tx.AccountNonce {
		return nil, errors.New(fmt.Sprintf("invalid tx nonce. Tx=%v expectedNonce=%v actualNonce=%v", tx.Hash().Hex(),
			currentNonce+1, tx.AccountNonce))
	}

	feePerByte := appState.State.FeePerByte()
	fee := chain.getTxFee(feePerByte, tx)
	totalCost := chain.getTxCost(feePerByte, tx)

	switch tx.Type {
	case types.ActivationTx:
		balance := stateDB.GetBalance(sender)
		generation, code := stateDB.GeneticCode(sender)
		change := new(big.Int).Sub(balance, totalCost)

		// zero balance and kill temp identity
		stateDB.SetBalance(sender, big.NewInt(0))
		stateDB.SetState(sender, state.Killed)

		// verify identity and add transfer all available funds from temp account
		recipient := *tx.To
		stateDB.SetState(recipient, state.Candidate)
		stateDB.AddBalance(recipient, change)
		stateDB.SetPubKey(recipient, tx.Payload)
		stateDB.SetGeneticCode(recipient, generation, code)

		inviter := stateDB.GetInviter(sender)
		if inviter != nil {
			removeLinkWithInviter(appState.State, sender)
			if inviter.Address == stateDB.GodAddress() || stateDB.GetIdentityState(inviter.Address).VerifiedOrBetter() {
				stateDB.AddInvitee(inviter.Address, recipient, inviter.TxHash)
				stateDB.SetInviter(recipient, inviter.Address, inviter.TxHash)
			}
		}

		collector.AfterBalanceUpdate(statsCollector, sender, appState)
		collector.AfterKillIdentity(statsCollector, sender, appState)
		collector.AfterBalanceUpdate(statsCollector, recipient, appState)
		collector.AddActivationTxBalanceTransfer(statsCollector, tx, change)
		if sender != *tx.To {
			collector.AddInviteBurntCoins(statsCollector, sender, appState.State.GetStakeBalance(sender), tx)
		}
	case types.SendTx:
		stateDB.SubBalance(sender, totalCost)
		stateDB.AddBalance(*tx.To, tx.AmountOrZero())
		collector.AfterBalanceUpdate(statsCollector, sender, appState)
		collector.AfterBalanceUpdate(statsCollector, *tx.To, appState)
	case types.BurnTx:
		stateDB.SubBalance(sender, totalCost)
		collector.AfterBalanceUpdate(statsCollector, sender, appState)
		collector.AddBurnTxBurntCoins(statsCollector, sender, tx)
	case types.InviteTx:
		if sender == stateDB.GodAddress() {
			stateDB.SubGodAddressInvite()
		} else {
			stateDB.SubInvite(sender, 1)
		}

		stateDB.SubBalance(sender, totalCost)
		generation, code := stateDB.GeneticCode(sender)

		stateDB.SetState(*tx.To, state.Invite)
		stateDB.AddBalance(*tx.To, tx.AmountOrZero())
		stateDB.SetGeneticCode(*tx.To, generation+1, append(code[1:], sender[0]))

		stateDB.SetInviter(*tx.To, sender, tx.Hash())
		collector.AfterBalanceUpdate(statsCollector, sender, appState)
		collector.AfterBalanceUpdate(statsCollector, *tx.To, appState)
	case types.KillTx:
		removeLinksWithInviterAndInvitees(stateDB, sender)
		stateDB.SetState(sender, state.Killed)
		appState.IdentityState.Remove(sender)
		amount := tx.AmountOrZero()
		stateDB.SubBalance(sender, amount)
		stateDB.SubBalance(sender, tx.TipsOrZero())
		stake := stateDB.GetStakeBalance(sender)
		stateDB.SubStake(sender, stake)
		stakeToTransfer := new(big.Int).Sub(stake, fee)
		stateDB.AddBalance(*tx.To, stakeToTransfer)
		stateDB.AddBalance(*tx.To, amount)
		collector.AfterBalanceUpdate(statsCollector, sender, appState)
		collector.AfterBalanceUpdate(statsCollector, *tx.To, appState)
		collector.AfterKillIdentity(statsCollector, sender, appState)
		collector.AddKillTxStakeTransfer(statsCollector, tx, stakeToTransfer)
	case types.KillInviteeTx:
		removeLinksWithInviterAndInvitees(stateDB, *tx.To)
		inviteePrevState := stateDB.GetIdentityState(*tx.To)
		stateDB.SetState(*tx.To, state.Killed)
		appState.IdentityState.Remove(*tx.To)
		stateDB.SubBalance(sender, fee)
		stateDB.SubBalance(sender, tx.TipsOrZero())
		if inviteePrevState == state.Newbie {
			stakeToTransfer := stateDB.GetStakeBalance(*tx.To)
			stateDB.AddBalance(sender, stakeToTransfer)
			collector.AddKillInviteeTxStakeTransfer(statsCollector, tx, stakeToTransfer)
		}
		if sender != stateDB.GodAddress() && stateDB.GetIdentityState(sender).VerifiedOrBetter() &&
			(inviteePrevState == state.Invite || inviteePrevState == state.Candidate) {
			stateDB.AddInvite(sender, 1)
		}
		collector.AfterBalanceUpdate(statsCollector, sender, appState)
		collector.AfterBalanceUpdate(statsCollector, *tx.To, appState)
		collector.AfterKillIdentity(statsCollector, *tx.To, appState)
	case types.SubmitFlipTx:
		stateDB.SubBalance(sender, fee)
		stateDB.SubBalance(sender, tx.TipsOrZero())
		attachment := attachments.ParseFlipSubmitAttachment(tx)
		stateDB.AddFlip(sender, attachment.Cid, attachment.Pair)
		collector.AfterBalanceUpdate(statsCollector, sender, appState)
	case types.OnlineStatusTx:
		stateDB.SubBalance(sender, fee)
		stateDB.SubBalance(sender, tx.TipsOrZero())
		stateDB.ToggleStatusSwitchAddress(sender)
		collector.AfterBalanceUpdate(statsCollector, sender, appState)
	case types.ChangeGodAddressTx:
		stateDB.SubBalance(sender, fee)
		stateDB.SubBalance(sender, tx.TipsOrZero())
		appState.State.SetGodAddress(*tx.To)
		collector.AfterBalanceUpdate(statsCollector, sender, appState)
	case types.ChangeProfileTx:
		stateDB.SubBalance(sender, fee)
		stateDB.SubBalance(sender, tx.TipsOrZero())
		attachment := attachments.ParseChangeProfileAttachment(tx)
		stateDB.SetProfileHash(sender, attachment.Hash)
		collector.AfterBalanceUpdate(statsCollector, sender, appState)
	case types.DeleteFlipTx:
		stateDB.SubBalance(sender, fee)
		stateDB.SubBalance(sender, tx.TipsOrZero())
		attachment := attachments.ParseDeleteFlipAttachment(tx)
		stateDB.DeleteFlip(sender, attachment.Cid)
		collector.AfterBalanceUpdate(statsCollector, sender, appState)
	case types.SubmitAnswersHashTx, types.SubmitShortAnswersTx, types.EvidenceTx, types.SubmitLongAnswersTx:
		stateDB.SetValidationTxBit(sender, tx.Type)
	}

	stateDB.SetNonce(sender, tx.AccountNonce)

	if senderAccount.Epoch() != tx.Epoch {
		stateDB.SetEpoch(sender, tx.Epoch)
	}
	collector.AddFeeBurntCoins(statsCollector, sender, fee, chain.config.Consensus.FeeBurnRate, tx)

	return fee, nil
}

func (chain *Blockchain) getTxFee(feePerByte *big.Int, tx *types.Transaction) *big.Int {
	return fee.CalculateFee(chain.appState.ValidatorsCache.NetworkSize(), feePerByte, tx)
}

func (chain *Blockchain) applyNextBlockFee(appState *appstate.AppState, block *types.Block) {
	feePerByte := chain.calculateNextBlockFeePerByte(appState, block)
	appState.State.SetFeePerByte(feePerByte)
}

func (chain *Blockchain) calculateNextBlockFeePerByte(appState *appstate.AppState, block *types.Block) *big.Int {
	feePerByte := appState.State.FeePerByte()
	if feePerByte == nil || feePerByte.Cmp(chain.config.Consensus.MinFeePerByte) == -1 {
		feePerByte = new(big.Int).Set(chain.config.Consensus.MinFeePerByte)
	}

	blockSize := len(block.Body.Bytes())
	k := chain.config.Consensus.FeeSensitivityCoef
	maxBlockSize := mempool.BlockBodySize

	// curBlockFee = prevBlockFee * (1 + k * (prevBlockSize / maxBlockSize - 0.5))
	newFeePerByteD := decimal.New(int64(blockSize), 0).
		Div(decimal.New(int64(maxBlockSize), 0)).
		Sub(decimal.NewFromFloat(0.5)).
		Mul(decimal.NewFromFloat32(k)).
		Add(decimal.New(1, 0)).
		Mul(decimal.NewFromBigInt(feePerByte, 0))

	newFeePerByte := math.ToInt(newFeePerByteD)
	if newFeePerByte.Cmp(chain.config.Consensus.MinFeePerByte) == -1 {
		newFeePerByte = new(big.Int).Set(chain.config.Consensus.MinFeePerByte)
	}
	return newFeePerByte
}

func (chain *Blockchain) applyVrfProposerThreshold(appState *appstate.AppState, block *types.Block) {
	appState.State.AddBlockBit(block.IsEmpty())
	currentThreshold := appState.State.VrfProposerThreshold()

	online := float64(appState.ValidatorsCache.OnlineSize())
	if online == 0 {
		online = 1
	}

	emptyBlocks := appState.State.EmptyBlocksCount()

	minVrf := math2.Max(0.5, 1.0-10.0/online)
	maxVrf := math2.Max(0.5, 1.0-1.0/online)

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
		currentStatus := appState.IdentityState.IsOnline(addr)
		appState.IdentityState.SetOnline(addr, !currentStatus)
	}

	appState.State.ClearStatusSwitchAddresses()
}

func (chain *Blockchain) getTxCost(feePerByte *big.Int, tx *types.Transaction) *big.Int {
	return fee.CalculateCost(chain.appState.ValidatorsCache.NetworkSize(), feePerByte, tx)
}

func getSeedData(prevBlock *types.Header) []byte {
	result := prevBlock.Seed().Bytes()
	result = append(result, common.ToBytes(prevBlock.Height()+1)...)
	return result
}

func (chain *Blockchain) GetProposerSortition() (bool, common.Hash, []byte) {

	if checkIfProposer(chain.coinBaseAddress, chain.appState) {
		return chain.getSortition(chain.getProposerData(), chain.appState.State.VrfProposerThreshold())
	}

	return false, common.Hash{}, nil
}

func (chain *Blockchain) ProposeBlock() *types.BlockProposal {
	head := chain.Head

	txs := chain.txpool.BuildBlockTransactions()
	checkState, _ := chain.appState.ForCheck(chain.Head.Height())

	filteredTxs, totalFee, totalTips := chain.filterTxs(checkState, txs)
	body := &types.Body{
		Transactions: filteredTxs,
	}
	var cid cid2.Cid
	cid, _ = chain.ipfs.Cid(body.Bytes())

	prevBlockTime := time.Unix(chain.Head.Time().Int64(), 0)
	newBlockTime := prevBlockTime.Add(MinBlockDelay).Unix()
	if localTime := time.Now().UTC().Unix(); localTime > newBlockTime {
		newBlockTime = localTime
	}
	var cidBytes []byte
	if cid != ipfs.EmptyCid {
		cidBytes = cid.Bytes()
	}
	header := &types.ProposedHeader{
		Height:         head.Height() + 1,
		ParentHash:     head.Hash(),
		Time:           new(big.Int).SetInt64(newBlockTime),
		ProposerPubKey: chain.pubKey,
		TxHash:         types.DeriveSha(types.Transactions(filteredTxs)),
		IpfsHash:       cidBytes,
		FeePerByte:     chain.appState.State.FeePerByte(),
	}

	block := &types.Block{
		Header: &types.Header{
			ProposedHeader: header,
		},
		Body: body,
	}

	block.Header.ProposedHeader.BlockSeed, block.Header.ProposedHeader.SeedProof = chain.secStore.VrfEvaluate(getSeedData(head))

	block.Header.ProposedHeader.TxBloom = calculateTxBloom(block)

	addr, flag := chain.offlineDetector.ProposeOffline(head)
	if addr != nil {
		block.Header.ProposedHeader.OfflineAddr = addr
		block.Header.ProposedHeader.Flags |= flag
	}
	block.Header.ProposedHeader.Flags |= chain.calculateFlags(checkState, block)

	block.Header.ProposedHeader.Root, block.Header.ProposedHeader.IdentityRoot, _ = chain.applyBlockOnState(checkState, block, chain.Head, totalFee, totalTips, nil)

	proposal := &types.BlockProposal{Block: block, Signature: chain.secStore.Sign(block.Hash().Bytes())}

	return proposal
}

func calculateTxBloom(block *types.Block) []byte {
	if block.IsEmpty() {
		return []byte{}
	}
	if len(block.Body.Transactions) == 0 {
		return []byte{}
	}

	addrs := make(map[common.Address]bool)

	for _, tx := range block.Body.Transactions {
		sender, _ := types.Sender(tx)
		addrs[sender] = true
		if tx.To != nil {
			addrs[*tx.To] = true
		}
	}
	bloom := common.NewSerializableBF(len(addrs))
	for addr := range addrs {
		bloom.Add(addr)
	}
	data, _ := bloom.Serialize()
	return data
}

func (chain *Blockchain) calculateFlags(appState *appstate.AppState, block *types.Block) types.BlockFlag {

	var flags types.BlockFlag

	for _, tx := range block.Body.Transactions {
		if tx.Type == types.KillTx || tx.Type == types.KillInviteeTx {
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

	if stateDb.ValidationPeriod() == state.AfterLongSessionPeriod &&
		chain.timing.isValidationFinished(stateDb.NextValidationTime(), block.Header.Time(), appState.ValidatorsCache.NetworkSize()) {
		flags |= types.ValidationFinished
		flags |= types.IdentityUpdate
	}

	if block.Height()-appState.State.LastSnapshot() >= chain.config.Consensus.SnapshotRange && appState.State.ValidationPeriod() == state.NonePeriod &&
		!flags.HasFlag(types.ValidationFinished) && !flags.HasFlag(types.FlipLotteryStarted) {
		flags |= types.Snapshot
	}

	if block.Header.Flags().HasFlag(types.OfflineCommit) {
		flags |= types.IdentityUpdate
	}

	if (flags.HasFlag(types.Snapshot) || block.Height()%chain.config.Consensus.StatusSwitchRange == 0) && len(appState.State.StatusSwitchAddresses()) > 0 {
		flags |= types.IdentityUpdate
	}

	return flags
}

func (chain *Blockchain) filterTxs(appState *appstate.AppState, txs []*types.Transaction) ([]*types.Transaction, *big.Int, *big.Int) {
	var result []*types.Transaction

	totalFee := new(big.Int)
	totalTips := new(big.Int)
	for _, tx := range txs {
		if err := validation.ValidateTx(appState, tx, chain.config.Consensus.MinFeePerByte, validation.InBlockTx); err != nil {
			continue
		}
		if fee, err := chain.ApplyTxOnState(appState, tx, nil); err == nil {
			totalFee.Add(totalFee, fee)
			totalTips.Add(totalTips, tx.TipsOrZero())
			result = append(result, tx)
		}
	}
	return result, totalFee, totalTips
}

func (chain *Blockchain) insertHeader(header *types.Header) {
	chain.repo.WriteBlockHeader(header)
	chain.repo.WriteHead(nil, header)
	chain.repo.WriteCanonicalHash(header.Height(), header.Hash())
}

func (chain *Blockchain) insertBlock(block *types.Block, diff *state.IdentityStateDiff) error {
	_, err := chain.ipfs.Add(block.Body.Bytes(), chain.ipfs.ShouldPin(ipfs.Block))
	if err != nil {
		return errors.Wrap(BlockInsertionErr, err.Error())
	}
	chain.insertHeader(block.Header)
	chain.WriteIdentityStateDiff(block.Height(), diff)
	chain.WriteTxIndex(block.Hash(), block.Body.Transactions)
	chain.indexer.HandleBlockTransactions(block.Header, block.Body.Transactions)
	chain.setCurrentHead(block.Header)
	return nil
}

func (chain *Blockchain) WriteTxIndex(hash common.Hash, txs types.Transactions) {
	for i, tx := range txs {
		idx := &types.TransactionIndex{
			BlockHash: hash,
			Idx:       uint16(i),
		}
		chain.repo.WriteTxIndex(tx.Hash(), idx)
	}
}

func (chain *Blockchain) getProposerData() []byte {
	head := chain.Head
	result := head.Seed().Bytes()
	result = append(result, common.ToBytes(ProposerRole)...)
	result = append(result, common.ToBytes(head.Height()+1)...)
	return result
}

func (chain *Blockchain) getSortition(data []byte, threshold float64) (bool, common.Hash, []byte) {
	hash, proof := chain.secStore.VrfEvaluate(data)

	v := new(big.Float).SetInt(new(big.Int).SetBytes(hash[:]))

	q := new(big.Float).Quo(v, MaxHash).SetPrec(10)

	if f, _ := q.Float64(); f >= threshold {
		return true, hash, proof
	}
	return false, common.Hash{}, nil
}

func (chain *Blockchain) validateBlock(checkState *appstate.AppState, block *types.Block, prevBlock *types.Header) error {

	if block.IsEmpty() {
		if chain.generateEmptyBlock(checkState, prevBlock).Hash() == block.Hash() {
			return nil
		}
		return errors.New("empty blocks' hashes mismatch")
	}

	if err := chain.ValidateHeader(block.Header, prevBlock); err != nil {
		return err
	}

	if checkState.State.FeePerByte().Cmp(block.Header.ProposedHeader.FeePerByte) != 0 {
		return errors.New("fee rate is invalid")
	}

	proposerAddr, _ := crypto.PubKeyBytesToAddress(block.Header.ProposedHeader.ProposerPubKey)

	if !checkIfProposer(proposerAddr, checkState) {
		return errors.New("proposer is not identity")
	}

	var txs = types.Transactions(block.Body.Transactions)

	if types.DeriveSha(txs) != block.Header.ProposedHeader.TxHash {
		return errors.New("txHash is invalid")
	}

	if bytes.Compare(calculateTxBloom(block), block.Header.ProposedHeader.TxBloom) != 0 {
		return errors.New("tx bloom is invalid")
	}

	var totalFee, totalTips *big.Int
	var err error
	if totalFee, totalTips, err = chain.processTxs(checkState, block, nil); err != nil {
		return err
	}

	persistentFlags := block.Header.ProposedHeader.Flags.UnsetFlag(types.OfflinePropose).UnsetFlag(types.OfflineCommit)

	if expected := chain.calculateFlags(checkState, block); expected != persistentFlags {
		return errors.Errorf("flags are invalid, expected=%v, actual=%v", expected, persistentFlags)
	}

	if root, identityRoot, _ := chain.applyBlockOnState(checkState, block, prevBlock, totalFee, totalTips, nil); root != block.Root() || identityRoot != block.IdentityRoot() {
		return errors.Errorf("invalid block roots. Expected=%x & %x, actual=%x & %x", root, identityRoot, block.Root(), block.IdentityRoot())
	}

	cid, _ := chain.ipfs.Cid(block.Body.Bytes())

	var cidBytes []byte
	if cid != ipfs.EmptyCid {
		cidBytes = cid.Bytes()
	}
	if bytes.Compare(cidBytes, block.Header.ProposedHeader.IpfsHash) != 0 {
		return errors.New("invalid block cid")
	}

	return nil
}

func (chain *Blockchain) ValidateBlockCertOnHead(block *types.Header, cert *types.BlockCert) error {
	return chain.ValidateBlockCert(chain.Head, block, cert, chain.appState.ValidatorsCache)
}

func (chain *Blockchain) ValidateBlockCert(prevBlock *types.Header, block *types.Header, cert *types.BlockCert, validatorsCache *validators.ValidatorsCache) (err error) {

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

		if !validators.Contains(vote.VoterAddr()) {
			return errors.New("invalid voter")
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
		voters.Add(vote.VoterAddr())
	}

	if voters.Cardinality() < chain.GetCommitteeVotesThreshold(validatorsCache, step == types.Final) {
		return errors.New("not enough votes")
	}
	return nil
}

func (chain *Blockchain) ValidateBlock(block *types.Block, checkState *appstate.AppState) error {
	if checkState == nil {
		var err error
		checkState, err = chain.appState.ForCheck(chain.Head.Height())
		if err != nil {
			return err
		}
	}

	return chain.validateBlock(checkState, block, chain.Head)
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
	blockTime := time.Unix(block.Time().Int64(), 0)

	if blockTime.Sub(time.Now().UTC()) > MaxFutureBlockOffset {
		return errors.New("block from future")
	}
	prevBlockTime := time.Unix(prevBlock.Time().Int64(), 0)

	if blockTime.Sub(prevBlockTime) < MinBlockDelay {
		return errors.Errorf("block is too close to previous one, prev: %v, current: %v", prevBlockTime.Unix(), blockTime.Unix())
	}

	return nil
}

func (chain *Blockchain) ValidateProposerProof(proof []byte, hash common.Hash, pubKeyData []byte) error {
	pubKey, err := crypto.UnmarshalPubkey(pubKeyData)
	if err != nil {
		return err
	}
	verifier, err := p256.NewVRFVerifier(pubKey)
	if err != nil {
		return err
	}

	h, err := verifier.ProofToHash(chain.getProposerData(), proof)

	if h != hash {
		return errors.New("Hashes are not equal")
	}

	v := new(big.Float).SetInt(new(big.Int).SetBytes(hash[:]))

	q := new(big.Float).Quo(v, MaxHash).SetPrec(10)

	if f, _ := q.Float64(); f < chain.appState.State.VrfProposerThreshold() {
		return errors.New("Proposer is invalid")
	}

	proposerAddr := crypto.PubkeyToAddress(*pubKey)

	if !checkIfProposer(proposerAddr, chain.appState) {
		return errors.New("Proposer is not identity")
	}
	return nil
}

func (chain *Blockchain) Round() uint64 {
	return chain.Head.Height() + 1
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

	if uint16(len(body.Transactions)) < idx.Idx {
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

	size := int(float64(cnt) * percent)
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
	return int(float64(size) * chain.config.Consensus.AgreementThreshold)
}

func (chain *Blockchain) Genesis() common.Hash {
	return chain.genesis.Hash()
}

func (chain *Blockchain) ValidateSubChain(startHeight uint64, blocks []types.BlockBundle) error {
	checkState, err := chain.appState.ForCheckWithOverwrite(startHeight)
	if err != nil {
		return err
	}
	prevBlock := chain.GetBlockHeaderByHeight(startHeight)

	for _, b := range blocks {
		if err := chain.validateBlock(checkState, b.Block, prevBlock); err != nil {
			return err
		}
		if b.Block.Header.Flags().HasFlag(types.IdentityUpdate) {
			if b.Cert.Empty() {
				return errors.New("Block cert is missing")
			}
		}
		if !b.Cert.Empty() {
			if err := chain.ValidateBlockCert(prevBlock, b.Block.Header, b.Cert, checkState.ValidatorsCache); err != nil {
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

func (chain *Blockchain) ResetTo(height uint64) error {
	prevHead := chain.Head.Height()
	if err := chain.appState.ResetTo(height); err != nil {
		return errors.WithMessage(err, "state is corrupted, try to resync from scratch")
	}
	chain.setHead(height, nil)

	for h := height + 1; h <= prevHead; h++ {
		hash := chain.repo.ReadCanonicalHash(h)
		if hash == (common.Hash{}) {
			continue
		}
		chain.repo.RemoveHeader(hash)
		chain.repo.RemoveCanonicalHash(h)
	}

	return nil
}

func (chain *Blockchain) EnsureIntegrity() error {
	wasReset := false
	for chain.Head.Root() != chain.appState.State.Root() ||
		chain.Head.IdentityRoot() != chain.appState.IdentityState.Root() {
		wasReset = true
		resetTo := uint64(0)
		for h, tryCnt := chain.Head.Height()-1, 0; h >= 1 && tryCnt < int(state.SyncTreeKeepEvery)+1; h, tryCnt = h-1, tryCnt+1 {
			if chain.appState.IdentityState.HasVersion(h) {
				resetTo = h
				break
			}
		}
		if resetTo == 0 {
			return errors.New("state db is corrupted, try to delete idenachain.db folder from your data directory and sync from scratch")
		}
		if err := chain.ResetTo(resetTo); err != nil {
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

func (chain *Blockchain) AddHeader(header *types.Header) error {

	prev := chain.PreliminaryHead
	if prev == nil {
		prev = chain.Head
	}
	if err := chain.ValidateHeader(header, prev); err != nil {
		return err
	}

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
	rlp.DecodeBytes(data, diff)
	return diff
}

func (chain *Blockchain) ReadSnapshotManifest() *snapshot.Manifest {
	cid, root, height, _ := chain.repo.LastSnapshotManifest()
	if cid == nil {
		return nil
	}
	return &snapshot.Manifest{
		Cid:    cid,
		Root:   root,
		Height: height,
	}
}

func (chain *Blockchain) ReadPreliminaryHead() *types.Header {
	return chain.repo.ReadPreliminaryHead()
}

func (chain *Blockchain) WriteIdentityStateDiff(height uint64, diff *state.IdentityStateDiff) {
	if !diff.Empty() {
		chain.repo.WriteIdentityStateDiff(height, diff.Bytes())
	}
}

func (chain *Blockchain) RemovePreliminaryHead(batch dbm.Batch) {
	if chain.PreliminaryHead != nil {
		chain.repo.RemovePreliminaryHead(batch)
	}
	chain.PreliminaryHead = nil
}

func (chain *Blockchain) IsPermanentCert(header *types.Header) bool {
	return header.Flags().HasFlag(types.IdentityUpdate|types.Snapshot) ||
		header.Height()%chain.config.Blockchain.StoreCertRange == 0
}

func (chain *Blockchain) ReadTxs(address common.Address, count int, token []byte) ([]*types.SavedTransaction, []byte) {
	return chain.repo.GetSavedTxs(address, count, token)
}

func (chain *Blockchain) ReadTotalBurntCoins() []*types.BurntCoins {
	return chain.repo.GetTotalBurntCoins()
}

func readPredefinedState() (*state.PredefinedState, error) {
	data, err := Asset("stategen.out")
	if err != nil {
		return nil, err
	}
	predefinedState := new(state.PredefinedState)
	if err := rlp.DecodeBytes(data, predefinedState); err != nil {
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

	oldStateDb := chain.appState.State.CommitSnapshot(manifest, batch)
	chain.appState.ValidatorsCache.Load()

	chain.setHead(chain.PreliminaryHead.Height(), batch)
	newHead := chain.PreliminaryHead
	chain.RemovePreliminaryHead(batch)
	if err := batch.WriteSync(); err != nil {
		return err
	}
	chain.setCurrentHead(newHead)
	go func() {
		common.ClearDb(oldIdentityStateDb)
		common.ClearDb(oldStateDb)
	}()
	return nil
}
