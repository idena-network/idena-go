package state

import (
	"bytes"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/rlp"
	"io"
	math2 "math"
	"math/big"
)

type IdentityState uint8

const (
	Undefined IdentityState = 0
	Invite    IdentityState = 1
	Candidate IdentityState = 2
	Verified  IdentityState = 3
	Suspended IdentityState = 4
	Killed    IdentityState = 5
	Zombie    IdentityState = 6
	Newbie    IdentityState = 7
	Human     IdentityState = 8

	MaxInvitesAmount    = math.MaxUint8
	EmptyBlocksBitsSize = 25

	AdditionalVerifiedFlips = 1
	AdditionalHumanFlips    = 2
)

func (s IdentityState) NewbieOrBetter() bool {
	return s == Newbie || s == Verified || s == Human
}

func (s IdentityState) VerifiedOrBetter() bool {
	return s == Verified || s == Human
}

// stateAccount represents an Idena account which is being modified.
//
// The usage pattern is as follows:
// First you need to obtain a state object.
// Account values can be accessed and modified through the object.
// Finally, call CommitTrie to write the modified storage trie into a database.
type stateAccount struct {
	address common.Address
	data    Account

	deleted bool
	onDirty func(addr common.Address) // Callback method to mark a state object newly dirty
}

type stateIdentity struct {
	address common.Address
	data    Identity

	deleted bool
	onDirty func(addr common.Address) // Callback method to mark a state object newly dirty
}
type stateApprovedIdentity struct {
	address common.Address
	data    ApprovedIdentity

	deleted bool
	onDirty func(addr common.Address) // Callback method to mark a state object newly dirty
}
type stateGlobal struct {
	data Global

	onDirty func(withEpoch bool) // Callback method to mark a state object newly dirty
}
type stateStatusSwitch struct {
	data IdentityStatusSwitch

	onDirty func()
}

type ValidationPeriod uint32

const (
	NonePeriod             ValidationPeriod = 0
	FlipLotteryPeriod      ValidationPeriod = 1
	ShortSessionPeriod     ValidationPeriod = 2
	LongSessionPeriod      ValidationPeriod = 3
	AfterLongSessionPeriod ValidationPeriod = 4
)

type IdentityStatusSwitch struct {
	Addresses []common.Address `rlp:"nil"`
}

type Global struct {
	Epoch                uint16
	NextValidationTime   *big.Int
	ValidationPeriod     ValidationPeriod
	GodAddress           common.Address
	WordsSeed            types.Seed `rlp:"nil"`
	LastSnapshot         uint64
	EpochBlock           uint64
	FeePerByte           *big.Int
	VrfProposerThreshold uint64
	EmptyBlocksBits      *big.Int
	GodAddressInvites    uint16
}

// Account is the Idena consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce   uint32
	Epoch   uint16
	Balance *big.Int
}

type IdentityFlip struct {
	Cid  []byte
	Pair uint8
}

type ValidationStatusFlag uint16

const (
	AtLeastOneFlipReported ValidationStatusFlag = 1 << iota
	AtLeastOneFlipNotQualified
	AllFlipsNotQualified
)

type Identity struct {
	ProfileHash    []byte `rlp:"nil"`
	Stake          *big.Int
	Invites        uint8
	Birthday       uint16
	State          IdentityState
	QualifiedFlips uint32
	// should use GetShortFlipPoints instead of reading directly
	ShortFlipPoints      uint32
	PubKey               []byte `rlp:"nil"`
	RequiredFlips        uint8
	Flips                []IdentityFlip `rlp:"nil"`
	Generation           uint32
	Code                 []byte   `rlp:"nil"`
	Invitees             []TxAddr `rlp:"nil"`
	Inviter              *TxAddr  `rlp:"nil"`
	Penalty              *big.Int
	ValidationTxsBits    byte
	LastValidationStatus ValidationStatusFlag
}

type TxAddr struct {
	TxHash  common.Hash
	Address common.Address
}

func (i *Identity) GetShortFlipPoints() float32 {
	return float32(i.ShortFlipPoints) / 2
}

func (i *Identity) GetTotalScore() float32 {
	if i.QualifiedFlips == 0 {
		return 0
	}
	return i.GetShortFlipPoints() / float32(i.QualifiedFlips)
}

