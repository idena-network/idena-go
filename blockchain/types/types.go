package types

import (
	"bytes"
	"errors"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	models "github.com/idena-network/idena-go/protobuf"
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
	ChangeGodAddressTx   uint16 = 0xB
	BurnTx               uint16 = 0xC
	ChangeProfileTx      uint16 = 0xD
	DeleteFlipTx         uint16 = 0xE
	DeployContract       uint16 = 0xF
	CallContract         uint16 = 0x10
	TerminateContract    uint16 = 0x11
	DelegateTx 			 uint16 = 0x12
	UndelegateTx         uint16 = 0x13
)

const (
	ReductionOne = 253
	ReductionTwo = 254
	Final        = 255

	MaxBlockGas = 3000 * 1024
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
	NewGenesis
)

var CeremonialTxs map[uint16]struct{}

func init() {
	CeremonialTxs = map[uint16]struct{}{
		SubmitAnswersHashTx:  {},
		SubmitShortAnswersTx: {},
		SubmitLongAnswersTx:  {},
		EvidenceTx:           {},
	}
}

type Network = uint32

type Seed [32]byte

func (h Seed) Bytes() []byte { return h[:] }

func (h *Seed) SetBytes(b []byte) {
	if len(b) > len(h) {
		b = b[len(b)-32:]
	}
	copy(h[32-len(b):], b)
}

func BytesToSeed(b []byte) Seed {
	var a Seed
	a.SetBytes(b)
	return a
}

type EmptyBlockHeader struct {
	ParentHash   common.Hash
	Height       uint64
	Root         common.Hash
	IdentityRoot common.Hash
	BlockSeed    Seed
	Time         int64
	Flags        BlockFlag
}

type ProposedHeader struct {
	ParentHash     common.Hash
	Height         uint64
	Time           int64
	TxHash         common.Hash // hash of tx hashes
	ProposerPubKey []byte
	Root           common.Hash // root of state tree
	IdentityRoot   common.Hash // root of approved identities tree
	Flags          BlockFlag
	IpfsHash       []byte          // ipfs hash of block body
	OfflineAddr    *common.Address `rlp:"nil"`
	TxBloom        []byte
	BlockSeed      Seed
	FeePerGas      *big.Int
	Upgrade        uint32
	SeedProof      []byte
	TxReceiptsCid  []byte
}

type Header struct {
	EmptyBlockHeader *EmptyBlockHeader `rlp:"nil"`
	ProposedHeader   *ProposedHeader   `rlp:"nil"`
}

type TxType = uint16

type VoteHeader struct {
	Round       uint64
	Step        uint8
	ParentHash  common.Hash
	VotedHash   common.Hash
	TurnOffline bool
	Upgrade     uint32
}

type Block struct {
	Header *Header

	Body *Body

	// caches
	hash        atomic.Value
	hash128     atomic.Value
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
	Amount       *big.Int        `json:"value"`
	MaxFee       *big.Int
	Tips         *big.Int
	Payload      []byte `rlp:"nil"       json:"input"`

	Signature []byte

	UseRlp bool `rlp:"-"`

	// caches
	hash    atomic.Value
	hash128 atomic.Value
	from    atomic.Value
}

type FullBlockCert struct {
	Votes []*Vote
}

type BlockCertSignature struct {
	TurnOffline bool
	Upgrade     uint32
	Signature   []byte
}

type BlockCert struct {
	Round      uint64
	Step       uint8
	VotedHash  common.Hash
	Signatures []*BlockCertSignature
}

type BlockBundle struct {
	Block *Block
	Cert  *BlockCert
}

type BlockProposal struct {
	*Block
	Signature []byte
	Proof     []byte

	pubKey atomic.Value
}

func BlockProposalPubKey(proposal *BlockProposal) ([]byte, error) {
	if pubKey := proposal.pubKey.Load(); pubKey != nil {
		return pubKey.([]byte), nil
	}

	hash := crypto.SignatureHash(proposal)
	pub, err := crypto.Ecrecover(hash[:], proposal.Signature)
	if err != nil {
		return nil, err
	}
	proposal.pubKey.Store(pub)
	return pub, nil
}

type ProofProposal struct {
	Proof []byte
	Round uint64

	Signature []byte

	pubKey  atomic.Value
	hash128 atomic.Value
}

func ProofProposalPubKey(proposal *ProofProposal) ([]byte, error) {
	if pubKey := proposal.pubKey.Load(); pubKey != nil {
		return pubKey.([]byte), nil
	}

	hash := crypto.SignatureHash(proposal)
	pub, err := crypto.Ecrecover(hash[:], proposal.Signature)
	if err != nil {
		return nil, err
	}
	proposal.pubKey.Store(pub)
	return pub, nil
}

// Transactions is a Transaction slice type for basic sorting.
type Transactions []*Transaction

type Vote struct {
	Header    *VoteHeader
	Signature []byte

	// caches
	hash    atomic.Value
	hash128 atomic.Value
	addr    atomic.Value
}

type Flip struct {
	Tx          *Transaction
	PublicPart  []byte
	PrivatePart []byte

	hash128 atomic.Value
}

