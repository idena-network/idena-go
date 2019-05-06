package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	cid2 "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/blockchain/validation"
	"idena-go/common"
	"idena-go/common/eventbus"
	"idena-go/common/math"
	"idena-go/config"
	"idena-go/core/appstate"
	"idena-go/core/mempool"
	"idena-go/core/state"
	"idena-go/crypto"
	"idena-go/crypto/vrf/p256"
	"idena-go/database"
	"idena-go/events"
	"idena-go/ipfs"
	"idena-go/log"
	"idena-go/secstore"
	math2 "math"
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
	MaxHash *big.Float
)

type Blockchain struct {
	repo            *database.Repo
	secStore        *secstore.SecStore
	Head            *types.Header
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
		chain.SetCurrentHead(head)
		if chain.genesis = chain.GetBlockHeaderByHeight(1); chain.genesis == nil {
			return errors.New("genesis block is not found")
		}
	} else {
		chain.GenerateGenesis(chain.config.Network)
	}

	log.Info("Chain initialized", "block", chain.Head.Hash().Hex(), "height", chain.Head.Height())
	log.Info("Coinbase address", "addr", chain.coinBaseAddress.Hex())
	return nil
}

func (chain *Blockchain) SetCurrentHead(head *types.Header) {
	chain.Head = head
}

func (chain *Blockchain) SetHead(height uint64) {
	chain.repo.SetHead(height)
	chain.SetCurrentHead(chain.GetHead())
}

func (chain *Blockchain) GenerateGenesis(network types.Network) (*types.Block, error) {

	for addr, alloc := range chain.config.GenesisConf.Alloc {
		if alloc.Balance != nil {
			chain.appState.State.SetBalance(addr, alloc.Balance)
		}
		if alloc.Stake != nil {
			chain.appState.State.AddStake(addr, alloc.Stake)
		}
		chain.appState.State.SetState(addr, alloc.State)
		if alloc.State == state.Verified {
			chain.appState.IdentityState.Add(addr)
		}
	}

	chain.appState.State.SetGodAddress(chain.config.GenesisConf.GodAddress)

	nextValidationTimestamp := chain.config.GenesisConf.FirstCeremonyTime
	if nextValidationTimestamp == 0 {
		nextValidationTimestamp = time.Now().UTC().Unix()
	}
	chain.appState.State.SetNextValidationTime(time.Unix(nextValidationTimestamp, 0))

	log.Info("Next validation time", "time", chain.appState.State.NextValidationTime().String(), "unix", nextValidationTimestamp)

	if err := chain.appState.Commit(); err != nil {
		return nil, err
	}

	var emptyHash [32]byte
	seed := types.Seed(crypto.Keccak256Hash(append([]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6}, common.ToBytes(network)...)))
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

	if err := chain.insertBlock(block); err != nil {
		return nil, err
	}
	chain.genesis = block.Header
	return block, nil
}

func (chain *Blockchain) GenerateEmptyBlock() *types.Block {
	head := chain.Head
	prevTimestamp := time.Unix(chain.Head.Time().Int64(), 0)

	checkState := chain.appState.ForCheck(chain.Head.Height())

	block := &types.Block{
		Header: &types.Header{
			EmptyBlockHeader: &types.EmptyBlockHeader{
				ParentHash:   head.Hash(),
				Height:       head.Height() + 1,
				Root:         chain.appState.State.Root(),
				IdentityRoot: chain.Head.IdentityRoot(),
				Time:         new(big.Int).SetInt64(prevTimestamp.Add(EmptyBlockTimeIncrement).Unix()),
			},
		},
		Body: &types.Body{},
	}

	block.Header.EmptyBlockHeader.Flags = chain.calculateFlags(block)

	chain.applyNewEpoch(checkState, block)
	chain.applyGlobalParams(checkState, block)

	checkState.Precommit()

	block.Header.EmptyBlockHeader.Root = checkState.State.Root()
	block.Header.EmptyBlockHeader.IdentityRoot = checkState.IdentityState.Root()
	block.Header.EmptyBlockHeader.BlockSeed = types.Seed(crypto.Keccak256Hash(chain.GetSeedData(block)))
	return block
}

