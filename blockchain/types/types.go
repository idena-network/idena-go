package types

import (
	"fmt"
	"idena-go/common"
	"idena-go/crypto"
	"idena-go/crypto/sha3"
	"idena-go/rlp"
	"math/big"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	RegularTx    uint16 = 0x0
	ActivationTx uint16 = 0x1
	InviteTx     uint16 = 0x2
	KillTx       uint16 = 0x3
	SubmitFlipTx uint16 = 0x4
)

type BlockFlag uint32

const (
	IdentityUpdate BlockFlag = 1 << iota
)

type Network = uint32

type Seed [32]byte

func (h Seed) Bytes() []byte { return h[:] }

type EmptyBlockHeader struct {
	ParentHash   common.Hash
	Height       uint64
	Root         common.Hash
	IdentityRoot common.Hash
	BlockSeed    Seed
}

type ProposedHeader struct {
	ParentHash     common.Hash
	Height         uint64
	Time           *big.Int    `json:"timestamp"        gencodec:"required"`
	TxHash         common.Hash // hash of tx hashes
	ProposerPubKey []byte
	Root           common.Hash    // root of state tree
	IdentityRoot   common.Hash    // root of approved identities tree
	Coinbase       common.Address // address of proposer
	Flags          BlockFlag
	IpfsHash       []byte // ipfs hash of block body

	BlockSeed Seed

	SeedProof []byte
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
	hash        atomic.Value
	proposeHash atomic.Value
}

type Body struct {
	Transactions []*Transaction `rlp:"nil"`
}

type Transaction struct {
	AccountNonce uint32
	Epoch        uint16
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

type NewEpochPayload struct {
	Identities []common.Address
}

type Vote struct {
	Header    *VoteHeader
	Signature []byte

	// caches
	hash atomic.Value
	addr atomic.Value
}

type Flip struct {
	Tx   *Transaction
	Data []byte
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

func (b *Block) ProposeHash() common.Hash {
	if hash := b.proposeHash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	var hash common.Hash
	if !b.IsEmpty() {
		h := b.Header.ProposedHeader
		hash = rlpHash([]interface{}{
			h.IpfsHash,
			h.Root,
			h.IdentityRoot,
			h.Height,
			h.Flags,
			h.Coinbase,
			h.ParentHash,
			h.TxHash,
			h.ProposerPubKey,
			h.Time,
		})
	} else {
		h := b.Header.EmptyBlockHeader
		hash = rlpHash([]interface{}{
			h.Root,
			h.IdentityRoot,
			h.Height,
			h.ParentHash,
		})
	}
	b.proposeHash.Store(hash)
	return hash
}

func (b *Block) IsEmpty() bool {
	return b.Header.EmptyBlockHeader != nil
}

func (b *Block) Seed() Seed {
	return b.Header.Seed()
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

func (b *Block) IdentityRoot() common.Hash {
	return b.Header.IdentityRoot()
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

func (h *Header) Seed() Seed {
	if h.EmptyBlockHeader != nil {
		return h.EmptyBlockHeader.BlockSeed
	} else {
		return h.ProposedHeader.BlockSeed
	}
}

func (h *Header) Root() common.Hash {
	if h.EmptyBlockHeader != nil {
		return h.EmptyBlockHeader.Root
	} else {
		return h.ProposedHeader.Root
	}
}

func (h *Header) IdentityRoot() common.Hash {
	if h.EmptyBlockHeader != nil {
		return h.EmptyBlockHeader.IdentityRoot
	} else {
		return h.ProposedHeader.IdentityRoot
	}
}

func (h *Header) Time() *big.Int {
	if h.EmptyBlockHeader != nil {
		return nil
	} else {
		return h.ProposedHeader.Time
	}
}

func (h *Header) IpfsHash() []byte {
	if h.EmptyBlockHeader != nil {
		return nil
	} else {
		return h.ProposedHeader.IpfsHash
	}
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

func (tx *Transaction) AmountOrZero() *big.Int {
	if tx.Amount == nil {
		return big.NewInt(0)
	}
	return tx.Amount
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

func (p NewEpochPayload) Bytes() []byte {
	enc, _ := rlp.EncodeToBytes(p)
	return enc
}

func (f BlockFlag) HasFlag(flag BlockFlag) bool {
	return f&flag != 0
}

func (b Body) Bytes() []byte {
	if len(b.Transactions) == 0 {
		return []byte{}
	}
	enc, _ := rlp.EncodeToBytes(b)
	return enc
}

func (b *Body) FromBytes(data []byte) {
	if len(data) != 0 {
		rlp.DecodeBytes(data, b)
	}
	if b.Transactions == nil {
		b.Transactions = Transactions{}
	}
}

func (b Body) IsEmpty() bool {
	return len(b.Transactions) == 0
}

func (b Body) ToIpfs() map[string][]byte {
	txs := make(map[string][]byte)
	for i, tx := range b.Transactions {
		enc, _ := rlp.EncodeToBytes(tx)
		txs[fmt.Sprintf("%v_%x", i, tx.Hash())] = enc
	}
	return txs
}

func (b *Body) FromIpfs(txs map[string][]byte) {
	list := make([]*Transaction, len(txs))
	for key, tx := range txs {
		parts := strings.Split(key, "_")
		if len(parts) != 2 {
			continue
		}
		if idx, err := strconv.Atoi(parts[0]); err != nil {
			continue
		} else {
			decoded := &Transaction{}
			if err := rlp.DecodeBytes(tx, decoded); err != nil {
				continue
			}
			list[idx] = decoded
		}
	}
	result := make([]*Transaction, 0)
	for _, tx := range list {
		if tx != nil {
			result = append(result, tx)
		}
	}

	b.Transactions = Transactions(result)
}
