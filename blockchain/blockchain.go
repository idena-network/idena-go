package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	mapset "github.com/deckarep/golang-set"
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
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/secstore"
	cid2 "github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	dbm "github.com/tendermint/tendermint/libs/db"
	"math/big"
	"time"
)

const (
	Mainnet types.Network = 0x0
	Testnet types.Network = 0x1
)

const (
	ProposerRole            uint8 = 0x1
	EmptyBlockTimeIncrement       = time.Second * 10
	MaxFutureBlockOffset          = time.Minute * 2
	MinBlockDelay                 = time.Second * 10
)

var (
	MaxHash             *big.Float
	ParentHashIsInvalid = errors.New("parentHash is invalid")
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
	secretKey       *ecdsa.PrivateKey
	ipfs            ipfs.Proxy
	timing          *timing
	bus             eventbus.Bus
	applyNewEpochFn func(appState *appstate.AppState) int
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

func NewBlockchain(config *config.Config, db dbm.DB, txpool *mempool.TxPool, appState *appstate.AppState, ipfs ipfs.Proxy, secStore *secstore.SecStore, bus eventbus.Bus) *Blockchain {
	return &Blockchain{
		repo:     database.NewRepo(db),
		config:   config,
		log:      log.New(),
		txpool:   txpool,
		appState: appState,
		ipfs:     ipfs,
		timing:   NewTiming(config.Validation),
		bus:      bus,
		secStore: secStore,
	}
}

func (chain *Blockchain) ProvideApplyNewEpochFunc(fn func(appState *appstate.AppState) int) {
	chain.applyNewEpochFn = fn
}

func (chain *Blockchain) GetHead() *types.Header {
	head := chain.repo.ReadHead()
	return head
}

func (chain *Blockchain) Network() types.Network {
	return chain.config.Network
}

func (chain *Blockchain) Config() config.Config {
	return *chain.config
}

func (chain *Blockchain) InitializeChain() error {

	chain.coinBaseAddress = chain.secStore.GetAddress()
	chain.pubKey = chain.secStore.GetPubKey()
	head := chain.GetHead()
	if head != nil {
		chain.setCurrentHead(head)
		if chain.genesis = chain.GetBlockHeaderByHeight(1); chain.genesis == nil {
			return errors.New("genesis block is not found")
		}
	} else {
		_, err := chain.GenerateGenesis(chain.config.Network)
		if err != nil {
			return err
		}
	}
	chain.PreliminaryHead = chain.repo.ReadPreliminaryHead()
	log.Info("Chain initialized", "block", chain.Head.Hash().Hex(), "height", chain.Head.Height())
	log.Info("Coinbase address", "addr", chain.coinBaseAddress.Hex())
	return nil
}

func (chain *Blockchain) setCurrentHead(head *types.Header) {
	chain.Head = head
}

func (chain *Blockchain) setHead(height uint64) {
	chain.repo.SetHead(height)
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
		if state.IdentityState(alloc.State) == state.Verified {
			chain.appState.IdentityState.Add(addr)
		}
	}

	chain.appState.State.SetGodAddress(chain.config.GenesisConf.GodAddress)

	seed := types.Seed(crypto.Keccak256Hash(append([]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6}, common.ToBytes(network)...)))

	nextValidationTimestamp := chain.config.GenesisConf.FirstCeremonyTime
	if nextValidationTimestamp == 0 {
		nextValidationTimestamp = time.Now().UTC().Unix()
	}
	chain.appState.State.SetNextValidationTime(time.Unix(nextValidationTimestamp, 0))
	chain.appState.State.SetFlipWordsSeed(seed)

	log.Info("Next validation time", "time", chain.appState.State.NextValidationTime().String(), "unix", nextValidationTimestamp)

	if err := chain.appState.Commit(nil); err != nil {
		return nil, err
	}

	var emptyHash [32]byte

	block := &types.Block{Header: &types.Header{
		ProposedHeader: &types.ProposedHeader{
			ParentHash:   emptyHash,
			Time:         big.NewInt(0),
			Height:       1,
			Root:         chain.appState.State.Root(),
			IdentityRoot: chain.appState.IdentityState.Root(),
			BlockSeed:    seed,
			IpfsHash:     ipfs.EmptyCid.Bytes(),
		},
	}, Body: &types.Body{}}

	if err := chain.insertBlock(block, state.IdentityStateDiff{}); err != nil {
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

	chain.applyEmptyBlockOnState(checkState, block)

	block.Header.EmptyBlockHeader.Root = checkState.State.Root()
	block.Header.EmptyBlockHeader.IdentityRoot = checkState.IdentityState.Root()
	return block
}