func (f *Flip) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoFlip{
		PublicPart:  f.PublicPart,
		PrivatePart: f.PrivatePart,
	}
	if f.Tx != nil {
		protoObj.Transaction = f.Tx.ToProto()
	}
	return proto.Marshal(protoObj)
}

func (f *Flip) FromBytes(data []byte) error {
	protoObj := new(models.ProtoFlip)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	f.PublicPart = protoObj.PublicPart
	f.PrivatePart = protoObj.PrivatePart
	if protoObj.Transaction != nil {
		f.Tx = new(Transaction).FromProto(protoObj.Transaction)
	}
	return nil
}

func (f *Flip) Hash128() common.Hash128 {
	if hash := f.hash128.Load(); hash != nil {
		return hash.(common.Hash128)
	}

	data, _ := f.ToBytes()
	h := common.Hash128(crypto.Hash128(data))

	f.hash128.Store(h)
	return h
}

func (f *Flip) IsValid() bool {
	return f.Tx != nil
}

type AddrActivity struct {
	Addr common.Address
	Time time.Time
}

type ActivityMonitor struct {
	UpdateDt time.Time
	Data     []*AddrActivity
}

func (s *ActivityMonitor) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoActivityMonitor{
		Timestamp: s.UpdateDt.Unix(),
	}
	for _, item := range s.Data {
		protoObj.Activities = append(protoObj.Activities, &models.ProtoActivityMonitor_Activity{
			Address:   item.Addr[:],
			Timestamp: item.Time.Unix(),
		})
	}
	return proto.Marshal(protoObj)
}

func (s *ActivityMonitor) FromBytes(data []byte) error {
	protoObj := new(models.ProtoActivityMonitor)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	s.UpdateDt = time.Unix(protoObj.Timestamp, 0)
	for _, item := range protoObj.Activities {
		s.Data = append(s.Data, &AddrActivity{
			Addr: common.BytesToAddress(item.Address),
			Time: time.Unix(item.Timestamp, 0),
		})
	}

	return nil
}

type SavedTransaction struct {
	Tx        *Transaction
	FeePerGas *big.Int
	BlockHash common.Hash
	Timestamp int64
}

func (s *SavedTransaction) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoSavedTransaction{
		FeePerGas: common.BigIntBytesOrNil(s.FeePerGas),
		BlockHash: s.BlockHash[:],
		Timestamp: s.Timestamp,
	}
	if s.Tx != nil {
		protoObj.Tx = s.Tx.ToProto()
	}
	return proto.Marshal(protoObj)
}

func (s *SavedTransaction) FromBytes(data []byte) error {
	protoObj := new(models.ProtoSavedTransaction)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	tx := new(Transaction)
	if protoObj != nil {
		s.Tx = tx.FromProto(protoObj.Tx)
	}
	s.Timestamp = protoObj.Timestamp
	s.BlockHash = common.BytesToHash(protoObj.BlockHash)
	s.FeePerGas = common.BigIntOrNil(protoObj.FeePerGas)
	return nil
}

type BurntCoins struct {
	Address common.Address
	Key     string
	Amount  *big.Int
}

func (s *BurntCoins) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoBurntCoins{
		Address: s.Address[:],
		Key:     s.Key,
		Amount:  common.BigIntBytesOrNil(s.Amount),
	}
	return proto.Marshal(protoObj)
}

func (s *BurntCoins) FromBytes(data []byte) error {
	protoObj := new(models.ProtoBurntCoins)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	s.Key = protoObj.Key
	s.Amount = common.BigIntOrNil(protoObj.Amount)
	s.Address = common.BytesToAddress(protoObj.Address)
	return nil
}

func (b *Block) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.Header.Hash()
	b.hash.Store(v)
	return v
}

func (b *Block) Hash128() common.Hash128 {
	if hash := b.hash128.Load(); hash != nil {
		return hash.(common.Hash128)
	}

	data, _ := b.ToBytes()
	h := common.Hash128(crypto.Hash128(data))

	b.hash128.Store(h)
	return h
}

func (b *Block) IsEmpty() bool {
	return b.Header != nil && b.Header.EmptyBlockHeader != nil
}