func (i *Identity) HasDoneAllRequiredFlips() bool {
	return uint8(len(i.Flips)) >= i.RequiredFlips
}

func (i *Identity) GetTotalWordPairsCount() int {
	return common.WordPairsPerFlip * int(i.RequiredFlips)
}

func (i *Identity) GetMaximumAvailableFlips() uint8 {
	if i.State == Verified {
		return i.RequiredFlips + AdditionalVerifiedFlips
	}
	if i.State == Human {
		return i.RequiredFlips + AdditionalHumanFlips
	}
	return i.RequiredFlips
}

type ApprovedIdentity struct {
	Approved bool
	Online   bool
}

// newAccountObject creates a state object.
func newAccountObject(address common.Address, data Account, onDirty func(addr common.Address)) *stateAccount {
	if data.Balance == nil {
		data.Balance = new(big.Int)
	}

	return &stateAccount{
		address: address,
		data:    data,
		onDirty: onDirty,
	}
}

// newAccountObject creates a state object.
func newIdentityObject(address common.Address, data Identity, onDirty func(addr common.Address)) *stateIdentity {

	return &stateIdentity{
		address: address,
		data:    data,
		onDirty: onDirty,
	}
}

// newGlobalObject creates a global state object.
func newGlobalObject(data Global, onDirty func(withEpoch bool)) *stateGlobal {
	return &stateGlobal{
		data:    data,
		onDirty: onDirty,
	}
}

func newStatusSwitchObject(data IdentityStatusSwitch, onDirty func()) *stateStatusSwitch {
	return &stateStatusSwitch{
		data:    data,
		onDirty: onDirty,
	}
}

func newApprovedIdentityObject(address common.Address, data ApprovedIdentity, onDirty func(addr common.Address)) *stateApprovedIdentity {
	return &stateApprovedIdentity{
		address: address,
		data:    data,
		onDirty: onDirty,
	}
}

// EncodeRLP implements rlp.Encoder.
func (s *stateAccount) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, s.data)
}

func (s *stateAccount) touch() {
	if s.onDirty != nil {
		s.onDirty(s.Address())
		s.onDirty = nil
	}
}

// AddBalance removes amount from c's balance.
// It is used to add funds to the destination account of a transfer.
func (s *stateAccount) AddBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		if s.empty() {
			s.touch()
		}

		return
	}
	s.SetBalance(new(big.Int).Add(s.Balance(), amount))
}

// SubBalance removes amount from c's balance.
// It is used to remove funds from the origin account of a transfer.
func (s *stateAccount) SubBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	s.SetBalance(new(big.Int).Sub(s.Balance(), amount))
}

func (s *stateAccount) SetBalance(amount *big.Int) {
	s.setBalance(amount)
}

func (s *stateAccount) setBalance(amount *big.Int) {

	if s.data.Balance == nil {
		s.data.Balance = new(big.Int)
	}
	s.data.Balance = amount
	s.touch()
}

func (s *stateAccount) deepCopy(db *StateDB, onDirty func(addr common.Address)) *stateAccount {
	stateObject := newAccountObject(s.address, s.data, onDirty)
	stateObject.deleted = s.deleted
	return stateObject
}

// empty returns whether the account is considered empty.
func (s *stateAccount) empty() bool {
	return s.data.Balance.Sign() == 0 && s.data.Nonce == 0
}

// Returns the address of the contract/account
func (s *stateAccount) Address() common.Address {
	return s.address
}

func (s *stateAccount) SetNonce(nonce uint32) {
	s.setNonce(nonce)
}

func (s *stateAccount) setNonce(nonce uint32) {
	s.data.Nonce = nonce
	s.touch()
}

func (s *stateAccount) SetEpoch(nonce uint16) {
	s.setEpoch(nonce)
}

func (s *stateAccount) setEpoch(nonce uint16) {
	s.data.Epoch = nonce
	s.touch()
}

func (s *stateAccount) Balance() *big.Int {

	if s.data.Balance == nil {
		return big.NewInt(0)
	}
	return s.data.Balance
}

func (s *stateAccount) Nonce() uint32 {
	return s.data.Nonce
}

func (s *stateAccount) Epoch() uint16 {
	return s.data.Epoch
}

// Returns the address of the contract/account
func (s *stateIdentity) Address() common.Address {
	return s.address
}

func (s *stateIdentity) Invites() uint8 {
	return s.data.Invites
}

