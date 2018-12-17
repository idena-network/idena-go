package blockchain

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/blockchain/validation"
	"idena-go/common"
	"idena-go/config"
	"idena-go/core/appstate"
	"idena-go/core/mempool"
	"idena-go/core/state"
	"idena-go/crypto"
	"idena-go/crypto/vrf"
	"idena-go/crypto/vrf/p256"
	"idena-go/log"
	"math/big"
	"time"
)

const (
	Mainnet types.Network = 0x1
	Testnet types.Network = 0x2
)

const (
	ProposerRole uint8 = 0x1
)

var (
	MaxHash *big.Float
)

type Blockchain struct {
	repo *repo

	Head            *types.Block
	config          *config.Config
	vrfSigner       vrf.PrivateKey
	pubKey          *ecdsa.PublicKey
	coinBaseAddress common.Address
	log             log.Logger
	txpool          *mempool.TxPool
	appState        *appstate.AppState
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

func NewBlockchain(config *config.Config, db dbm.DB, txpool *mempool.TxPool, appState *appstate.AppState) *Blockchain {
	return &Blockchain{
		repo:     NewRepo(db),
		config:   config,
		log:      log.New(),
		txpool:   txpool,
		appState: appState,
	}
}

func (chain *Blockchain) GetHead() *types.Block {
	head := chain.repo.ReadHead()
	if head == nil {
		return nil
	}
	return chain.repo.ReadBlock(head.Hash())
}

func (chain *Blockchain) InitializeChain(secretKey *ecdsa.PrivateKey) error {
	signer, err := p256.NewVRFSigner(secretKey)
	if err != nil {
		return err
	}
	chain.vrfSigner = signer

	chain.pubKey = secretKey.Public().(*ecdsa.PublicKey)
	chain.coinBaseAddress = crypto.PubkeyToAddress(*chain.pubKey)
	head := chain.GetHead()
	if head != nil {
		chain.SetCurrentHead(head)
	} else {
		chain.GenerateGenesis(chain.config.Network)
	}
	log.Info("Chain initialized", "block", chain.Head.Hash().Hex(), "height", chain.Head.Height())
	return nil
}

func (chain *Blockchain) SetCurrentHead(block *types.Block) {
	chain.Head = block
}

func (chain *Blockchain) GenerateGenesis(network types.Network) *types.Block {

	chain.appState.State.Commit(true)

	root := chain.appState.State.Root()

	var emptyHash [32]byte
	seed := types.Seed(crypto.Keccak256Hash(append([]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6}, common.ToBytes(network)...)))
	block := types.Block{Header: &types.Header{
		ProposedHeader: &types.ProposedHeader{
			ParentHash: emptyHash,
			Time:       big.NewInt(0),
			Height:     1,
			Root:       root,
		},
	}, Body: &types.Body{
		BlockSeed: seed,
	}}

	chain.insertBlock(&block)
	return &block
}

func (chain *Blockchain) GetBlockByHeight(height uint64) *types.Block {
	hash := chain.repo.ReadCanonicalHash(height)
	if hash == (common.Hash{}) {
		return nil
	}
	return chain.repo.ReadBlock(hash)
}

func (chain *Blockchain) GenerateEmptyBlock() *types.Block {
	head := chain.Head
	block := &types.Block{
		Header: &types.Header{
			EmptyBlockHeader: &types.EmptyBlockHeader{
				ParentHash: head.Hash(),
				Height:     head.Height() + 1,
				Root:       chain.appState.State.Root(),
			},
		},
		Body: &types.Body{
			Transactions: []*types.Transaction{},
		},
	}
	block.Body.BlockSeed = types.Seed(crypto.Keccak256Hash(chain.GetSeedData(block)))
	return block
}

func (chain *Blockchain) AddBlock(block *types.Block) error {

	if err := chain.validateBlockParentHash(block); err != nil {
		return err
	}
	if block.IsEmpty() {
		if err := chain.applyBlock(chain.appState.State, block); err != nil {
			return err
		}
		chain.insertBlock(chain.GenerateEmptyBlock())
	} else {
		if err := chain.ValidateProposedBlock(block); err != nil {
			return err
		}
		if err := chain.applyBlock(chain.appState.State, block); err != nil {
			return err
		}
		chain.insertBlock(block)
	}
	return nil
}

