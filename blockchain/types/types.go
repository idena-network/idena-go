package types

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/rlp"
	"math/big"
	"sync/atomic"
	"time"
)

const (
	SendTx               uint16 = 0x0
	ActivationTx         uint16 = 0x1
	InviteTx             uint16 = 0x2
	KillTx               uint16 = 0x3
	SubmitFlipTx         uint16 = 0x4
	SubmitAnswersHashTx  uint16 = 0x5
	SubmitShortAnswersTx uint16 = 0x6
	SubmitLongAnswersTx  uint16 = 0x7
	EvidenceTx           uint16 = 0x8
	OnlineStatusTx       uint16 = 0x9
	KillInviteeTx        uint16 = 0xA
)

const (
	ReductionOne = 998
	ReductionTwo = 999
	Final        = 1000
)

type BlockFlag uint32

const (
	IdentityUpdate BlockFlag = 1 << iota
	FlipLotteryStarted
	ShortSessionStarted
	LongSessionStarted
	AfterLongSessionStarted
	ValidationFinished
	Snapshot
	OfflinePropose
	OfflineCommit
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
	Time         *big.Int
	Flags        BlockFlag
}

type ProposedHeader struct {
	ParentHash     common.Hash
	Height         uint64
	Time           *big.Int    `json:"timestamp"`
	TxHash         common.Hash // hash of tx hashes
	ProposerPubKey []byte
	Root           common.Hash    // root of state tree
	IdentityRoot   common.Hash    // root of approved identities tree
	Coinbase       common.Address // address of proposer
	Flags          BlockFlag
	IpfsHash       []byte          // ipfs hash of block body
	OfflineAddr    *common.Address `rlp:"nil"`
	TxBloom        []byte
	BlockSeed      Seed

	SeedProof []byte
}

type Header struct {
	EmptyBlockHeader *EmptyBlockHeader `rlp:"nil"`
	ProposedHeader   *ProposedHeader   `rlp:"nil"`
}

type TxType = uint16

type VoteHeader struct {
	Round       uint64
	Step        uint16
	ParentHash  common.Hash
	VotedHash   common.Hash
	TurnOffline bool
}

type Block struct {
	Header *Header

	Body *Body
	// caches
	hash        atomic.Value
	proposeHash atomic.Value
	msgKey      atomic.Value
}

type Body struct {
	Transactions []*Transaction `rlp:"nil"`
}

type Transaction struct {
	AccountNonce uint32
	Epoch        uint16
	Type         TxType
	To           *common.Address `rlp:"nil"`
	Amount       *big.Int        `json:"value"`
	MaxFee       *big.Int
	Tips         *big.Int
	Payload      []byte `rlp:"nil"       json:"input"`

	Signature []byte

	// caches
	hash   atomic.Value
	from   atomic.Value
	msgKey atomic.Value
}

type BlockCert struct {
	Votes []*Vote
}

// Transactions is a Transaction slice type for basic sorting.
type Transactions []*Transaction

type NewEpochPayload struct {
	Identities []common.Address
}

type Vote struct {
	Header    *VoteHeader
	Signature []byte

	// caches
	hash   atomic.Value
	addr   atomic.Value
	msgKey atomic.Value
}

type Flip struct {
	Tx     *Transaction
	Data   []byte
	msgKey atomic.Value
}

type ActivityMonitor struct {
	UpdateDt time.Time
	Data     []*AddrActivity
}

