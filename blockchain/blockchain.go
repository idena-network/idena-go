package blockchain

import (
	"crypto/ecdsa"
	"errors"
	"idena-go/common"
	"idena-go/config"
	"idena-go/core/state"
	"idena-go/crypto"
	"idena-go/crypto/vrf"
	"idena-go/crypto/vrf/p256"
	"idena-go/idenadb"
	"idena-go/log"
	"math/big"
	"time"
)

const (
	Mainnet Network = 0x1
	Testnet Network = 0x2
)

const (
	ProposerRole uint8 = 0x1
)

var (
	MaxHash *big.Float
)

type Blockchain struct {
	db idenadb.Database

	Head      *Block
	config    *config.Config
	vrfSigner vrf.PrivateKey
	pubKey    *ecdsa.PublicKey
	log       log.Logger
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

func NewBlockchain(config *config.Config, db idenadb.Database) *Blockchain {
	return &Blockchain{
		db:     db,
		config: config,
		log:    log.New(),
	}
}

func (chain *Blockchain) GetHead() *Block {
	head := ReadHead(chain.db)
	if head == nil {
		return nil
	}
	return ReadBlock(chain.db, head.Hash())
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

func (chain *Blockchain) SetCurrentHead(block *Block) {
	chain.Head = block
}

func (chain *Blockchain) GenerateGenesis(network Network) *Block {

	statedb, _ := state.New(common.Hash{}, state.NewDatabase(chain.db))
	root := statedb.IntermediateRoot(false)

	var emptyHash [32]byte
	seed := Seed(crypto.Keccak256Hash(append([]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6}, common.ToBytes(network)...)))
	block := Block{Header: &Header{
		ProposedHeader: &ProposedHeader{
			ParentHash: emptyHash,

			Time:   big.NewInt(0),
			Height: 1,
			Root:   root,
		},
	}, BlockSeed: seed,}

	statedb.Commit(true)
	statedb.Database().TrieDB().Commit(root, true)

	chain.insertBlock(&block)
	return &block
}

func (chain *Blockchain) GetBlockByHeight(height uint64) *Block {
	hash := ReadCanonicalHash(chain.db, height)
	if len(hash) == 0 {
		return nil
	}
	return ReadBlock(chain.db, hash)
}

func (chain *Blockchain) GenerateEmptyBlock() *Block {
	head := chain.Head
	return &Block{
		Header: &Header{
			EmptyBlockHeader: &EmptyBlockHeader{
				ParentHash: head.Hash(),
				Height:     head.Height() + 1,
			},
		},
	}
}

func (chain *Blockchain) AddBlock(block *Block) error {

	if err := chain.validateBlockParentHash(block); err != nil {
		return err
	}
	if block.IsEmpty() {
		chain.insertBlock(chain.GenerateEmptyBlock())
	} else {
		if err := chain.ValidateProposedBlock(block); err != nil {
			return err
		}
		chain.insertBlock(block)
	}
	return nil
}

func (chain *Blockchain) GetSeedData(proposalBlock *Block) []byte {
	head := chain.Head
	result := head.Seed().Bytes()
	result = append(result, common.ToBytes(proposalBlock.Height())...)
	result = append(result, proposalBlock.Hash().Bytes()...)
	return result
}

func (chain *Blockchain) GetProposerSortition() (bool, common.Hash, []byte) {
	return chain.getSortition(chain.getProposerData())
}

func (chain *Blockchain) ProposeBlock(hash common.Hash, proof []byte) *Block {
	head := chain.Head

	header := &ProposedHeader{
		Height:         head.Height(),
		ParentHash:     head.Hash(),
		Time:           new(big.Int).SetInt64(time.Now().UTC().Unix()),
		ProposerPubKey: crypto.FromECDSAPub(chain.pubKey),
	}
	block := &Block{
		Header: &Header{
			ProposedHeader: header,
		},
	}
	block.BlockSeed, block.SeedProof = chain.vrfSigner.Evaluate(chain.GetSeedData(block))
	return block
}

func (chain *Blockchain) insertBlock(block *Block) {
	WriteBlock(chain.db, block)
	WriteHead(chain.db, block.Header)
	WriteCanonicalHash(chain.db, block.Height(), block.Hash())
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

	i := new(big.Int).SetBytes(hash[:])

	if f, _ := new(big.Float).Quo(new(big.Float).SetInt(i), MaxHash).Float64(); f >= chain.config.Consensus.ProposerTheshold {
		return true, hash, proof
	}
	return false, common.Hash{}, nil
}

func (chain *Blockchain) ValidateProposedBlock(block *Block) error {

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

	hash, err := verifier.ProofToHash(seedData, block.SeedProof)
	if err != nil {
		return err
	}
	if hash != block.Seed() || len(block.Seed()) == 0 {
		return errors.New("Seed is invalid")
	}
	return nil
}

func (chain *Blockchain) validateBlockParentHash(block *Block) error {
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