func (chain *Blockchain) applyBlock(state *state.StateDB, block *types.Block) error {
	if !block.IsEmpty() {
		if root, err := chain.applyAndValidateBlockState(state, block); err != nil {
			state.Reset()
			return err
		} else if root != block.Root() {
			state.Reset()
			return errors.New(fmt.Sprintf("Invalid block root. Exptected=%x, blockroot=%x", root, block.Root()))
		}
	}
	hash, version, _ := state.Commit(true)
	chain.log.Info("Applied block", "root", fmt.Sprintf("0x%x", hash), "version", version, "blockroot", block.Root())
	chain.txpool.ResetTo(block)
	chain.appState.ValidatorsCache.RefreshIfUpdated(block.Body.Transactions)
	return nil
}

func (chain *Blockchain) applyAndValidateBlockState(state *state.StateDB, block *types.Block) (common.Hash, error) {
	var totalFee *big.Int
	var err error
	if totalFee, err = chain.processTxs(state, block); err != nil {
		return common.Hash{}, err
	}

	feeReward := new(big.Float).SetInt(totalFee)
	feeReward = feeReward.Mul(feeReward, big.NewFloat(chain.config.Consensus.FeeBurnRate))
	intFeeReward, _ := feeReward.Int(nil)

	stake := big.NewFloat(0).SetInt(chain.config.Consensus.BlockReward)
	stake = stake.Mul(stake, big.NewFloat(chain.config.Consensus.StakeRewardRate))
	intStake, _ := stake.Int(nil)

	blockReward := big.NewInt(0)
	blockReward = blockReward.Sub(chain.config.Consensus.BlockReward, intStake)

	totalReward := big.NewInt(0).Add(blockReward, intFeeReward)

	state.AddBalance(block.Header.ProposedHeader.Coinbase, totalReward)

	state.AddStake(block.Header.ProposedHeader.Coinbase, intStake)

	chain.rewardFinalCommittee(state, block)

	state.Precommit(true)
	return state.Root(), nil
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

	stake := big.NewFloat(0).SetInt(totalReward)
	stake.Mul(stake, big.NewFloat(chain.config.Consensus.StakeRewardRate))

	intStake, _ := stake.Int(nil)
	reward := big.NewInt(0)
	reward.Sub(totalReward, intStake)

	for _, item := range identities.ToSlice() {
		addr := item.(common.Address)
		state.AddBalance(addr, reward)
		state.AddStake(addr, intStake)
	}
}

func (chain *Blockchain) processTxs(state *state.StateDB, block *types.Block) (*big.Int, error) {
	totalFee := new(big.Int)
	for i := 0; i < len(block.Body.Transactions); i++ {
		tx := block.Body.Transactions[i]
		sender, _ := types.Sender(tx)

		if expected := state.GetNonce(sender) + 1; expected != tx.AccountNonce {
			return nil, errors.New(fmt.Sprintf("Invalid tx nonce. Tx=%v exptectedNonce=%v actualNonce=%v", tx.Hash().Hex(),
				expected, tx.AccountNonce))
		}
		fee := chain.getTxFee(tx)

		switch tx.Type {
		case types.ApprovingTx:
			state.GetOrNewIdentityObject(sender).Approve()
			break
		case types.SendTx:

			balance := state.GetBalance(sender)
			amount := tx.Amount
			change := new(big.Int).Sub(new(big.Int).Sub(balance, amount), fee)
			if change.Sign() < 0 {
				return nil, errors.New("Not enough funds")
			}
			state.SubBalance(sender, amount)
			state.SubBalance(sender, fee)

			state.AddBalance(*tx.To, amount)

			totalFee = new(big.Int).Add(totalFee, fee)
			break
		case types.SendInviteTx:
			break
		case types.RevokeTx:
			state.GetOrNewIdentityObject(sender).Revoke()
		}

		state.SetNonce(sender, tx.AccountNonce+1)
	}

	return totalFee, nil
}

func (chain *Blockchain) getTxFee(tx *types.Transaction) *big.Int {
	return types.CalculateFee(chain.appState.ValidatorsCache.GetCountOfValidNodes(), tx)
}

func (chain *Blockchain) GetSeedData(proposalBlock *types.Block) []byte {
	head := chain.Head
	result := head.Seed().Bytes()
	result = append(result, common.ToBytes(proposalBlock.Height())...)
	result = append(result, proposalBlock.Hash().Bytes()...)
	return result
}

func (chain *Blockchain) GetProposerSortition() (bool, common.Hash, []byte) {
	return chain.getSortition(chain.getProposerData())
}