func (chain *Blockchain) GenerateEmptyBlock() *types.Block {
	return chain.generateEmptyBlock(chain.appState.Readonly(chain.Head.Height()), chain.Head)
}

func (chain *Blockchain) AddBlock(block *types.Block, checkState *appstate.AppState) error {

	if err := validateBlockParentHash(block.Header, chain.Head); err != nil {
		return err
	}
	if err := chain.ValidateBlock(block, checkState); err != nil {
		return err
	}
	diff, err := chain.processBlock(block)
	if err != nil {
		return err
	}

	if err := chain.insertBlock(block, diff); err != nil {
		return err
	}

	chain.bus.Publish(&events.NewBlockEvent{
		Block: block,
	})

	return nil
}

func (chain *Blockchain) processBlock(block *types.Block) (diff state.IdentityStateDiff, err error) {
	var root, identityRoot common.Hash
	if block.IsEmpty() {
		root, identityRoot = chain.applyEmptyBlockOnState(chain.appState, block)
	} else {
		if root, identityRoot, diff, err = chain.applyBlockOnState(chain.appState, block, chain.Head); err != nil {
			chain.appState.Reset()
			return nil, err
		}
	}

	if root != block.Root() || identityRoot != block.IdentityRoot() {
		chain.appState.Reset()
		return nil, errors.Errorf("Invalid block root. Exptected=%x, blockroot=%x", root, block.Root())
	}

	if err := chain.appState.Commit(block); err != nil {
		return nil, err
	}
	chain.log.Trace("Applied block", "root", fmt.Sprintf("0x%x", block.Root()), "height", block.Height())

	if !chain.isSyncing {
		chain.txpool.ResetTo(block)
	}
	return diff, nil
}

func (chain *Blockchain) applyBlockOnState(appState *appstate.AppState, block *types.Block, prevBlock *types.Header) (root common.Hash, identityRoot common.Hash, diff state.IdentityStateDiff, err error) {
	var totalFee *big.Int
	if totalFee, err = chain.processTxs(appState, block); err != nil {
		return
	}

	chain.applyNewEpoch(appState, block)
	chain.applyBlockRewards(totalFee, appState, block, prevBlock)
	chain.applyGlobalParams(appState, block)

	diff = appState.Precommit()

	return appState.State.Root(), appState.IdentityState.Root(), diff, nil
}

func (chain *Blockchain) applyEmptyBlockOnState(appState *appstate.AppState, block *types.Block) (root common.Hash, identityRoot common.Hash) {

	chain.applyNewEpoch(appState, block)
	chain.applyGlobalParams(appState, block)

	appState.Precommit()

	return appState.State.Root(), appState.IdentityState.Root()
}

func (chain *Blockchain) applyBlockRewards(totalFee *big.Int, appState *appstate.AppState, block *types.Block, prevBlock *types.Header) {

	// calculate fee reward
	burnFee := decimal.NewFromBigInt(totalFee, 0)
	burnFee = burnFee.Mul(decimal.NewFromFloat32(chain.config.Consensus.FeeBurnRate))
	intBurn := math.ToInt(&burnFee)
	intFeeReward := new(big.Int)
	intFeeReward.Sub(totalFee, intBurn)

	// calculate stake
	stake := decimal.NewFromBigInt(chain.config.Consensus.BlockReward, 0)
	stake = stake.Mul(decimal.NewFromFloat32(chain.config.Consensus.StakeRewardRate))
	intStake := math.ToInt(&stake)
	// calculate block reward
	blockReward := big.NewInt(0)
	blockReward = blockReward.Sub(chain.config.Consensus.BlockReward, intStake)

	// calculate total reward
	totalReward := big.NewInt(0).Add(blockReward, intFeeReward)

	// update state
	appState.State.AddBalance(block.Header.ProposedHeader.Coinbase, totalReward)
	appState.State.AddStake(block.Header.ProposedHeader.Coinbase, intStake)

	chain.rewardFinalCommittee(appState, block, prevBlock)
}