func (b *Block) IsValid() bool {
	return b.Header.IsValid() && b.Body.IsValid()
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

func (b *Block) ToBytes() ([]byte, error) {
	protoObj := new(models.ProtoBlock)
	if b.Header != nil {
		protoObj.Header = b.Header.ToProto()
	}
	if b.Body != nil {
		protoObj.Body = b.Body.ToProto()
	}

	return proto.Marshal(protoObj)
}

func (b *Block) FromBytes(data []byte) error {
	protoObj := new(models.ProtoBlock)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	if protoObj.Header != nil {
		b.Header = new(Header).FromProto(protoObj.Header)
	}
	if protoObj.Body != nil {
		b.Body = new(Body).FromProto(protoObj.Body)
	}
	return nil
}

func (h *Header) ToProto() *models.ProtoBlockHeader {
	protoHeader := new(models.ProtoBlockHeader)
	if h.EmptyBlockHeader != nil {
		protoHeader.EmptyHeader = h.EmptyBlockHeader.ToProto()
	}
	if h.ProposedHeader != nil {
		protoHeader.ProposedHeader = h.ProposedHeader.ToProto()
	}
	return protoHeader
}

func (h *Header) ToBytes() ([]byte, error) {
	return proto.Marshal(h.ToProto())
}

func (h *Header) FromProto(protoHeader *models.ProtoBlockHeader) *Header {
	if protoHeader.EmptyHeader != nil {
		empty := &EmptyBlockHeader{
			ParentHash:   common.BytesToHash(protoHeader.EmptyHeader.ParentHash),
			Height:       protoHeader.EmptyHeader.Height,
			Root:         common.BytesToHash(protoHeader.EmptyHeader.Root),
			IdentityRoot: common.BytesToHash(protoHeader.EmptyHeader.IdentityRoot),
			BlockSeed:    BytesToSeed(protoHeader.EmptyHeader.BlockSeed),
			Time:         protoHeader.EmptyHeader.Timestamp,
			Flags:        BlockFlag(protoHeader.EmptyHeader.Flags),
		}
		h.EmptyBlockHeader = empty
	}
	if protoHeader.ProposedHeader != nil {
		proposed := &ProposedHeader{
			ParentHash:     common.BytesToHash(protoHeader.ProposedHeader.ParentHash),
			Height:         protoHeader.ProposedHeader.Height,
			Time:           protoHeader.ProposedHeader.Timestamp,
			TxHash:         common.BytesToHash(protoHeader.ProposedHeader.TxHash),
			ProposerPubKey: protoHeader.ProposedHeader.ProposerPubKey,
			Root:           common.BytesToHash(protoHeader.ProposedHeader.Root),
			IdentityRoot:   common.BytesToHash(protoHeader.ProposedHeader.IdentityRoot),
			Flags:          BlockFlag(protoHeader.ProposedHeader.Flags),
			IpfsHash:       protoHeader.ProposedHeader.IpfsHash,
			TxBloom:        protoHeader.ProposedHeader.TxBloom,
			BlockSeed:      BytesToSeed(protoHeader.ProposedHeader.BlockSeed),
			FeePerGas:      common.BigIntOrNil(protoHeader.ProposedHeader.FeePerGas),
			Upgrade:        protoHeader.ProposedHeader.Upgrade,
			SeedProof:      protoHeader.ProposedHeader.SeedProof,
			TxReceiptsCid:  protoHeader.ProposedHeader.ReceiptsCid,
		}
		if len(protoHeader.ProposedHeader.OfflineAddr) > 0 {
			addr := common.BytesToAddress(protoHeader.ProposedHeader.OfflineAddr)
			proposed.OfflineAddr = &addr
		}
		h.ProposedHeader = proposed
	}
	return h
}

func (h *Header) FromBytes(data []byte) error {
	protoHeader := new(models.ProtoBlockHeader)
	if err := proto.Unmarshal(data, protoHeader); err != nil {
		return err
	}

	h.FromProto(protoHeader)
	return nil
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

func (h *Header) Time() int64 {
	if h.EmptyBlockHeader != nil {
		return h.EmptyBlockHeader.Time
	} else {
		return h.ProposedHeader.Time
	}
}

func (h *Header) FeePerGas() *big.Int {
	if h.EmptyBlockHeader != nil {
		return nil
	} else {
		return h.ProposedHeader.FeePerGas
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
		addr, _ := crypto.PubKeyBytesToAddress(h.ProposedHeader.ProposerPubKey)
		return addr
	}
}

func (h *Header) OfflineAddr() *common.Address {
	if h.EmptyBlockHeader != nil {
		return nil
	} else {
		return h.ProposedHeader.OfflineAddr
	}
}

func (h *Header) IsValid() bool {
	if h == nil {
		return false
	}
	return (h.EmptyBlockHeader == nil && h.ProposedHeader != nil) || (h.EmptyBlockHeader != nil && h.ProposedHeader == nil)
}

func (h *ProposedHeader) ToProto() *models.ProtoBlockHeader_Proposed {
	protoProposed := &models.ProtoBlockHeader_Proposed{
		ParentHash:     h.ParentHash[:],
		Height:         h.Height,
		Timestamp:      h.Time,
		TxHash:         h.TxHash[:],
		ProposerPubKey: h.ProposerPubKey,
		Root:           h.Root[:],
		IdentityRoot:   h.IdentityRoot[:],
		Flags:          uint32(h.Flags),
		IpfsHash:       h.IpfsHash,
		TxBloom:        h.TxBloom,
		BlockSeed:      h.BlockSeed[:],
		FeePerGas:      common.BigIntBytesOrNil(h.FeePerGas),
		Upgrade:        h.Upgrade,
		SeedProof:      h.SeedProof,
		ReceiptsCid:    h.TxReceiptsCid,
	}
	if h.OfflineAddr != nil {
		protoProposed.OfflineAddr = (*h.OfflineAddr)[:]
	}
	return protoProposed
}

func (h *ProposedHeader) Hash() common.Hash {
	b, _ := proto.Marshal(h.ToProto())
	return crypto.Hash(b)
}

func (h *EmptyBlockHeader) ToProto() *models.ProtoBlockHeader_Empty {
	protoEmpty := &models.ProtoBlockHeader_Empty{
		ParentHash:   h.ParentHash[:],
		Height:       h.Height,
		Root:         h.Root[:],
		IdentityRoot: h.IdentityRoot[:],
		Timestamp:    h.Time,
		BlockSeed:    h.BlockSeed[:],
		Flags:        uint32(h.Flags),
	}
	return protoEmpty
}

func (h *EmptyBlockHeader) Hash() common.Hash {
	b, _ := proto.Marshal(h.ToProto())
	return crypto.Hash(b)
}

func (v *Vote) ToSignatureBytes() ([]byte, error) {
	protoObj := &models.ProtoVote_Data{
		Round:       v.Header.Round,
		Step:        uint32(v.Header.Step),
		ParentHash:  v.Header.ParentHash[:],
		VotedHash:   v.Header.VotedHash[:],
		TurnOffline: v.Header.TurnOffline,
		Upgrade:     v.Header.Upgrade,
	}
	return proto.Marshal(protoObj)
}

func (v *Vote) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoVote{
		Signature: v.Signature,
	}
	if v.Header != nil {
		protoObj.Data = &models.ProtoVote_Data{
			Round:       v.Header.Round,
			Step:        uint32(v.Header.Step),
			ParentHash:  v.Header.ParentHash[:],
			VotedHash:   v.Header.VotedHash[:],
			TurnOffline: v.Header.TurnOffline,
			Upgrade:     v.Header.Upgrade,
		}
	}
	return proto.Marshal(protoObj)
}

func (v *Vote) FromBytes(data []byte) error {
	protoObj := new(models.ProtoVote)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	v.Signature = protoObj.Signature
	if protoObj.Data != nil {
		v.Header = &VoteHeader{
			Round:       protoObj.Data.Round,
			Step:        uint8(protoObj.Data.Step),
			ParentHash:  common.BytesToHash(protoObj.Data.ParentHash),
			VotedHash:   common.BytesToHash(protoObj.Data.VotedHash),
			TurnOffline: protoObj.Data.TurnOffline,
			Upgrade:     protoObj.Data.Upgrade,
		}
	}
	return nil
}

func (v *Vote) Hash() common.Hash {

	if hash := v.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}

	voteHash := crypto.SignatureHash(v)
	voterAddr := v.VoterAddr()
	h := common.Hash(crypto.Hash(append(voteHash[:], voterAddr[:]...)))

	v.hash.Store(h)
	return h
}

