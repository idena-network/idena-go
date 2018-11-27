package state

import (
	"idena-go/common"
	"idena-go/rlp"
	"io"
	"math/big"
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

// empty returns whether the account is considered empty.
func (s *stateAccount) empty() bool {
	return false
	//return s.data.Nonce == 0 && s.data.Balance.Sign() == 0
}

// Account is the Idena consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce   uint64
	Balance *big.Int
	Invites uint16
}

// newObject creates a state object.
func newObject(db *StateDB, address common.Address, data Account, onDirty func(addr common.Address)) *stateAccount {
	if data.Balance == nil {
		data.Balance = new(big.Int)
	}

	return &stateAccount{
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
	stateObject := newObject(db, s.address, s.data, onDirty)
	stateObject.deleted = s.deleted
	return stateObject
}

//
// Attribute accessors
//

// Returns the address of the contract/account
func (s *stateAccount) Address() common.Address {
	return s.address
}

func (s *stateAccount) SetNonce(nonce uint64) {
	s.setNonce(nonce)
}

func (s *stateAccount) setNonce(nonce uint64) {
	s.data.Nonce = nonce
	if s.onDirty != nil {
		s.onDirty(s.Address())
		s.onDirty = nil
	}
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

func (s *stateAccount) Invites() uint16 {
	return s.data.Invites
}

func (s *stateAccount) setInvites(invites uint16) {
	s.data.Invites = invites
	if s.onDirty != nil {
		s.onDirty(s.Address())
		s.onDirty = nil
	}
}

func (s *stateAccount) SetInvites(invites uint16) {
	s.setInvites(invites)
}
