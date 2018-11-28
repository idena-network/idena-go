package blockchain

import (
	"crypto/ecdsa"
	"errors"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/blockchain/types"
	"idena-go/blockchain/validation"
	"idena-go/common"
	"idena-go/config"
	"idena-go/core/appstate"
	"idena-go/core/mempool"
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

	Head      *types.Block
	config    *config.Config
	vrfSigner vrf.PrivateKey
	pubKey    *ecdsa.PublicKey
	log       log.Logger
	txpool    *mempool.TxPool
	appState  *appstate.AppState
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

	head := chain.GetHead()
	if head != nil {
		chain.SetCurrentHead(head)
	} else {
		chain.GenerateGenesis(chain.config.Network)
	}
	log.Info("Chain initialized", "block", chain.Head.Hash().Hex())
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

			Time:   big.NewInt(0),
			Height: 1,
			Root:   root,
		},
	}, Body: &types.Body{
		BlockSeed: seed,
	},}

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
		chain.insertBlock(chain.GenerateEmptyBlock())
	} else {
		if err := chain.ValidateProposedBlock(block); err != nil {
			return err
		}

		chain.processTxs(block)

		chain.insertBlock(block)
	}
	return nil
}

func (chain *Blockchain) processTxs(block *types.Block) {
	for i := 0; i < len(block.Body.Transactions); i++ {
		tx := block.Body.Transactions[i]

		if tx.To == nil {
			chain.appState.ValidatorsState.AddValidator(tx.Sender())
		}

		chain.txpool.Remove(tx)
	}
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

func (chain *Blockchain) ProposeBlock(hash common.Hash, proof []byte) *types.Block {
	head := chain.Head

	txs := chain.txpool.GetPendingTransaction()

	header := &types.ProposedHeader{
		Height:         head.Height() + 1,
		ParentHash:     head.Hash(),
		Time:           new(big.Int).SetInt64(time.Now().UTC().Unix()),
		ProposerPubKey: crypto.FromECDSAPub(chain.pubKey),
		TxHash:         types.DeriveSha(types.Transactions(txs)),
	}

	block := &types.Block{
		Header: &types.Header{
			ProposedHeader: header,
		},
		Body: &types.Body{
			Transactions: txs,
		},
	}
	block.Body.BlockSeed, block.Body.SeedProof = chain.vrfSigner.Evaluate(chain.GetSeedData(block))
	return block
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

	f, _ := q.Float64()
	if f >= chain.config.Consensus.ProposerTheshold {
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