func (v *Vote) Hash128() common.Hash128 {
	if hash := v.hash128.Load(); hash != nil {
		return hash.(common.Hash128)
	}

	b, _ := v.ToBytes()
	h := common.Hash128(crypto.Hash128(b))

	v.hash128.Store(h)
	return h
}

func (v *Vote) VoterAddr() common.Address {
	if addr := v.addr.Load(); addr != nil {
		return addr.(common.Address)
	}

	hash := crypto.SignatureHash(v)

	addr := common.Address{}
	pubKey, err := crypto.Ecrecover(hash[:], v.Signature)
	if err == nil {
		addr, _ = crypto.PubKeyBytesToAddress(pubKey)
	}
	v.addr.Store(addr)
	return addr
}

func (v *Vote) IsValid() bool {
	return v.Header != nil
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

	bytes, _ := tx.ToBytes()
	h := common.Hash(crypto.Hash(bytes))
	tx.hash.Store(h)
	return h
}

func (tx *Transaction) Hash128() common.Hash128 {
	if hash := tx.hash128.Load(); hash != nil {
		return hash.(common.Hash128)
	}

	b, _ := tx.ToBytes()
	h := common.Hash128(crypto.Hash128(b))

	tx.hash128.Store(h)

	return h
}

func (tx *Transaction) Size() int {
	b, _ := tx.ToBytes()
	return len(b)
}

func (tx *Transaction) ToSignatureBytes() ([]byte, error) {
	protoTx := &models.ProtoTransaction_Data{
		Nonce:   tx.AccountNonce,
		Epoch:   uint32(tx.Epoch),
		Type:    uint32(tx.Type),
		Payload: tx.Payload,
		Amount:  common.BigIntBytesOrNil(tx.Amount),
		Tips:    common.BigIntBytesOrNil(tx.Tips),
		MaxFee:  common.BigIntBytesOrNil(tx.MaxFee),
	}
	if tx.To != nil {
		protoTx.To = tx.To.Bytes()
	}
	return proto.Marshal(protoTx)
}

func (tx *Transaction) ToProto() *models.ProtoTransaction {
	protoTx := &models.ProtoTransaction{
		Data: &models.ProtoTransaction_Data{
			Nonce:   tx.AccountNonce,
			Epoch:   uint32(tx.Epoch),
			Type:    uint32(tx.Type),
			Payload: tx.Payload,
			Amount:  common.BigIntBytesOrNil(tx.Amount),
			Tips:    common.BigIntBytesOrNil(tx.Tips),
			MaxFee:  common.BigIntBytesOrNil(tx.MaxFee),
		},
		Signature: tx.Signature,
		UseRlp:    tx.UseRlp,
	}

	if tx.To != nil {
		protoTx.Data.To = tx.To.Bytes()
	}
	return protoTx
}