func (chain *Blockchain) applyNewEpoch(appState *appstate.AppState, block *types.Block) {

	if !block.Header.Flags().HasFlag(types.ValidationFinished) {
		return
	}

	clearIdentityState(appState)

	networkSize := chain.applyNewEpochFn(appState)

	setNewIdentitiesAttributes(appState, networkSize)

	appState.State.IncEpoch()

	appState.State.SetNextValidationTime(appState.State.NextValidationTime().Add(chain.config.Validation.GetEpochDuration(networkSize)))

	appState.State.SetFlipWordsSeed(block.Seed())
}

func setNewIdentitiesAttributes(appState *appstate.AppState, networkSize int) {
	_, invites, flips := common.NetworkParams(networkSize)
	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {

		s := identity.State

		if !identity.HasDoneAllRequiredFlips() {
			switch identity.State {
			case state.Verified:
				s = state.Suspended
			case state.Newbie:
				s = state.Killed
			default:
				s = state.Killed
			}
		}

		if identity.State == state.Invite {
			s = state.Killed
		}

		switch s {
		case state.Verified:
			appState.State.SetInvites(addr, uint8(invites))
			appState.State.SetRequiredFlips(addr, uint8(flips))
			appState.IdentityState.Add(addr)
		case state.Newbie:
			appState.State.SetRequiredFlips(addr, uint8(flips))
			appState.State.SetInvites(addr, 0)
			appState.IdentityState.Add(addr)
		default:
			appState.State.SetInvites(addr, 0)
			appState.State.SetRequiredFlips(addr, 0)
		}

		appState.State.ClearFlips(addr)

		appState.State.SetState(addr, s)
	})
}

func clearIdentityState(appState *appstate.AppState) {
	appState.IdentityState.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])
		appState.IdentityState.Remove(addr)
		return false
	})
}

func (chain *Blockchain) applyGlobalParams(appState *appstate.AppState, block *types.Block) {

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
	if block.Height()-appState.State.LastSnapshot() >= state.SnapshotBlocksRange && appState.State.ValidationPeriod() == state.NonePeriod &&
		!flags.HasFlag(types.ValidationFinished) {
		appState.State.SetLastSnapshot(block.Height())
	}
}

func (chain *Blockchain) rewardFinalCommittee(appState *appstate.AppState, block *types.Block, prevBlock *types.Header) {
	if block.IsEmpty() {
		return
	}
	identities := appState.ValidatorsCache.GetOnlineValidators(prevBlock.Seed(), prevBlock.Height(), 1000, chain.GetCommitteSize(appState.ValidatorsCache, true))
	if identities == nil || identities.Cardinality() == 0 {
		return
	}
	totalReward := big.NewInt(0)
	totalReward.Div(chain.config.Consensus.FinalCommitteeReward, big.NewInt(int64(identities.Cardinality())))

	stake := decimal.NewFromBigInt(totalReward, 0)
	stake = stake.Mul(decimal.NewFromFloat32(chain.config.Consensus.StakeRewardRate))
	intStake := math.ToInt(&stake)

	reward := big.NewInt(0)
	reward.Sub(totalReward, intStake)

	for _, item := range identities.ToSlice() {
		addr := item.(common.Address)
		appState.State.AddBalance(addr, reward)
		appState.State.AddStake(addr, intStake)
	}
}

func (chain *Blockchain) processTxs(appState *appstate.AppState, block *types.Block) (totalFee *big.Int, err error) {
	totalFee = new(big.Int)
	fee := new(big.Int)
	for i := 0; i < len(block.Body.Transactions); i++ {
		tx := block.Body.Transactions[i]
		if err := validation.ValidateTx(appState, tx, false); err != nil {
			return nil, err
		}
		if fee, err = chain.applyTxOnState(appState, tx); err != nil {
			return nil, err
		}

		totalFee.Add(totalFee, fee)
	}

	return totalFee, nil
}