func (s *stateIdentity) SetInvites(invites uint8) {
	s.setInvites(invites)
}

func (s *stateIdentity) setInvites(invites uint8) {
	s.data.Invites = invites
	s.touch()
}

// empty returns whether the account is considered empty.
func (s *stateIdentity) empty() bool {
	return s.data.State == Killed
}

func (s *stateIdentity) touch() {
	if s.onDirty != nil {
		s.onDirty(s.Address())
		s.onDirty = nil
	}
}

func (s *stateIdentity) State() IdentityState {
	return s.data.State
}

func (s *stateIdentity) SetState(state IdentityState) {
	s.data.State = state
	s.touch()
}

func (s *stateIdentity) SetGeneticCode(generation uint32, code []byte) {
	s.data.Generation = generation
	s.data.Code = code
	s.touch()
}

func (s *stateIdentity) GeneticCode() (generation uint32, code []byte) {
	return s.data.Generation, s.data.Code
}

func (s *stateIdentity) Stake() *big.Int {
	if s.data.Stake == nil {
		return big.NewInt(0)
	}
	return s.data.Stake
}

// EncodeRLP implements rlp.Encoder.
func (s *stateIdentity) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, s.data)
}
func (s *stateIdentity) AddStake(amount *big.Int) {
	if amount.Sign() == 0 {
		if s.empty() {
			s.touch()
		}
		return
	}
	s.SetStake(new(big.Int).Add(s.Stake(), amount))
}

func (s *stateIdentity) SubStake(amount *big.Int) {
	if amount.Sign() == 0 {
		if s.empty() {
			s.touch()
		}
		return
	}

	s.SetStake(new(big.Int).Sub(s.Stake(), amount))
}

func (s *stateIdentity) SetStake(amount *big.Int) {
	if s.data.Stake == nil {
		s.data.Stake = new(big.Int)
	}

	s.data.Stake = amount
	s.touch()
}

func (s *stateIdentity) AddInvite(i uint8) {
	if s.Invites() == MaxInvitesAmount {
		return
	}
	s.SetInvites(s.Invites() + i)
}
func (s *stateIdentity) SubInvite(i uint8) {
	s.SetInvites(s.Invites() - i)
}

func (s *stateIdentity) SetPubKey(pubKey []byte) {
	s.data.PubKey = pubKey
	s.touch()
}

func (s *stateIdentity) AddQualifiedFlipsCount(qualifiedFlips uint32) {
	s.data.QualifiedFlips += qualifiedFlips
	s.touch()
}

func (s *stateIdentity) QualifiedFlipsCount() uint32 {
	return s.data.QualifiedFlips
}

func (s *stateIdentity) AddShortFlipPoints(flipPoints float32) {
	s.data.ShortFlipPoints += uint32(flipPoints * 2)
	s.touch()
}

func (s *stateIdentity) ShortFlipPoints() float32 {
	return s.data.GetShortFlipPoints()
}

func (s *stateIdentity) GetRequiredFlips() uint8 {
	return s.data.RequiredFlips
}

func (s *stateIdentity) SetRequiredFlips(amount uint8) {
	s.data.RequiredFlips = amount
	s.touch()
}

func (s *stateIdentity) GetMadeFlips() uint8 {
	return uint8(len(s.data.Flips))
}

func (s *stateIdentity) AddFlip(cid []byte, pair uint8) {
	if len(s.data.Flips) < math.MaxUint8 {
		s.data.Flips = append(s.data.Flips, IdentityFlip{
			Cid:  cid,
			Pair: pair,
		})
		s.touch()
	}
}

func (s *stateIdentity) DeleteFlip(cid []byte) {
	for i, flip := range s.data.Flips {
		if bytes.Compare(flip.Cid, cid) != 0 {
			continue
		}
		s.data.Flips = append(s.data.Flips[:i], s.data.Flips[i+1:]...)
		s.touch()
		return
	}
}

func (s *stateIdentity) ClearFlips() {
	s.data.Flips = nil
	s.touch()
}

func (s *stateIdentity) SetInviter(address common.Address, txHash common.Hash) {
	s.data.Inviter = &TxAddr{
		TxHash:  txHash,
		Address: address,
	}
	s.touch()
}

func (s *stateIdentity) GetInviter() *TxAddr {
	return s.data.Inviter
}