func (tx *Transaction) ToBytes() ([]byte, error) {
	return proto.Marshal(tx.ToProto())
}

func (tx *Transaction) FromProto(protoTx *models.ProtoTransaction) *Transaction {
	if protoTx.Data != nil {
		tx.AccountNonce = protoTx.Data.GetNonce()
		tx.Epoch = uint16(protoTx.Data.GetEpoch())
		tx.Type = TxType(protoTx.Data.GetType())
		tx.Payload = protoTx.Data.GetPayload()

		if to := protoTx.Data.GetTo(); to != nil {
			addr := common.BytesToAddress(to)
			tx.To = &addr
		}
		tx.Amount = common.BigIntOrNil(protoTx.Data.Amount)
		tx.Tips = common.BigIntOrNil(protoTx.Data.Tips)
		tx.MaxFee = common.BigIntOrNil(protoTx.Data.MaxFee)
	}

	tx.Signature = protoTx.GetSignature()
	tx.UseRlp = protoTx.GetUseRlp()

	return tx
}

func (tx *Transaction) FromBytes(data []byte) error {
	protoTx := new(models.ProtoTransaction)
	if err := proto.Unmarshal(data, protoTx); err != nil {
		return err
	}
	tx.FromProto(protoTx)
	return nil
}

// Len returns the length of s.
func (s Transactions) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s.
func (s Transactions) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s Transactions) GetBytes(i int) []byte {
	enc, _ := s[i].ToBytes()
	return enc
}

func (s *BlockCert) Empty() bool {
	return s == nil || len(s.Signatures) == 0
}

func (s *BlockCert) ToProto() *models.ProtoBlockCert {
	protoObj := &models.ProtoBlockCert{
		Round:     s.Round,
		Step:      uint32(s.Step),
		VotedHash: s.VotedHash[:],
	}
	for _, item := range s.Signatures {
		protoObj.Signatures = append(protoObj.Signatures, &models.ProtoBlockCert_Signature{
			TurnOffline: item.TurnOffline,
			Upgrade:     item.Upgrade,
			Signature:   item.Signature,
		})
	}
	return protoObj
}

func (s *BlockCert) ToBytes() ([]byte, error) {
	return proto.Marshal(s.ToProto())
}

func (s *BlockCert) FromProto(protoObj *models.ProtoBlockCert) *BlockCert {
	s.Round = protoObj.Round
	s.Step = uint8(protoObj.Step)
	s.VotedHash = common.BytesToHash(protoObj.VotedHash)

	for _, item := range protoObj.Signatures {
		s.Signatures = append(s.Signatures, &BlockCertSignature{
			TurnOffline: item.TurnOffline,
			Upgrade:     item.Upgrade,
			Signature:   item.Signature,
		})
	}
	return s
}

func (s *BlockCert) FromBytes(data []byte) error {
	protoObj := new(models.ProtoBlockCert)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	s.FromProto(protoObj)
	return nil
}

func (s *FullBlockCert) Compress() *BlockCert {
	if len(s.Votes) == 0 {
		return &BlockCert{}
	}
	cert := &BlockCert{
		Round:     s.Votes[0].Header.Round,
		Step:      s.Votes[0].Header.Step,
		VotedHash: s.Votes[0].Header.VotedHash,
	}
	for _, vote := range s.Votes {
		cert.Signatures = append(cert.Signatures, &BlockCertSignature{
			Signature:   vote.Signature,
			Upgrade:     vote.Header.Upgrade,
			TurnOffline: vote.Header.TurnOffline,
		})
	}
	return cert
}

func (f BlockFlag) HasFlag(flag BlockFlag) bool {
	return f&flag != 0
}

func (f BlockFlag) UnsetFlag(flag BlockFlag) BlockFlag {
	return f &^ flag
}

func (b *Body) ToProto() *models.ProtoBlockBody {
	protoBody := new(models.ProtoBlockBody)

	for _, item := range b.Transactions {
		protoBody.Transactions = append(protoBody.Transactions, item.ToProto())
	}
	return protoBody
}

func (b *Body) ToBytes() []byte {
	res, _ := proto.Marshal(b.ToProto())
	return res
}

func (b *Body) FromProto(protoBody *models.ProtoBlockBody) *Body {
	if len(protoBody.Transactions) > 0 {
		for _, item := range protoBody.Transactions {
			tx := new(Transaction).FromProto(item)
			b.Transactions = append(b.Transactions, tx)
		}
	} else {
		b.Transactions = Transactions{}
	}
	return b
}

func (b *Body) FromBytes(data []byte) {
	protoBody := new(models.ProtoBlockBody)
	proto.Unmarshal(data, protoBody)
	b.FromProto(protoBody)
}

func (b *Body) IsValid() bool {
	return b != nil
}

