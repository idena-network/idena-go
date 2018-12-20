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
	Candidate IdentityState = 0
	Verified  IdentityState = 1
	Suspended IdentityState = 2
	Killed    IdentityState = 3

	MaxInvitesAmount = math.MaxUint8
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

// Account is the Idena consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce   uint64
	Balance *big.Int
}

type Identity struct {
	Nickname *[64]byte `rlp:"nil"`
	Stake    *big.Int
	Invites  uint8
	Age      uint16
	State    IdentityState
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
	return s.data.Balance.Sign() == 0
}

// Returns the address of the contract/account
func (s *stateAccount) Address() common.Address {
	return s.address
}

func (s *stateAccount) SetNonce(nonce uint64) {
	s.setNonce(nonce)
}

func (s *stateAccount) setNonce(nonce uint64) {
	s.data.Nonce = nonce
	s.touch()
}

func (s *stateAccount) Balance() *big.Int {

	if s.data.Balance == nil {
		return big.NewInt(0)
	}
	return s.data.Balance
}

func (s *stateAccount) Nonce() uint64 {
	return s.data.Nonce
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

func (s *stateIdentity) Approve() {
	s.data.State = Verified
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
func (s *stateIdentity) Revoke() {
	s.data.State = Suspended
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