func (s *stateIdentity) ResetInviter() {
	s.data.Inviter = nil
	s.touch()
}

func (s *stateIdentity) AddInvitee(address common.Address, txHash common.Hash) {
	s.data.Invitees = append(s.data.Invitees, TxAddr{
		TxHash:  txHash,
		Address: address,
	})
	s.touch()
}

func (s *stateIdentity) GetInvitees() []TxAddr {
	return s.data.Invitees
}

func (s *stateIdentity) RemoveInvitee(address common.Address) {
	if len(s.data.Invitees) == 0 {
		return
	}
	for i, invitee := range s.data.Invitees {
		if invitee.Address != address {
			continue
		}
		s.data.Invitees = append(s.data.Invitees[:i], s.data.Invitees[i+1:]...)
		s.touch()
		return
	}
}

func (s *stateIdentity) SetBirthday(birthday uint16) {
	s.data.Birthday = birthday
	s.touch()
}

func (s *stateIdentity) SetPenalty(penalty *big.Int) {
	s.data.Penalty = penalty
	s.touch()
}

func (s *stateIdentity) SubPenalty(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	p := s.data.Penalty

	if p.Cmp(amount) <= 0 {
		s.SetPenalty(nil)
	} else {
		s.SetPenalty(new(big.Int).Sub(s.data.Penalty, amount))
	}
}

func (s *stateIdentity) GetPenalty() *big.Int {
	return s.data.Penalty
}

func (s *stateIdentity) SetProfileHash(hash []byte) {
	s.data.ProfileHash = hash
	s.touch()
}

func (s *stateIdentity) GetProfileHash() []byte {
	return s.data.ProfileHash
}

func (s *stateIdentity) SetValidationTxBit(txType types.TxType) {
	mask := validationTxBitMask(txType)
	if mask == 0 {
		return
	}
	s.data.ValidationTxsBits = s.data.ValidationTxsBits | mask
	s.touch()
}

func (s *stateIdentity) HasValidationTx(txType types.TxType) bool {
	mask := validationTxBitMask(txType)
	if mask == 0 {
		return false
	}
	return s.data.ValidationTxsBits&mask > 0
}

func (s *stateIdentity) ResetValidationTxBits() {
	s.data.ValidationTxsBits = 0
	s.touch()
}

func (s *stateIdentity) SetValidationStatus(status ValidationStatusFlag) {
	s.data.LastValidationStatus = status
	s.touch()
}

// EncodeRLP implements rlp.Encoder.
func (s *stateGlobal) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, s.data)
}

func (s *stateGlobal) Epoch() uint16 {
	return s.data.Epoch
}

func (s *stateGlobal) IncEpoch() {
	s.data.Epoch++
	s.touch(true)
}

func (s *stateGlobal) VrfProposerThreshold() float64 {
	return math2.Float64frombits(s.data.VrfProposerThreshold)
}

func (s *stateGlobal) VrfProposerThresholdRaw() uint64 {
	return s.data.VrfProposerThreshold
}

func (s *stateGlobal) SetVrfProposerThreshold(value float64) {
	s.data.VrfProposerThreshold = math2.Float64bits(value)
	s.touch(false)
}

func (s *stateGlobal) AddBlockBit(empty bool) {
	if s.data.EmptyBlocksBits == nil {
		s.data.EmptyBlocksBits = new(big.Int)
	}
	s.data.EmptyBlocksBits.Lsh(s.data.EmptyBlocksBits, 1)
	if !empty {
		s.data.EmptyBlocksBits.SetBit(s.data.EmptyBlocksBits, 0, 1)
	}
	s.data.EmptyBlocksBits.SetBit(s.data.EmptyBlocksBits, EmptyBlocksBitsSize, 0)
	s.touch(false)
}

func (s *stateGlobal) EmptyBlocksCount() int {
	cnt := 0
	for i := 0; i < EmptyBlocksBitsSize; i++ {
		if s.data.EmptyBlocksBits.Bit(i) == 0 {
			cnt++
		}
	}
	return cnt
}

func (s *stateGlobal) EmptyBlocksBits() *big.Int {
	return s.data.EmptyBlocksBits
}

func (s *stateGlobal) LastSnapshot() uint64 {
	return s.data.LastSnapshot
}

func (s *stateGlobal) SetLastSnapshot(height uint64) {
	s.data.LastSnapshot = height
	s.touch(false)
}