func (p *ProofProposal) ToSignatureBytes() ([]byte, error) {
	protoObj := &models.ProtoProposeProof_Data{
		Proof: p.Proof,
		Round: p.Round,
	}
	return proto.Marshal(protoObj)
}

func (p *ProofProposal) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoProposeProof{
		Data: &models.ProtoProposeProof_Data{
			Proof: p.Proof,
			Round: p.Round,
		},
		Signature: p.Signature,
	}
	return proto.Marshal(protoObj)
}

func (p *ProofProposal) FromBytes(data []byte) error {
	protoObj := new(models.ProtoProposeProof)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	p.Signature = protoObj.Signature
	if protoObj.Data != nil {
		p.Round = protoObj.Data.Round
		p.Proof = protoObj.Data.Proof
	}
	return nil
}

func (p *ProofProposal) Hash128() common.Hash128 {
	if hash := p.hash128.Load(); hash != nil {
		return hash.(common.Hash128)
	}

	data, _ := p.ToBytes()
	h := common.Hash128(crypto.Hash128(data))

	p.hash128.Store(h)
	return h
}

func (p *BlockProposal) ToSignatureBytes() ([]byte, error) {
	protoObj := new(models.ProtoBlockProposal_Data)
	if p.Block != nil {
		if p.Block.Header != nil {
			protoObj.Header = p.Header.ToProto()
		}
		if p.Block.Body != nil {
			protoObj.Body = p.Body.ToProto()
		}
	}
	protoObj.Proof = p.Proof
	return proto.Marshal(protoObj)
}

func (p *BlockProposal) ToBytes() ([]byte, error) {
	protoObj := new(models.ProtoBlockProposal)
	protoObj.Signature = p.Signature
	protoObj.Data = &models.ProtoBlockProposal_Data{}
	protoObj.Data.Proof = p.Proof

	if p.Block != nil {
		if p.Block.Header != nil {
			protoObj.Data.Header = p.Header.ToProto()
		}
		if p.Block.Body != nil {
			protoObj.Data.Body = p.Body.ToProto()
		}
	}

	return proto.Marshal(protoObj)
}

func (p *BlockProposal) FromBytes(data []byte) error {
	protoObj := new(models.ProtoBlockProposal)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	p.Signature = protoObj.Signature
	if protoObj.Data != nil {
		p.Proof = protoObj.Data.Proof
		p.Block = &Block{}
		if protoObj.Data.Header != nil {
			p.Block.Header = new(Header).FromProto(protoObj.Data.Header)
		}
		if protoObj.Data.Body != nil {
			p.Block.Body = new(Body).FromProto(protoObj.Data.Body)
		}
	}
	return nil
}

func (p *BlockProposal) IsValid() bool {
	if p.Block == nil || len(p.Signature) == 0 || !p.Block.IsValid() || p.Block.IsEmpty() {
		return false
	}
	pubKey, err := BlockProposalPubKey(p)
	if err != nil {
		return false
	}
	return bytes.Compare(pubKey, p.Block.Header.ProposedHeader.ProposerPubKey) == 0
}

type PublicFlipKey struct {
	Key       []byte
	Signature []byte
	Epoch     uint16

	from atomic.Value
}

func (k *PublicFlipKey) ToSignatureBytes() ([]byte, error) {
	protoObj := &models.ProtoFlipKey_Data{
		Key:   k.Key,
		Epoch: uint32(k.Epoch),
	}
	return proto.Marshal(protoObj)
}

func (k *PublicFlipKey) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoFlipKey{
		Data: &models.ProtoFlipKey_Data{
			Key:   k.Key,
			Epoch: uint32(k.Epoch),
		},
		Signature: k.Signature,
	}
	return proto.Marshal(protoObj)
}

func (k *PublicFlipKey) FromBytes(data []byte) error {
	protoObj := new(models.ProtoFlipKey)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	k.Signature = protoObj.Signature
	if protoObj.Data != nil {
		k.Epoch = uint16(protoObj.Data.Epoch)
		k.Key = protoObj.Data.Key
	}
	return nil
}

func (k *PublicFlipKey) Hash() common.Hash {
	b, _ := k.ToBytes()
	return crypto.Hash(b)
}

type PrivateFlipKeysPackage struct {
	Data      []byte
	Epoch     uint16
	Signature []byte

	from    atomic.Value
	hash128 atomic.Value
}

func (k *PrivateFlipKeysPackage) ToSignatureBytes() ([]byte, error) {
	protoObj := &models.ProtoPrivateFlipKeysPackage_Data{
		Package: k.Data,
		Epoch:   uint32(k.Epoch),
	}
	return proto.Marshal(protoObj)
}

func (k *PrivateFlipKeysPackage) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoPrivateFlipKeysPackage{
		Data: &models.ProtoPrivateFlipKeysPackage_Data{
			Package: k.Data,
			Epoch:   uint32(k.Epoch),
		},
		Signature: k.Signature,
	}
	return proto.Marshal(protoObj)
}

func (k *PrivateFlipKeysPackage) FromBytes(data []byte) error {
	protoObj := new(models.ProtoPrivateFlipKeysPackage)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	k.Signature = protoObj.Signature
	if protoObj.Data != nil {
		k.Data = protoObj.Data.Package
		k.Epoch = uint16(protoObj.Data.Epoch)
	}
	return nil
}

