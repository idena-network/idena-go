package blockchain

import (
	"idena-go/common"
	"idena-go/crypto/sha3"
	"idena-go/rlp"
	"math/big"
	"sync/atomic"
)

type Network = int32

type Seed [32]byte

func (h Seed) Bytes() []byte { return h[:] }

type EmptyBlockHeader struct {
	ParentHash common.Hash
	Height     uint64
}

type ProposedHeader struct {
	ParentHash     common.Hash
	Height         uint64
	Time           *big.Int `json:"timestamp"        gencodec:"required"`
	TxHash         common.Hash // hash of tx hashes
	ProposerPubKey []byte
}

type Header struct {
	EmptyBlockHeader *EmptyBlockHeader `rlp:"nil"`
	ProposedHeader   *ProposedHeader   `rlp:"nil"`
}

type Block struct {
	Header *Header

	BlockSeed Seed

	SeedProof []byte

	// caches
	hash atomic.Value
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

func (b *Block) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.Header.Hash()
	b.hash.Store(v)
	return v
}

func (b *Block) IsEmpty() bool {
	return b.Header.EmptyBlockHeader != nil
}

func (b *Block) Seed() Seed {
	return b.BlockSeed
}
func (b *Block) Height() uint64 {
	if b.IsEmpty() {
		return b.Header.EmptyBlockHeader.Height
	}
	return b.Header.ProposedHeader.Height
}

func (h *Header) Hash() common.Hash {
	if h.ProposedHeader != nil {
		return h.ProposedHeader.Hash()
	}
	return h.EmptyBlockHeader.Hash()
}

func (h *Header) Height() uint64 {
	if h.ProposedHeader != nil {
		return h.ProposedHeader.Height
	}
	return h.EmptyBlockHeader.Height
}

func (h *Header) ParentHash() common.Hash {
	if h.ProposedHeader != nil {
		return h.ProposedHeader.ParentHash
	}
	return h.EmptyBlockHeader.ParentHash
}

func (h *ProposedHeader) Hash() common.Hash {
	return rlpHash(h)
}
func (h *EmptyBlockHeader) Hash() common.Hash {
	return rlpHash(h)
}