func (s *stateGlobal) ValidationPeriod() ValidationPeriod {
	return s.data.ValidationPeriod
}

func (s *stateGlobal) touch(withEpoch bool) {
	if s.onDirty != nil {
		s.onDirty(withEpoch)
	}
}

func (s *stateGlobal) NextValidationTime() *big.Int {
	return s.data.NextValidationTime
}

func (s *stateGlobal) SetNextValidationTime(unix int64) {
	s.data.NextValidationTime = big.NewInt(unix)
	s.touch(false)
}

func (s *stateGlobal) SetValidationPeriod(period ValidationPeriod) {
	s.data.ValidationPeriod = period
	s.touch(false)
}

func (s *stateGlobal) SetGodAddress(godAddress common.Address) {
	s.data.GodAddress = godAddress
	s.touch(false)
}

func (s *stateGlobal) GodAddress() common.Address {
	return s.data.GodAddress
}

func (s *stateGlobal) SetFlipWordsSeed(seed types.Seed) {
	s.data.WordsSeed = seed
	s.touch(false)
}

func (s *stateGlobal) FlipWordsSeed() types.Seed {
	return s.data.WordsSeed
}

func (s *stateGlobal) SetEpoch(epoch uint16) {
	s.data.Epoch = epoch
	s.touch(true)
}

func (s *stateGlobal) SetEpochBlock(height uint64) {
	s.data.EpochBlock = height
	s.touch(false)
}

func (s *stateGlobal) EpochBlock() uint64 {
	return s.data.EpochBlock
}

func (s *stateGlobal) SetFeePerByte(fee *big.Int) {
	s.data.FeePerByte = fee
	s.touch(false)
}

func (s *stateGlobal) FeePerByte() *big.Int {
	return s.data.FeePerByte
}

func (s *stateGlobal) SubGodAddressInvite() {
	s.data.GodAddressInvites -= 1
	s.touch(false)
}

func (s *stateGlobal) GodAddressInvites() uint16 {
	return s.data.GodAddressInvites
}

func (s *stateGlobal) SetGodAddressInvites(count uint16) {
	s.data.GodAddressInvites = count
	s.touch(false)
}

// EncodeRLP implements rlp.Encoder.
func (s *stateApprovedIdentity) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, s.data)
}

func (s *stateApprovedIdentity) Address() common.Address {
	return s.address
}

func (s *stateApprovedIdentity) Online() bool {
	return s.data.Online
}

// empty returns whether the account is considered empty.
func (s *stateApprovedIdentity) empty() bool {
	return !s.data.Approved
}

func (s *stateApprovedIdentity) touch() {
	if s.onDirty != nil {
		s.onDirty(s.Address())
		s.onDirty = nil
	}
}

func (s *stateApprovedIdentity) SetState(approved bool) {
	s.data.Approved = approved
	s.touch()
}

func (s *stateApprovedIdentity) SetOnline(online bool) {
	s.data.Online = online
	s.touch()
}

func IsCeremonyCandidate(identity Identity) bool {
	state := identity.State
	return (state == Candidate || state.NewbieOrBetter() || state == Suspended ||
		state == Zombie) && identity.HasDoneAllRequiredFlips()
}

// EncodeRLP implements rlp.Encoder.
func (s *stateStatusSwitch) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, s.data)
}

func (s *stateStatusSwitch) Addresses() []common.Address {
	return s.data.Addresses
}

func (s *stateStatusSwitch) Clear() {
	s.data.Addresses = nil
	s.touch()
}

func (s *stateStatusSwitch) ToggleAddress(sender common.Address) {
	defer s.touch()
	for i := 0; i < len(s.data.Addresses); i++ {
		if s.data.Addresses[i] == sender {
			s.data.Addresses = append(s.data.Addresses[:i], s.data.Addresses[i+1:]...)
			return
		}
	}
	s.data.Addresses = append(s.data.Addresses, sender)
}

func (s *stateStatusSwitch) HasAddress(addr common.Address) bool {
	for _, item := range s.data.Addresses {
		if item == addr {
			return true
		}
	}
	return false
}

func (s *stateStatusSwitch) touch() {
	if s.onDirty != nil {
		s.onDirty()
	}
}

func (f ValidationStatusFlag) HasFlag(flag ValidationStatusFlag) bool {
	return f&flag != 0
}