func (k *PrivateFlipKeysPackage) Hash128() common.Hash128 {
	if hash := k.hash128.Load(); hash != nil {
		return hash.(common.Hash128)
	}

	data, _ := k.ToBytes()
	h := common.Hash128(crypto.Hash128(data))

	k.hash128.Store(h)
	return h
}

type Answer byte

const (
	None  Answer = 0
	Left  Answer = 1
	Right Answer = 2
)

type Grade byte

const (
	GradeNone     Grade = 0
	GradeReported Grade = 1
	GradeD        Grade = 2
	GradeC        Grade = 3
	GradeB        Grade = 4
	GradeA        Grade = 5
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

func (a *Answers) Grade(flipIndex uint, grade Grade) {
	if flipIndex >= a.FlipsCount {
		panic("index is out of range")
	}
	if grade > GradeA {
		panic("grade is out of range")
	}
	t := big.NewInt(int64(grade))
	a.Bits.Or(a.Bits, t.Lsh(t, flipIndex*3+a.FlipsCount*2))
}

func (a *Answers) Bytes() []byte {
	return a.Bits.Bytes()
}

func (a *Answers) Answer(flipIndex uint) (answer Answer, grade Grade) {
	answer = None
	if a.Bits.Bit(int(flipIndex)) == 1 {
		answer = Left
	} else if a.Bits.Bit(int(flipIndex+a.FlipsCount)) == 1 {
		answer = Right
	}
	t := new(big.Int)
	t.SetBit(t, 0, a.Bits.Bit(int(flipIndex*3+a.FlipsCount*2)))
	t.SetBit(t, 1, a.Bits.Bit(int(flipIndex*3+a.FlipsCount*2+1)))
	t.SetBit(t, 2, a.Bits.Bit(int(flipIndex*3+a.FlipsCount*2+2)))
	grade = Grade(t.Uint64())
	if grade > GradeA {
		grade = GradeNone
	}
	return
}

type ValidationResult struct {
	FlipsToReward    []*FlipToReward
	Missed           bool
	NewIdentityState uint8
}

type FlipToReward struct {
	Cid   []byte
	Grade Grade
}

type InviterValidationResult struct {
	SuccessfulInvites   []*SuccessfulInvite
	SavedInvites        uint8
	NewIdentityState    uint8
	PayInvitationReward bool
}

type SuccessfulInvite struct {
	Age    uint16
	TxHash common.Hash
}

type AuthorResults struct {
	HasOneReportedFlip     bool
	HasOneNotQualifiedFlip bool
	AllFlipsNotQualified   bool
}

type ValidationResults struct {
	BadAuthors              map[common.Address]BadAuthorReason
	GoodAuthors             map[common.Address]*ValidationResult
	AuthorResults           map[common.Address]*AuthorResults
	GoodInviters            map[common.Address]*InviterValidationResult
	ReportersToRewardByFlip map[int]map[common.Address]*Candidate
}

type Candidate struct {
	Address          common.Address
	NewIdentityState uint8
}

type BadAuthorReason = byte

const (
	NoQualifiedFlipsBadAuthor BadAuthorReason = 0
	QualifiedByNoneBadAuthor  BadAuthorReason = 1
	WrongWordsBadAuthor       BadAuthorReason = 2
)

type TransactionIndex struct {
	BlockHash common.Hash
	// tx index in block's body
	Idx uint32
}

func (i *TransactionIndex) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoTransactionIndex{
		BlockHash: i.BlockHash[:],
		Idx:       i.Idx,
	}
	return proto.Marshal(protoObj)
}

func (i *TransactionIndex) FromBytes(data []byte) error {
	protoObj := new(models.ProtoTransactionIndex)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	i.Idx = protoObj.Idx
	i.BlockHash = common.BytesToHash(protoObj.BlockHash)

	return nil
}

type TxEvent struct {
	EventName string
	Data      [][]byte
}

type TxReceipt struct {
	ContractAddress common.Address
	Success         bool
	GasUsed         uint64
	GasCost         *big.Int
	From            common.Address
	TxHash          common.Hash
	Error           error
	Events          []*TxEvent
	Method          string
}

type TxReceipts []*TxReceipt

func (txrs TxReceipts) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoTxReceipts{}
	for _, r := range txrs {
		protoObj.Receipts = append(protoObj.Receipts, r.ToProto())
	}
	return proto.Marshal(protoObj)
}

func (txrs TxReceipts) FromBytes(data []byte) TxReceipts {
	protoObj := new(models.ProtoTxReceipts)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return nil
	}
	return txrs.FromProto(protoObj)
}

func (txrs TxReceipts) FromProto(protoObj *models.ProtoTxReceipts) TxReceipts {
	var ret TxReceipts
	for _, r := range protoObj.Receipts {
		receipt := new(TxReceipt)
		receipt.FromProto(r)
		ret = append(ret, receipt)
	}
	return ret
}