func (chain *Blockchain) applyTxOnState(appState *appstate.AppState, tx *types.Transaction) (*big.Int, error) {

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
		return nil, errors.New(fmt.Sprintf("invalid tx nonce. Tx=%v exptectedNonce=%v actualNonce=%v", tx.Hash().Hex(),
			currentNonce+1, tx.AccountNonce))
	}

	fee := chain.getTxFee(tx)
	totalCost := chain.getTxCost(tx)

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
		break
	case types.SendTx:
		amount := tx.AmountOrZero()
		stateDB.SubBalance(sender, totalCost)
		stateDB.AddBalance(*tx.To, amount)
		break
	case types.InviteTx:
		if sender != stateDB.GodAddress() {
			stateDB.SubInvite(sender, 1)
		}
		stateDB.SubBalance(sender, totalCost)

		generation, code := stateDB.GeneticCode(sender)

		stateDB.SetState(*tx.To, state.Invite)
		stateDB.AddBalance(*tx.To, new(big.Int).Sub(totalCost, fee))
		stateDB.SetGeneticCode(*tx.To, generation+1, append(code[1:], sender[0]))
		break
	case types.KillTx:
		stateDB.SetState(sender, state.Killed)
		appState.IdentityState.Remove(sender)
		amount := tx.AmountOrZero()
		stateDB.SubBalance(sender, amount)
		stateDB.AddBalance(*tx.To, new(big.Int).Sub(stateDB.GetStakeBalance(sender), fee))
		stateDB.AddBalance(*tx.To, amount)
		break
	case types.SubmitFlipTx:
		stateDB.SubBalance(sender, fee)
		stateDB.AddFlip(sender, tx.Payload)
	case types.OnlineStatusTx:
		stateDB.SubBalance(sender, fee)
		shouldBecomeOnline := len(tx.Payload) > 0 && tx.Payload[0] != 0
		appState.IdentityState.SetOnline(sender, shouldBecomeOnline)
	}

	stateDB.SetNonce(sender, tx.AccountNonce)

	if senderAccount.Epoch() != tx.Epoch {
		stateDB.SetEpoch(sender, tx.Epoch)
	}

	return fee, nil
}

func (chain *Blockchain) getTxFee(tx *types.Transaction) *big.Int {
	return types.CalculateFee(chain.appState.ValidatorsCache.NetworkSize(), tx)
}

func (chain *Blockchain) getTxCost(tx *types.Transaction) *big.Int {
	return types.CalculateCost(chain.appState.ValidatorsCache.NetworkSize(), tx)
}

func getSeedData(prevBlock *types.Header) []byte {
	result := prevBlock.Seed().Bytes()
	result = append(result, common.ToBytes(prevBlock.Height()+1)...)
	return result
}

func (chain *Blockchain) GetProposerSortition() (bool, common.Hash, []byte) {

	if checkIfProposer(chain.coinBaseAddress, chain.appState) {
		return chain.getSortition(chain.getProposerData())
	}

	return false, common.Hash{}, nil
}

func (chain *Blockchain) ProposeBlock() *types.Block {
	head := chain.Head

	txs := chain.txpool.BuildBlockTransactions()
	checkState := chain.appState.Readonly(chain.Head.Height())

	filteredTxs, totalFee := chain.filterTxs(checkState, txs)
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
		Coinbase:       chain.coinBaseAddress,
		IpfsHash:       cidBytes,
	}

	block := &types.Block{
		Header: &types.Header{
			ProposedHeader: header,
		},
		Body: body,
	}

	block.Header.ProposedHeader.BlockSeed, block.Header.ProposedHeader.SeedProof = chain.secStore.VrfEvaluate(getSeedData(head))
	header.Flags = chain.calculateFlags(checkState, block)

	chain.applyNewEpoch(checkState, block)
	chain.applyBlockRewards(totalFee, checkState, block, chain.Head)
	chain.applyGlobalParams(checkState, block)

	checkState.Precommit()

	block.Header.ProposedHeader.Root = checkState.State.Root()
	block.Header.ProposedHeader.IdentityRoot = checkState.IdentityState.Root()

	return block
}