func (chain *Blockchain) AddBlock(block *types.Block) error {

	if err := chain.validateBlockParentHash(block); err != nil {
		return err
	}
	if !block.IsEmpty() {
		if err := chain.ValidateProposedBlock(block); err != nil {
			return err
		}
	}
	if err := chain.processBlock(block); err != nil {
		return err
	}
	if err := chain.insertBlock(block); err != nil {
		return err
	}

	chain.bus.Publish(&events.NewBlockEvent{
		Block: block,
	})

	return nil
}

func (chain *Blockchain) processBlock(block *types.Block) (err error) {
	var root, identityRoot common.Hash

	if block.IsEmpty() {
		root, identityRoot = chain.applyEmptyBlockOnState(chain.appState, block)
	} else {
		if root, identityRoot, err = chain.applyBlockOnState(chain.appState, block); err != nil {
			chain.appState.Reset()
			return err
		}
	}

	if root != block.Root() || identityRoot != block.IdentityRoot() {
		chain.appState.Reset()
		return errors.Errorf("Invalid block root. Exptected=%x, blockroot=%x", root, block.Root())
	}

	if err := chain.appState.Commit(); err != nil {
		return err
	}
	chain.log.Trace("Applied block", "root", fmt.Sprintf("0x%x", block.Root()), "height", block.Height())
	chain.txpool.ResetTo(block)
	chain.appState.ValidatorsCache.RefreshIfUpdated(block.Header.Flags().HasFlag(types.IdentityUpdate))
	return nil
}

func (chain *Blockchain) applyBlockOnState(appState *appstate.AppState, block *types.Block) (root common.Hash, identityRoot common.Hash, err error) {
	var totalFee *big.Int
	if totalFee, err = chain.processTxs(appState, block); err != nil {
		return
	}

	chain.applyNewEpoch(appState, block)
	chain.applyBlockRewards(totalFee, appState, block)
	chain.applyGlobalParams(appState, block)

	appState.Precommit()

	return appState.State.Root(), appState.IdentityState.Root(), nil
}

func (chain *Blockchain) applyEmptyBlockOnState(appState *appstate.AppState, block *types.Block) (root common.Hash, identityRoot common.Hash) {

	chain.applyNewEpoch(appState, block)
	chain.applyGlobalParams(appState, block)

	appState.Precommit()

	return appState.State.Root(), appState.IdentityState.Root()
}

func (chain *Blockchain) applyBlockRewards(totalFee *big.Int, appState *appstate.AppState, block *types.Block) {

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

	chain.rewardFinalCommittee(appState.State, block)
}

func (chain *Blockchain) applyNewEpoch(appState *appstate.AppState, block *types.Block) {

	if !block.Header.Flags().HasFlag(types.ValidationFinished) {
		return
	}

	clearIdentityState(appState)

	networkSize := chain.applyNewEpochFn(appState)

	setNewIdentitiesAttributes(appState, networkSize)

	appState.State.ClearFlips()
	appState.State.IncEpoch()

	appState.State.SetNextValidationTime(appState.State.NextValidationTime().Add(chain.config.Validation.ValidationInterval))
}

func networkParams(networkSize int) (epochDuration int, invites int, flips int) {

	epochDurationF := math2.Round(math2.Pow(float64(networkSize), 0.33))
	epochDuration = int(epochDurationF)

	if networkSize <= 1 {
		return epochDuration, 0, 0
	}

	invitesCount := 20 / math2.Pow(math2.Log10(float64(networkSize)), 2)
	if invitesCount < 1 {
		flips = int(math2.Round((1 + invitesCount) * 5))
	} else {
		flips = int(math2.Round(epochDurationF * 2.0 / 7.0))
	}
	invites = int(math2.Round(invitesCount))
	return
}