func (chain *Blockchain) ProposeBlock(hash common.Hash, proof []byte) (*types.Block, error) {
	head := chain.Head

	txs := chain.txpool.BuildBlockTransactions()

	header := &types.ProposedHeader{
		Height:         head.Height() + 1,
		ParentHash:     head.Hash(),
		Time:           new(big.Int).SetInt64(time.Now().UTC().Unix()),
		ProposerPubKey: crypto.FromECDSAPub(chain.pubKey),
		TxHash:         types.DeriveSha(types.Transactions(txs)),
		Coinbase:       chain.coinBaseAddress,
	}

	block := &types.Block{
		Header: &types.Header{
			ProposedHeader: header,
		},
		Body: &types.Body{
			Transactions: txs,
		},
	}
	checkState := state.NewForCheck(chain.appState.State)
	if root, err := chain.applyAndValidateBlockState(checkState, block); err != nil {
		return nil, err
	} else {
		block.Header.ProposedHeader.Root = root
	}

	block.Body.BlockSeed, block.Body.SeedProof = chain.vrfSigner.Evaluate(chain.GetSeedData(block))
	return block, nil
}

func (chain *Blockchain) insertBlock(block *types.Block) {
	chain.repo.WriteBlock(block)
	chain.repo.WriteHead(block.Header)
	chain.repo.WriteCanonicalHash(block.Height(), block.Hash())
	chain.SetCurrentHead(block)
}

func (chain *Blockchain) getProposerData() []byte {
	head := chain.Head
	result := head.Seed().Bytes()
	result = append(result, common.ToBytes(ProposerRole)...)
	result = append(result, common.ToBytes(head.Height()+1)...)
	return result
}

func (chain *Blockchain) getSortition(data []byte) (bool, common.Hash, []byte) {
	hash, proof := chain.vrfSigner.Evaluate(data)

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
	var seedData = chain.GetSeedData(block)
	pubKey, err := crypto.UnmarshalPubkey(block.Header.ProposedHeader.ProposerPubKey)
	if err != nil {
		return err
	}
	verifier, err := p256.NewVRFVerifier(pubKey)
	if err != nil {
		return err
	}

	hash, err := verifier.ProofToHash(seedData, block.Body.SeedProof)
	if err != nil {
		return err
	}
	if hash != block.Seed() || len(block.Seed()) == 0 {
		return errors.New("Seed is invalid")
	}

	proposerAddr, _ := crypto.PubKeyBytesToAddress(block.Header.ProposedHeader.ProposerPubKey)
	if chain.appState.ValidatorsCache.GetCountOfValidNodes() > 0 &&
		!chain.appState.ValidatorsCache.Contains(proposerAddr) {
		return errors.New("Proposer is not identity")
	}

	var txs = types.Transactions(block.Body.Transactions)

	if types.DeriveSha(txs) != block.Header.ProposedHeader.TxHash {
		return errors.New("TxHash is invalid")
	}

	for i := 0; i < len(block.Body.Transactions); i++ {
		tx := block.Body.Transactions[i]

		if err := validation.ValidateTx(chain.appState, tx); err != nil {
			return err
		}
	}
	checkState := state.NewForCheck(chain.appState.State)
	if root, err := chain.applyAndValidateBlockState(checkState, block); err != nil {
		return err
	} else if root != block.Root() {
		return errors.New(fmt.Sprintf("Invalid block root. Exptected=%x, blockroot=%x", root, block.Root()))
	}
	return nil
}

func (chain *Blockchain) validateBlockParentHash(block *types.Block) error {
	head := chain.Head
	if head.Height() != (block.Height() - 1) {
		return errors.New("Height is invalid")
	}
	if head.Hash() != block.Header.ParentHash() {
		return errors.New("ParentHash is invalid")
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
	if chain.appState.ValidatorsCache.GetCountOfValidNodes() > 0 &&
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
	return chain.repo.ReadBlock(hash)
}

func (chain *Blockchain) GetCommitteSize(final bool) int {
	var cnt = chain.appState.ValidatorsCache.GetCountOfValidNodes()
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

	var cnt = chain.appState.ValidatorsCache.GetCountOfValidNodes()
	percent := chain.config.Consensus.CommitteePercent
	if final {
		percent = chain.config.Consensus.FinalCommitteeConsensusPercent
	}

	switch cnt {
	case 1, 2:
		return 1
	case 3, 4:
		return 2
	case 5, 6:
		return 3
	case 7, 8:
		return 4
	}
	return int(float64(cnt) * percent * chain.config.Consensus.ThesholdBa)
}
