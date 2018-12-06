// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package state

import (
	"fmt"
	"idena-go/log"
	"idena-go/rlp"

	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/common"
	"math/big"
	"sync"

	"bytes"
	"sort"
)

var (
	addressPrefix = []byte("a")
)

type StateDB struct {
	db   dbm.DB
	tree Tree

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateAccounts      map[common.Address]*stateAccount
	stateAccountsDirty map[common.Address]struct{}
	log                log.Logger
	lock               sync.Mutex
}

type StakeCache struct {
	TotalValue *big.Int
	BipValue   *big.Int
}

func NewForCheck(s *StateDB) *StateDB {
	tree := NewMutableTree(s.db)
	tree.Load()
	return &StateDB{
		db:                 s.db,
		tree:               tree,
		stateAccounts:      make(map[common.Address]*stateAccount),
		stateAccountsDirty: make(map[common.Address]struct{}),
		log:                log.New(),
	}
}

func NewLazy(db dbm.DB) (*StateDB, error) {
	tree := NewMutableTree(db)

	return &StateDB{
		db:                 db,
		tree:               tree,
		stateAccounts:      make(map[common.Address]*stateAccount),
		stateAccountsDirty: make(map[common.Address]struct{}),
		log:                log.New(),
	}, nil
}

func New(height int64, db dbm.DB) (*StateDB, error) {
	tree := NewMutableTree(db)

	_, err := tree.LoadVersion(height)

	if err != nil {
		return nil, err
	}

	return &StateDB{
		db:                 db,
		tree:               tree,
		stateAccounts:      make(map[common.Address]*stateAccount),
		stateAccountsDirty: make(map[common.Address]struct{}),
		log:                log.New(),
	}, nil
}

func (s *StateDB) Load(height uint64) error {
	_, err := s.tree.LoadVersion(int64(height))
	return err
}

func (s *StateDB) Clear() {
	s.stateAccounts = make(map[common.Address]*stateAccount)
	s.stateAccountsDirty = make(map[common.Address]struct{})
	s.lock = sync.Mutex{}
}

func (s *StateDB) Version() int64 {
	return s.tree.Version()
}

// Retrieve the balance from the given address or 0 if object not found
func (s *StateDB) GetBalance(addr common.Address) *big.Int {
	stateObject := s.getStateAccount(addr)
	if stateObject != nil {
		return stateObject.Balance()
	}
	return common.Big0
}

func (s *StateDB) GetNonce(addr common.Address) uint64 {
	stateObject := s.getStateAccount(addr)
	if stateObject != nil {
		return stateObject.Nonce()
	}

	return 0
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr
func (s *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddBalance(amount)
	}
}

// SubBalance subtracts amount from the account associated with addr
func (s *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubBalance(amount)
	}
}

func (s *StateDB) SetBalance(addr common.Address, amount *big.Int) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetBalance(amount)
	}
}

func (s *StateDB) SetNonce(addr common.Address, nonce uint64) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetNonce(nonce)
	}
}

//
// Setting, updating & deleting state object methods
//

// updateStateObject writes the given object to the trie.
func (s *StateDB) updateStateObject(stateObject *stateAccount) {
	addr := stateObject.Address()
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}

	s.tree.Set(append(addressPrefix, addr[:]...), data)
}

// deleteStateObject removes the given object from the state trie.
func (s *StateDB) deleteStateObject(stateObject *stateAccount) {
	stateObject.deleted = true
	addr := stateObject.Address()

	s.tree.Remove(append(addressPrefix, addr[:]...))
}

// Retrieve a state account given my the address. Returns nil if not found.
func (s *StateDB) getStateAccount(addr common.Address) (stateObject *stateAccount) {
	// Prefer 'live' objects.
	if obj := s.stateAccounts[addr]; obj != nil {
		if obj.deleted {
			return nil
		}
		return obj
	}

	// Load the object from the database.
	_, enc := s.tree.Get(append(addressPrefix, addr[:]...))
	if len(enc) == 0 {
		return nil
	}
	var data Account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		s.log.Error("Failed to decode state object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newObject(s, addr, data, s.MarkStateObjectDirty)
	s.setStateObject(obj)
	return obj
}

func (s *StateDB) setStateObject(object *stateAccount) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateAccounts[object.Address()] = object
}

// Retrieve a state object or create a new state object if nil
func (s *StateDB) GetOrNewStateObject(addr common.Address) *stateAccount {
	stateObject := s.getStateAccount(addr)
	if stateObject == nil || stateObject.deleted {
		stateObject, _ = s.createAccount(addr)
	}
	return stateObject
}

// MarkStateObjectDirty adds the specified object to the dirty map to avoid costly
// state object cache iteration to find a handful of modified ones.
func (s *StateDB) MarkStateObjectDirty(addr common.Address) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateAccountsDirty[addr] = struct{}{}
}

func (s *StateDB) createAccount(addr common.Address) (newobj, prev *stateAccount) {
	prev = s.getStateAccount(addr)
	newobj = newObject(s, addr, Account{}, s.MarkStateObjectDirty)
	newobj.setNonce(0) // sets the object to dirty
	s.setStateObject(newobj)
	return newobj, prev
}

// Commit writes the state to the underlying in-memory trie database.
func (s *StateDB) Commit(deleteEmptyObjects bool) (root []byte, version int64, err error) {
	// Commit objects to the trie.
	for _, addr := range getOrderedObjectsKeys(s.stateAccountsDirty) {
		stateObject := s.stateAccounts[addr]
		if deleteEmptyObjects && stateObject.empty() {
			s.deleteStateObject(stateObject)
		} else {
			s.updateStateObject(stateObject)
		}
		delete(s.stateAccountsDirty, addr)
	}

	hash, version, err := s.tree.SaveVersion()
	//TODO: snapshots
	if version > 1 {
		err = s.tree.DeleteVersion(version - 1)

		if err != nil {
			panic(err)
		}
	}

	s.Clear()

	return hash, version, err
}

func (s *StateDB) Precommit(deleteEmptyObjects bool) {
	// Commit objects to the trie.
	for _, addr := range getOrderedObjectsKeys(s.stateAccountsDirty) {
		stateObject := s.stateAccounts[addr]
		if deleteEmptyObjects && stateObject.empty() {
			s.deleteStateObject(stateObject)
		} else {
			s.updateStateObject(stateObject)
		}
		delete(s.stateAccountsDirty, addr)
	}
}

func (s *StateDB) Reset() {
	s.Clear()
	s.tree = NewMutableTree(s.db)
	s.tree.Load()
}

func getOrderedObjectsKeys(objects map[common.Address]struct{}) []common.Address {
	keys := make([]common.Address, 0, len(objects))
	for k := range objects {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i].Bytes(), keys[j].Bytes()) == 1
	})

	return keys
}

func (s *StateDB) AccountExists(address common.Address) bool {
	return s.getStateAccount(address) != nil
}

func (s *StateDB) Root() common.Hash {
	return s.tree.WorkingHash()
}