func (r *TxReceipt) ToProto() *models.ProtoTxReceipts_ProtoTxReceipt {
	protoObj := &models.ProtoTxReceipts_ProtoTxReceipt{
		Success:  r.Success,
		Contract: r.ContractAddress.Bytes(),
		From:     r.From.Bytes(),
		GasUsed:  r.GasUsed,
		TxHash:   r.TxHash.Bytes(),
		Method:   r.Method,
	}
	if r.Error != nil {
		protoObj.Error = r.Error.Error()
	}
	if r.GasCost != nil {
		protoObj.GasCost = r.GasCost.Bytes()
	}
	for idx := range r.Events {
		e := r.Events[idx]
		protoObj.Events = append(protoObj.Events, &models.ProtoTxReceipts_ProtoEvent{
			Event: e.EventName,
			Data:  e.Data,
		})
	}
	return protoObj
}

func (r *TxReceipt) ToBytes() ([]byte, error) {
	return proto.Marshal(r.ToProto())
}

func (r *TxReceipt) FromProto(protoObj *models.ProtoTxReceipts_ProtoTxReceipt) {
	var from common.Address
	from.SetBytes(protoObj.From)
	r.From = from

	var contract common.Address
	contract.SetBytes(protoObj.Contract)
	r.ContractAddress = contract

	gasCost := big.NewInt(0)
	gasCost.SetBytes(protoObj.GasCost)
	r.GasCost = gasCost

	if protoObj.Error != "" {
		r.Error = errors.New(protoObj.Error)
	}
	r.Success = protoObj.Success
	r.GasUsed = protoObj.GasUsed
	r.Method = protoObj.Method
	var hash common.Hash
	hash.SetBytes(protoObj.TxHash)
	r.TxHash = hash

	for idx := range protoObj.Events {
		e := protoObj.Events[idx]
		r.Events = append(r.Events, &TxEvent{
			EventName: e.Event,
			Data:      e.Data,
		})
	}
}

func (r *TxReceipt) FromBytes(data []byte) error {
	protoObj := new(models.ProtoTxReceipts_ProtoTxReceipt)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	r.FromProto(protoObj)
	return nil
}

type TxReceiptIndex struct {
	ReceiptCid []byte
	// tx index in receipts
	Idx uint32
}

func (i *TxReceiptIndex) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoTxReceiptIndex{
		Cid:   i.ReceiptCid,
		Index: i.Idx,
	}
	return proto.Marshal(protoObj)
}

func (i *TxReceiptIndex) FromBytes(data []byte) error {
	protoObj := new(models.ProtoTxReceiptIndex)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	i.Idx = protoObj.Index
	i.ReceiptCid = protoObj.Cid
	return nil
}

type SavedEvent struct {
	Contract common.Address
	Event    string
	Args     [][]byte
}

func (i *SavedEvent) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoSavedEvent{
		Contract: i.Contract.Bytes(),
		Event:    i.Event,
		Args:     i.Args,
	}
	return proto.Marshal(protoObj)
}

func (i *SavedEvent) FromBytes(data []byte) error {
	protoObj := new(models.ProtoSavedEvent)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	i.Contract.SetBytes(protoObj.Contract)
	i.Event = protoObj.Event
	i.Args = protoObj.Args
	return nil
}

type GenesisInfo struct {
	// the genesis with highest height
	Genesis *Header
	// previous genesis or empty
	OldGenesis *Header
}

func (gi *GenesisInfo) EqualAny(genesis common.Hash, oldGenesis *common.Hash) bool {
	ownHash := gi.Genesis.Hash()
	var ownOldHash common.Hash
	if gi.OldGenesis != nil {
		ownOldHash = gi.OldGenesis.Hash()
	}
	if ownHash == genesis {
		return true
	}
	if oldGenesis != nil {
		if ownOldHash == *oldGenesis || ownHash == *oldGenesis {
			return true
		}
	}
	if ownOldHash == genesis {
		return true
	}
	return false
}

type UpgradeVote struct {
	Voter   common.Address
	Upgrade uint32
}

type UpgradeVotes struct {
	Dict map[common.Address]uint32
}

func NewUpgradeVotes() *UpgradeVotes {
	return &UpgradeVotes{Dict: make(map[common.Address]uint32)}
}

func (uv *UpgradeVotes) Add(voter common.Address, upgrade uint32) {
	uv.Dict[voter] = upgrade
}

func (uv *UpgradeVotes) Remove(addr common.Address) {
	delete(uv.Dict, addr)
}

func (uv *UpgradeVotes) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoUpgradeVotes{}
	for v, u := range uv.Dict {
		protoObj.Votes = append(protoObj.Votes, &models.ProtoUpgradeVotes_ProtoUpgradeVote{
			Voter:   v.Bytes(),
			Upgrade: u,
		})
	}
	return proto.Marshal(protoObj)
}

func (uv *UpgradeVotes) FromBytes(data []byte) error {
	protoObj := new(models.ProtoUpgradeVotes)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	for _, v := range protoObj.Votes {
		var addr common.Address
		addr.SetBytes(v.Voter)
		uv.Dict[addr] = v.Upgrade
	}
	return nil
}