func setNewIdentitiesAttributes(appState *appstate.AppState, networkSize int) {

	_, invites, flips := networkParams(networkSize)

	appState.State.IterateOverIdentities(func(addr common.Address, identity state.Identity) {

		s := identity.State

		if identity.RequiredFlips > 0 {
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
}

func (chain *Blockchain) rewardFinalCommittee(state *state.StateDB, block *types.Block) {
	if block.IsEmpty() {
		return
	}
	identities := chain.appState.ValidatorsCache.GetActualValidators(chain.Head.Seed(), chain.Head.Height(), 1000, chain.GetCommitteSize(true))
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
		state.AddBalance(addr, reward)
		state.AddStake(addr, intStake)
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
		senderIdentity := stateDB.GetOrNewIdentityObject(sender)

		balance := stateDB.GetBalance(sender)
		change := new(big.Int).Sub(balance, totalCost)

		// zero balance and kill temp identity
		stateDB.SetBalance(sender, big.NewInt(0))
		senderIdentity.SetState(state.Killed)

		// verify identity and add transfer all available funds from temp account
		recipient := *tx.To
		stateDB.GetOrNewIdentityObject(recipient).SetState(state.Candidate)
		stateDB.AddBalance(recipient, change)
		stateDB.SetPubKey(recipient, tx.Payload)
		break
	case types.RegularTx:
		amount := tx.AmountOrZero()
		stateDB.SubBalance(sender, totalCost)
		stateDB.AddBalance(*tx.To, amount)
		break
	case types.InviteTx:
		if sender != stateDB.GodAddress() {
			stateDB.SubInvite(sender, 1)
		}
		stateDB.SubBalance(sender, totalCost)

		stateDB.GetOrNewIdentityObject(*tx.To).SetState(state.Invite)
		stateDB.AddBalance(*tx.To, new(big.Int).Sub(totalCost, fee))
		break
	case types.KillTx:
		stateDB.GetOrNewIdentityObject(sender).SetState(state.Killed)
		appState.IdentityState.Remove(sender)
		break
	case types.SubmitFlipTx:
		stateDB.AddFlip(tx.Payload)
		stateDB.SubBalance(sender, totalCost)
		if sender != stateDB.GodAddress() {
			stateDB.SubRequiredFlips(sender, 1)
		}
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

func (chain *Blockchain) GetSeedData(proposalBlock *types.Block) []byte {
	head := chain.Head
	result := head.Seed().Bytes()
	result = append(result, common.ToBytes(proposalBlock.Height())...)
	return result
}

func (chain *Blockchain) GetProposerSortition() (bool, common.Hash, []byte) {

	// only validated nodes can propose block
	if !chain.appState.ValidatorsCache.Contains(chain.coinBaseAddress) && chain.appState.ValidatorsCache.NetworkSize() > 0 {
		return false, common.Hash{}, nil
	}

	return chain.getSortition(chain.getProposerData())
}

func (chain *Blockchain) ProposeBlock() *types.Block {
	head := chain.Head

	txs := chain.txpool.BuildBlockTransactions()
	checkState := chain.appState.ForCheck(chain.Head.Height())

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

	header := &types.ProposedHeader{
		Height:         head.Height() + 1,
		ParentHash:     head.Hash(),
		Time:           new(big.Int).SetInt64(newBlockTime),
		ProposerPubKey: chain.pubKey,
		TxHash:         types.DeriveSha(types.Transactions(filteredTxs)),
		Coinbase:       chain.coinBaseAddress,
		IpfsHash:       cid.Bytes(),
	}

	block := &types.Block{
		Header: &types.Header{
			ProposedHeader: header,
		},
		Body: body,
	}

	header.Flags = chain.calculateFlags(block)

	chain.applyNewEpoch(checkState, block)
	chain.applyBlockRewards(totalFee, checkState, block)
	chain.applyGlobalParams(checkState, block)

	checkState.Precommit()

	block.Header.ProposedHeader.Root = checkState.State.Root()
	block.Header.ProposedHeader.IdentityRoot = checkState.IdentityState.Root()
	block.Header.ProposedHeader.BlockSeed, block.Header.ProposedHeader.SeedProof = chain.secStore.VrfEvaluate(chain.GetSeedData(block))

	return block
}

func (chain *Blockchain) calculateFlags(block *types.Block) types.BlockFlag {

	var flags types.BlockFlag

	for _, tx := range block.Body.Transactions {
		if tx.Type == types.KillTx {
			flags |= types.IdentityUpdate
		}
	}

	appState := chain.appState.State

	if appState.ValidationPeriod() == state.NonePeriod &&
		chain.timing.isFlipLotteryStarted(appState.NextValidationTime(), block.Header.Time()) {
		flags |= types.FlipLotteryStarted
	}

	if appState.ValidationPeriod() == state.FlipLotteryPeriod &&
		chain.timing.isShortSessionStarted(appState.NextValidationTime(), block.Header.Time()) {
		flags |= types.ShortSessionStarted
	}

	if appState.ValidationPeriod() == state.ShortSessionPeriod &&
		chain.timing.isLongSessionStarted(appState.NextValidationTime(), block.Header.Time()) {
		flags |= types.LongSessionStarted
	}

	if appState.ValidationPeriod() == state.LongSessionPeriod &&
		chain.timing.isAfterLongSessionStarted(appState.NextValidationTime(), block.Header.Time()) {
		flags |= types.AfterLongSessionStarted
	}

	if appState.ValidationPeriod() == state.AfterLongSessionPeriod &&
		chain.timing.isValidationFinished(appState.NextValidationTime(), block.Header.Time()) {
		flags |= types.ValidationFinished
		flags |= types.IdentityUpdate
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

func (chain *Blockchain) insertBlock(block *types.Block) error {
	chain.repo.WriteBlockHeader(block)
	chain.repo.WriteCanonicalHash(block.Height(), block.Hash())
	_, err := chain.ipfs.Add(block.Body.Bytes())
	chain.writeTxIndex(block)
	chain.repo.WriteHead(block.Header)

	if err == nil {
		chain.SetCurrentHead(block.Header)
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

func (chain *Blockchain) ValidateProposedBlock(block *types.Block) error {

	if err := chain.validateBlockParentHash(block); err != nil {
		return err
	}
	if err := chain.validateBlockTimestamp(block); err != nil {
		return err
	}
	var seedData = chain.GetSeedData(block)
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
	if chain.appState.ValidatorsCache.NetworkSize() > 0 &&
		!chain.appState.ValidatorsCache.Contains(proposerAddr) {
		return errors.New("proposer is not identity")
	}

	var txs = types.Transactions(block.Body.Transactions)

	if types.DeriveSha(txs) != block.Header.ProposedHeader.TxHash {
		return errors.New("txHash is invalid")
	}

	if chain.calculateFlags(block) != block.Header.ProposedHeader.Flags {
		return errors.New("flags are invalid")
	}

	checkState := chain.appState.ForCheck(chain.Head.Height())

	if root, identityRoot, err := chain.applyBlockOnState(checkState, block); err != nil {
		return err
	} else if root != block.Root() || identityRoot != block.IdentityRoot() {
		return errors.Errorf("Invalid block roots. Exptected=%x & %x, actual=%x & %x", root, identityRoot, block.Root(), block.IdentityRoot())
	}

	cid, _ := chain.ipfs.Cid(block.Body.Bytes())

	if bytes.Compare(cid.Bytes(), block.Header.ProposedHeader.IpfsHash) != 0 {
		return errors.New("invalid block cid")
	}

	return nil
}

func (chain *Blockchain) validateBlockParentHash(block *types.Block) error {
	head := chain.Head
	if head.Height()+1 != (block.Height()) {
		return errors.New(fmt.Sprintf("Height is invalid. Expected=%v but received=%v", head.Height()+1, block.Height()))
	}
	if head.Hash() != block.Header.ParentHash() {
		return errors.New("parentHash is invalid")
	}
	return nil
}

func (chain *Blockchain) validateBlockTimestamp(block *types.Block) error {
	blockTime := time.Unix(block.Header.Time().Int64(), 0)

	if blockTime.Sub(time.Now().UTC()) > MaxFutureBlockOffset {
		return errors.New("block from future")
	}
	prevBlockTime := time.Unix(chain.Head.Time().Int64(), 0)

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
	if chain.appState.ValidatorsCache.NetworkSize() > 0 &&
		!chain.appState.ValidatorsCache.Contains(proposerAddr) {
		return errors.New("Proposer is not identity")
	}

	return nil
}

func (chain *Blockchain) Round() uint64 {
	return chain.Head.Height() + 1
}
func (chain *Blockchain) WriteFinalConsensus(hash common.Hash, cert *types.BlockCert) {
	chain.repo.WriteFinalConsensus(hash)
	chain.repo.WriteCert(hash, cert)
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

func (chain *Blockchain) GetCommitteSize(final bool) int {
	var cnt = chain.appState.ValidatorsCache.NetworkSize()
	percent := chain.config.Consensus.CommitteePercent
	if final {
		percent = chain.config.Consensus.FinalCommitteeConsensusPercent
	}
	if cnt <= 8 {
		return cnt
	}
	return int(float64(cnt) * percent)
}

func (chain *Blockchain) GetCommitteeVotesTreshold(final bool) int {

	var cnt = chain.appState.ValidatorsCache.NetworkSize()
	percent := chain.config.Consensus.CommitteePercent
	if final {
		percent = chain.config.Consensus.FinalCommitteeConsensusPercent
	}

	switch cnt {
	case 1:
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