func (chain *Blockchain) calculateFlags(appState *appstate.AppState, block *types.Block) types.BlockFlag {

	var flags types.BlockFlag

	for _, tx := range block.Body.Transactions {
		if tx.Type == types.KillTx || tx.Type == types.OnlineStatusTx {
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

	if block.Height()-appState.State.LastSnapshot() >= state.SnapshotBlocksRange && appState.State.ValidationPeriod() == state.NonePeriod &&
		!flags.HasFlag(types.ValidationFinished) {
		flags |= types.Snapshot
	}

	return flags
}

func (chain *Blockchain) filterTxs(appState *appstate.AppState, txs []*types.Transaction) ([]*types.Transaction, *big.Int) {
	var result []*types.Transaction

	totalFee := new(big.Int)
	for _, tx := range txs {
		if err := validation.ValidateTx(appState, tx, false); err != nil {
			continue
		}
		if fee, err := chain.applyTxOnState(appState, tx); err == nil {
			totalFee.Add(totalFee, fee)
			result = append(result, tx)
		}
	}
	return result, totalFee
}

func (chain *Blockchain) insertHeader(header *types.Header) {
	chain.repo.WriteBlockHeader(header)
	chain.repo.WriteHead(header)
	chain.repo.WriteCanonicalHash(header.Height(), header.Hash())
}

func (chain *Blockchain) insertBlock(block *types.Block, diff state.IdentityStateDiff) error {
	chain.insertHeader(block.Header)
	_, err := chain.ipfs.Add(block.Body.Bytes())

	if !diff.Empty() {
		chain.repo.WriteIdentityStateDiff(block.Height(), diff.Bytes())
	}

	chain.writeTxIndex(block)
	chain.repo.WriteHead(block.Header)

	if err == nil {
		chain.setCurrentHead(block.Header)
	}
	return err
}

func (chain *Blockchain) writeTxIndex(block *types.Block) {
	for i, tx := range block.Body.Transactions {
		idx := &types.TransactionIndex{
			BlockHash: block.Hash(),
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

func (chain *Blockchain) getSortition(data []byte) (bool, common.Hash, []byte) {
	hash, proof := chain.secStore.VrfEvaluate(data)

	v := new(big.Float).SetInt(new(big.Int).SetBytes(hash[:]))

	q := new(big.Float).Quo(v, MaxHash).SetPrec(10)

	if f, _ := q.Float64(); f >= chain.config.Consensus.ProposerTheshold {
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

	if err := validateBlockParentHash(block.Header, prevBlock); err != nil {
		return err
	}
	if err := validateBlockTimestamp(block.Header, prevBlock); err != nil {
		return err
	}
	var seedData = getSeedData(prevBlock)
	pubKey, err := crypto.UnmarshalPubkey(block.Header.ProposedHeader.ProposerPubKey)
	if err != nil {
		return err
	}
	verifier, err := p256.NewVRFVerifier(pubKey)
	if err != nil {
		return err
	}

	hash, err := verifier.ProofToHash(seedData, block.Header.ProposedHeader.SeedProof)
	if err != nil {
		return err
	}
	if hash != block.Seed() {
		return errors.New("seed is invalid")
	}

	proposerAddr, _ := crypto.PubKeyBytesToAddress(block.Header.ProposedHeader.ProposerPubKey)

	if !checkIfProposer(proposerAddr, checkState) {
		return errors.New("proposer is not identity")
	}

	var txs = types.Transactions(block.Body.Transactions)

	if types.DeriveSha(txs) != block.Header.ProposedHeader.TxHash {
		return errors.New("txHash is invalid")
	}

	if chain.calculateFlags(checkState, block) != block.Header.ProposedHeader.Flags {
		return errors.New("flags are invalid")
	}

	if root, identityRoot, _, err := chain.applyBlockOnState(checkState, block, prevBlock); err != nil {
		return err
	} else if root != block.Root() || identityRoot != block.IdentityRoot() {
		return errors.Errorf("invalid block roots. Exptected=%x & %x, actual=%x & %x", root, identityRoot, block.Root(), block.IdentityRoot())
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

func (chain *Blockchain) ValidateBlockCert(prevBlock *types.Header, block *types.Header, cert types.BlockCert, validatorsCache *validators.ValidatorsCache) (err error) {

	step := cert[0].Header.Step
	validators := validatorsCache.GetOnlineValidators(prevBlock.Seed(), block.Height(), step, chain.GetCommitteSize(validatorsCache, step == types.Final))

	voters := mapset.NewSet()

	for _, vote := range cert {
		if !validators.Contains(vote.VoterAddr()) {
			return errors.New("invalid voter")
		}
		if vote.Header.Step != step || vote.Header.Round != block.Height() {
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

	if voters.Cardinality() < chain.GetCommitteeVotesTreshold(validatorsCache, step == types.Final) {
		return errors.New("not enough votes")
	}
	return nil
}

func (chain *Blockchain) ValidateBlock(block *types.Block, checkState *appstate.AppState) error {
	if checkState == nil {
		checkState = chain.appState.Readonly(chain.Head.Height())
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

	if f, _ := q.Float64(); f < chain.config.Consensus.ProposerTheshold {
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
	if bodyBytes, err := chain.ipfs.Get(header.ProposedHeader.IpfsHash); err != nil {
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

func (chain *Blockchain) GetTx(hash common.Hash) (*types.Transaction, *types.TransactionIndex) {
	idx := chain.repo.ReadTxIndex(hash)
	if idx == nil {
		return nil, nil
	}
	header := chain.repo.ReadBlockHeader(idx.BlockHash)
	if header == nil || header.ProposedHeader == nil {
		return nil, nil
	}

	data, err := chain.ipfs.Get(header.ProposedHeader.IpfsHash)
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

func (chain *Blockchain) GetCommitteSize(vc *validators.ValidatorsCache, final bool) int {
	var cnt = vc.OnlineSize()
	percent := chain.config.Consensus.CommitteePercent
	if final {
		percent = chain.config.Consensus.FinalCommitteeConsensusPercent
	}
	if cnt <= 8 {
		return cnt
	}
	return int(float64(cnt) * percent)
}

func (chain *Blockchain) GetCommitteeVotesTreshold(vc *validators.ValidatorsCache, final bool) int {

	var cnt = vc.OnlineSize()
	percent := chain.config.Consensus.CommitteePercent
	if final {
		percent = chain.config.Consensus.FinalCommitteeConsensusPercent
	}

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
	return int(float64(cnt) * percent * chain.config.Consensus.ThesholdBa)
}

func (chain *Blockchain) Genesis() common.Hash {
	return chain.genesis.Hash()
}

func (chain *Blockchain) ValidateSubChain(startHeight uint64, blocks []*types.Block) error {
	checkState, err := chain.appState.ForCheckWithNewCache(startHeight)
	if err != nil {
		return err
	}
	prevBlock := chain.GetBlockHeaderByHeight(startHeight)

	for _, b := range blocks {
		if err := chain.validateBlock(checkState, b, prevBlock); err != nil {
			return err
		}
		if err := checkState.Commit(b); err != nil {
			return err
		}
		prevBlock = b.Header
	}

	return nil
}

func (chain *Blockchain) ResetTo(height uint64) error {
	if err := chain.appState.ResetTo(height); err != nil {
		return errors.WithMessage(err, "state is corrupted, try to resync from scratch")
	}
	chain.setHead(height)
	return nil
}

func (chain *Blockchain) EnsureIntegrity() error {
	wasReset := false
	for chain.Head.Root() != chain.appState.State.Root() ||
		chain.Head.IdentityRoot() != chain.appState.IdentityState.Root() {
		wasReset = true
		if err := chain.ResetTo(chain.Head.Height() - 1); err != nil {
			return err
		}
	}
	if wasReset {
		chain.log.Warn("blockchain was reseted", "new head", chain.Head.Height())
	}
	return nil
}
func (chain *Blockchain) StartSync() {
	chain.isSyncing = true
	chain.txpool.StartSync()
}

func (chain *Blockchain) StopSync() {
	chain.isSyncing = false
	chain.txpool.StopSync(chain.GetBlock(chain.Head.Hash()))
}

func checkIfProposer(addr common.Address, appState *appstate.AppState) bool {
	return appState.ValidatorsCache.Contains(addr) ||
		appState.State.GodAddress() == addr && appState.ValidatorsCache.OnlineSize() == 0
}

func (chain *Blockchain) AddHeader(header *types.Header) error {

	if err := chain.ValidateHeader(header, chain.PreliminaryHead); err != nil {
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

func (chain *Blockchain) GetCertificate(hash common.Hash) types.BlockCert {
	return chain.repo.ReadCertificate(hash)
}

func (chain *Blockchain) GetIdentityDiff(height uint64) state.IdentityStateDiff {

	data := chain.repo.ReadIdentityStateDiff(height)
	if data == nil {
		return nil
	}
	diff := state.IdentityStateDiff{}
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

func (chain *Blockchain) RemovePreliminaryHead() {
	chain.PreliminaryHead = nil
	chain.repo.RemovePreliminaryHead()
}
