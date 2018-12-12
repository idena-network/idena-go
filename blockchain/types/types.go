package types

import (
	"idena-go/common"
	"idena-go/crypto"
	"idena-go/crypto/sha3"
	"idena-go/rlp"
	"math/big"
	"sync/atomic"
)

const (
	SendTx       uint16 = 0x0
	ApprovingTx  uint16 = 0x1
	SendInviteTx uint16 = 0x2
)

type Network = int32

type Seed [32]byte

func (h Seed) Bytes() []byte { return h[:] }

type EmptyBlockHeader struct {
	ParentHash common.Hash
	Height     uint64
	Root       common.Hash
}

type ProposedHeader struct {
	ParentHash     common.Hash
	Height         uint64
	Time           *big.Int `json:"timestamp"        gencodec:"required"`
	TxHash         common.Hash // hash of tx hashes
	ProposerPubKey []byte
	Root           common.Hash
	Coinbase       common.Address // address of proposer
}

type Header struct {
	EmptyBlockHeader *EmptyBlockHeader `rlp:"nil"`
	ProposedHeader   *ProposedHeader   `rlp:"nil"`
}

type TxType = uint16

type VoteHeader struct {
	Round      uint64
	Step       uint16
	ParentHash common.Hash
	VotedHash  common.Hash
}

type Block struct {
	Header *Header

	Body *Body

	// caches
	hash atomic.Value
}

type Body struct {
	BlockSeed Seed

	SeedProof []byte

	Transactions []*Transaction
}

type Transaction struct {
	AccountNonce uint64
	Type         TxType
	To           *common.Address `rlp:"nil"`
	Amount       *big.Int        `json:"value"    gencodec:"required"`
	Payload      []byte          `rlp:"nil"	json:"input"    gencodec:"required"`

	Signature []byte

	// caches
	hash atomic.Value
	from atomic.Value
}

type BlockCert []*Vote

// Transactions is a Transaction slice type for basic sorting.
type Transactions []*Transaction

type Vote struct {
	Header    *VoteHeader
	Signature []byte

	// caches
	hash atomic.Value
	addr atomic.Value
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
	return b.Body.BlockSeed
}
func (b *Block) Height() uint64 {
	if b.IsEmpty() {
		return b.Header.EmptyBlockHeader.Height
	}
	return b.Header.ProposedHeader.Height
}
func (b *Block) Root() common.Hash {
	if b.IsEmpty() {
		return b.Header.EmptyBlockHeader.Root
	}
	return b.Header.ProposedHeader.Root
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

func (h *VoteHeader) SignatureHash() common.Hash {
	return rlpHash(h)
}

func (v *Vote) Hash() common.Hash {

	if hash := v.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	h := rlpHash([]interface{}{v.Header.SignatureHash(),
		v.VoterAddr(),
	})
	v.hash.Store(h)
	return h
}
func (v *Vote) VoterAddr() common.Address {
	if addr := v.addr.Load(); addr != nil {
		return addr.(common.Address)
	}

	hash := v.Header.SignatureHash()

	addr := common.Address{}
	pubKey, err := crypto.Ecrecover(hash[:], v.Signature)
	if err == nil {
		addr, _ = crypto.PubKeyBytesToAddress(pubKey)
	}
	v.addr.Store(addr)
	return addr
}

func (tx *Transaction) Hash() common.Hash {

	if hash := tx.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	h := rlpHash(tx)
	tx.hash.Store(h)
	return h
}

func (tx *Transaction) Size() int {
	b, _ := rlp.EncodeToBytes(tx)
	return len(b)
}

// Len returns the length of s.
func (s Transactions) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s.
func (s Transactions) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp.
func (s Transactions) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}

func (s BlockCert) Len() int { return len(s) }