type AddrActivity struct {
	Addr common.Address
	Time time.Time
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

func (b *Block) MsgKey() string {
	if key := b.msgKey.Load(); key != nil {
		return key.(string)
	}
	hash := rlp.Hash(b)
	key := string(hash[:])
	b.msgKey.Store(key)
	return key
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
		return h.EmptyBlockHeader.Time
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

func (h *Header) Flags() BlockFlag {
	if h.EmptyBlockHeader != nil {
		return h.EmptyBlockHeader.Flags
	} else {
		return h.ProposedHeader.Flags
	}
}

func (h *Header) Coinbase() common.Address {
	if h.EmptyBlockHeader != nil {
		return common.Address{}
	} else {
		return h.ProposedHeader.Coinbase
	}
}

func (h *Header) OfflineAddr() *common.Address {
	if h.EmptyBlockHeader != nil {
		return nil
	} else {
		return h.ProposedHeader.OfflineAddr
	}
}

func (h *ProposedHeader) Hash() common.Hash {
	return rlp.Hash(h)
}
func (h *EmptyBlockHeader) Hash() common.Hash {
	return rlp.Hash(h)
}

func (h *VoteHeader) SignatureHash() common.Hash {
	return rlp.Hash(h)
}

func (v *Vote) Hash() common.Hash {

	if hash := v.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	h := common.Hash(rlp.Hash([]interface{}{v.Header.SignatureHash(),
		v.VoterAddr(),
	}))
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

func (v *Vote) MsgKey() string {
	if key := v.msgKey.Load(); key != nil {
		return key.(string)
	}
	hash := rlp.Hash(v)
	key := string(hash[:])
	v.msgKey.Store(key)
	return key
}

func (tx *Transaction) AmountOrZero() *big.Int {
	if tx.Amount == nil {
		return big.NewInt(0)
	}
	return tx.Amount
}

func (tx *Transaction) MaxFeeOrZero() *big.Int {
	if tx.MaxFee == nil {
		return big.NewInt(0)
	}
	return tx.MaxFee
}

func (tx *Transaction) TipsOrZero() *big.Int {
	if tx.Tips == nil {
		return big.NewInt(0)
	}
	return tx.Tips
}

func (tx *Transaction) Hash() common.Hash {

	if hash := tx.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	h := common.Hash(rlp.Hash(tx))
	tx.hash.Store(h)
	return h
}

func (tx *Transaction) Size() int {
	b, _ := rlp.EncodeToBytes(tx)
	return len(b)
}

func (tx *Transaction) MsgKey() string {
	if key := tx.msgKey.Load(); key != nil {
		return key.(string)
	}
	hash := rlp.Hash(tx)
	key := string(hash[:])
	tx.msgKey.Store(key)
	return key
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

func (s *BlockCert) Len() int { return len(s.Votes) }

func (s *BlockCert) Empty() bool {
	return s == nil || s.Len() == 0
}

func (p NewEpochPayload) Bytes() []byte {
	enc, _ := rlp.EncodeToBytes(p)
	return enc
}

func (f BlockFlag) HasFlag(flag BlockFlag) bool {
	return f&flag != 0
}

func (f BlockFlag) UnsetFlag(flag BlockFlag) BlockFlag {
	return f &^ flag
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

type FlipKey struct {
	Key       []byte
	Signature []byte
	Epoch     uint16

	from atomic.Value
}

func (k FlipKey) Hash() common.Hash {
	return rlp.Hash(k)
}

func (f *Flip) MsgKey() string {
	if key := f.msgKey.Load(); key != nil {
		return key.(string)
	}
	hash := rlp.Hash(f)
	key := string(hash[:])
	f.msgKey.Store(key)
	return key
}

type Answer byte

const (
	None          Answer = 0
	Left          Answer = 1
	Right         Answer = 2
	Inappropriate Answer = 3
)

type Answers struct {
	Bits       *big.Int
	FlipsCount uint
}

func NewAnswers(flipsCount uint) *Answers {
	a := Answers{
		Bits:       new(big.Int),
		FlipsCount: flipsCount,
	}
	return &a
}

func NewAnswersFromBits(flipsCount uint, bits []byte) *Answers {
	a := Answers{
		Bits:       new(big.Int).SetBytes(bits),
		FlipsCount: flipsCount,
	}
	return &a
}

func (a *Answers) Left(flipIndex uint) {
	if flipIndex >= a.FlipsCount {
		panic("index is out of range")
	}

	t := big.NewInt(1)
	a.Bits.Or(a.Bits, t.Lsh(t, flipIndex))
}

func (a *Answers) Right(flipIndex uint) {
	if flipIndex >= a.FlipsCount {
		panic("index is out of range")
	}

	t := big.NewInt(1)
	a.Bits.Or(a.Bits, t.Lsh(t, flipIndex+a.FlipsCount))
}

func (a *Answers) Inappropriate(flipIndex uint) {
	if flipIndex >= a.FlipsCount {
		panic("index is out of range")
	}
	t := big.NewInt(1)
	a.Bits.Or(a.Bits, t.Lsh(t, flipIndex+a.FlipsCount*2))
}

func (a *Answers) WrongWords(flipIndex uint) {
	if flipIndex >= a.FlipsCount {
		panic("index is out of range")
	}
	t := big.NewInt(1)
	a.Bits.Or(a.Bits, t.Lsh(t, flipIndex+a.FlipsCount*3))
}

func (a *Answers) Bytes() []byte {
	return a.Bits.Bytes()
}

func (a *Answers) Answer(flipIndex uint) (answer Answer, wrongWords bool) {
	answer = None
	if a.Bits.Bit(int(flipIndex)) == 1 {
		answer = Left
	} else if a.Bits.Bit(int(flipIndex+a.FlipsCount)) == 1 {
		answer = Right
	} else if a.Bits.Bit(int(flipIndex+a.FlipsCount*2)) == 1 {
		answer = Inappropriate
	}
	wrongWords = a.Bits.Bit(int(flipIndex+a.FlipsCount*3)) == 1
	return
}

type ValidationResult struct {
	StrongFlips       int
	WeakFlips         int
	SuccessfulInvites int
}

type ValidationAuthors struct {
	BadAuthors  map[common.Address]struct{}
	GoodAuthors map[common.Address]*ValidationResult
}
