package state

import (
	"idena-go/common"
	"idena-go/common/math"
	"idena-go/rlp"
	"io"
	"math/big"
)

type IdentityState uint8

const (
	Undefined        IdentityState = 0
	Invite           IdentityState = 1
	Candidate        IdentityState = 2
	Verified         IdentityState = 3
	Suspended        IdentityState = 4
	Killed           IdentityState = 5
	Zombie           IdentityState = 6
	Newbie           IdentityState = 7
	MaxInvitesAmount               = math.MaxUint8
)

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

	onDirty func() // Callback method to mark a state object newly dirty
}

type ValidationPeriod uint32

const (
	NonePeriod             ValidationPeriod = 0
	FlipLotteryPeriod      ValidationPeriod = 1
	ShortSessionPeriod     ValidationPeriod = 2
	LongSessionPeriod      ValidationPeriod = 3
	AfterLongSessionPeriod ValidationPeriod = 4
)

type Global struct {
	Epoch              uint16
	Flips              [][]byte
	NextValidationTime *big.Int
	ValidationPeriod   ValidationPeriod
	GodAddress         common.Address
}

// Account is the Idena consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce   uint32
	Epoch   uint16
	Balance *big.Int
}

type Identity struct {
	Nickname        *[64]byte `rlp:"nil"`
	Stake           *big.Int
	Invites         uint8
	Age             uint16
	State           IdentityState
	QualifiedFlips  uint32
	ShortFlipPoints uint32
	PubKey          []byte `rlp:"nil"`
	RequiredFlips   uint8
	MadeFlips       uint8
}

type ApprovedIdentity struct {
	Approved bool
}

// newAccountObject creates a state object.
func newAccountObject(db *StateDB, address common.Address, data Account, onDirty func(addr common.Address)) *stateAccount {
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
func newIdentityObject(db *StateDB, address common.Address, data Identity, onDirty func(addr common.Address)) *stateIdentity {

	return &stateIdentity{
		address: address,
		data:    data,
		onDirty: onDirty,
	}
}

// newGlobalObject creates a global state object.
func newGlobalObject(db *StateDB, data Global, onDirty func()) *stateGlobal {

	return &stateGlobal{
		data:    data,
		onDirty: onDirty,
	}
}
func newApprovedIdentityObject(db *IdentityStateDB, address common.Address, data ApprovedIdentity, onDirty func(addr common.Address)) *stateApprovedIdentity {
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
	if s.onDirty != nil {
		s.onDirty(s.Address())
		s.onDirty = nil
	}
}

func (s *stateAccount) deepCopy(db *StateDB, onDirty func(addr common.Address)) *stateAccount {
	stateObject := newAccountObject(db, s.address, s.data, onDirty)
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
	return float32(s.data.ShortFlipPoints) / 2
}

func (s *stateIdentity) GetRequiredFlips() uint8 {
	return s.data.RequiredFlips
}

func (s *stateIdentity) SetRequiredFlips(amount uint8) {
	s.data.RequiredFlips = amount
	s.touch()
}

func (s *stateIdentity) GetMadeFlips() uint8 {
	return s.data.MadeFlips
}

func (s *stateIdentity) SetMadeFlips(amount uint8) {
	s.data.MadeFlips = amount
	s.touch()
}

func (s *stateIdentity) AddMadeFlips(amount uint8) {
	sum := s.data.MadeFlips + amount
	if (sum > s.data.MadeFlips) == (amount > 0) {
		s.data.MadeFlips = sum
		s.touch()
	}
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
	s.touch()
}

func (s *stateGlobal) AddFlipCid(flipCid []byte) {
	s.data.Flips = append(s.data.Flips, flipCid)
	s.touch()
}

func (s *stateGlobal) ClearFlipCids() {
	s.data.Flips = [][]byte{}
	s.touch()
}

func (s *stateGlobal) ValidationPeriod() ValidationPeriod {
	return s.data.ValidationPeriod
}

func (s *stateGlobal) touch() {
	if s.onDirty != nil {
		s.onDirty()
	}
}

func (s *stateGlobal) SetNextValidationTime(unix int64) {
	s.data.NextValidationTime = big.NewInt(unix)
	s.touch()
}

func (s *stateGlobal) SetValidationPeriod(period ValidationPeriod) {
	s.data.ValidationPeriod = period
	s.touch()
}

func (s *stateGlobal) SetGodAddress(godAddress common.Address) {
	s.data.GodAddress = godAddress
	s.touch()
}

func (s *stateGlobal) GodAddress() common.Address {
	return s.data.GodAddress
}

// EncodeRLP implements rlp.Encoder.
func (s *stateApprovedIdentity) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, s.data)
}

func (s *stateApprovedIdentity) Address() common.Address {
	return s.address
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

func IsCeremonyCandidate(identity Identity) bool {
	state := identity.State
	return (state == Candidate || state == Newbie ||
		state == Verified || state == Suspended ||
		state == Zombie) && identity.MadeFlips >= identity.RequiredFlips
}
