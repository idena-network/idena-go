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
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"time"

	"github.com/idena-network/idena-go/common"
	dbm "github.com/tendermint/tendermint/libs/db"
	"math/big"
	"sync"

	"bytes"
	"sort"
)

const (
	MaxSavedStatesCount = 100
	GeneticCodeSize     = 12
)

var (
	addressPrefix  = []byte("a")
	identityPrefix = []byte("i")
	globalPrefix   = []byte("global")
)

type StateDB struct {
	db   dbm.DB
	tree Tree

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateAccounts        map[common.Address]*stateAccount
	stateAccountsDirty   map[common.Address]struct{}
	stateIdentities      map[common.Address]*stateIdentity
	stateIdentitiesDirty map[common.Address]struct{}

	stateGlobal      *stateGlobal
	stateGlobalDirty bool

	log  log.Logger
	lock sync.Mutex
}

func NewLazy(db dbm.DB) *StateDB {
	pdb := dbm.NewPrefixDB(db, database.StateDbPrefix)
	tree := NewMutableTree(pdb)
	return &StateDB{
		db:                   pdb,
		tree:                 tree,
		stateAccounts:        make(map[common.Address]*stateAccount),
		stateAccountsDirty:   make(map[common.Address]struct{}),
		stateIdentities:      make(map[common.Address]*stateIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}
}

func (s *StateDB) ForCheck(height uint64) (*StateDB, error) {
	db := database.NewBackedMemDb(s.db)
	tree := NewMutableTree(db)
	if _, err := tree.LoadVersionForOverwriting(int64(height)); err != nil {
		return nil, err
	}
	return &StateDB{
		db:                   db,
		tree:                 tree,
		stateAccounts:        make(map[common.Address]*stateAccount),
		stateAccountsDirty:   make(map[common.Address]struct{}),
		stateIdentities:      make(map[common.Address]*stateIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}, nil
}

func (s *StateDB) Readonly(height uint64) (*StateDB, error) {
	db := database.NewBackedMemDb(s.db)
	tree := NewMutableTree(db)
	if _, err := tree.LoadVersion(int64(height)); err != nil {
		return nil, err
	}
	return &StateDB{
		db:                   db,
		tree:                 tree,
		stateAccounts:        make(map[common.Address]*stateAccount),
		stateAccountsDirty:   make(map[common.Address]struct{}),
		stateIdentities:      make(map[common.Address]*stateIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}, nil
}

func (s *StateDB) MemoryState() *StateDB {
	tree := NewMutableTree(s.db)
	tree.Load()
	return &StateDB{
		db:                   s.db,
		tree:                 tree.GetImmutable(),
		stateAccounts:        make(map[common.Address]*stateAccount),
		stateAccountsDirty:   make(map[common.Address]struct{}),
		stateIdentities:      make(map[common.Address]*stateIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}
}

func (s *StateDB) Load(height uint64) error {
	_, err := s.tree.LoadVersion(int64(height))
	return err
}

func (s *StateDB) Clear() {
	s.stateAccounts = make(map[common.Address]*stateAccount)
	s.stateAccountsDirty = make(map[common.Address]struct{})
	s.stateIdentities = make(map[common.Address]*stateIdentity)
	s.stateIdentitiesDirty = make(map[common.Address]struct{})
	s.stateGlobal = nil
	s.stateGlobalDirty = false
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

func (s *StateDB) GetNonce(addr common.Address) uint32 {
	stateObject := s.getStateAccount(addr)
	if stateObject != nil {
		return stateObject.Nonce()
	}

	return 0
}

func (s *StateDB) GetStakeBalance(addr common.Address) *big.Int {
	stateObject := s.getStateIdentity(addr)
	if stateObject != nil {
		return stateObject.Stake()
	}
	return common.Big0
}

func (s *StateDB) GetEpoch(addr common.Address) uint16 {
	stateObject := s.getStateAccount(addr)
	if stateObject != nil {
		return stateObject.Epoch()
	}

	return 0
}

func (s *StateDB) Epoch() uint16 {
	stateObject := s.GetOrNewGlobalObject()
	return stateObject.data.Epoch
}

func (s *StateDB) NextValidationTime() time.Time {
	stateObject := s.GetOrNewGlobalObject()
	if stateObject.data.NextValidationTime == nil {
		return time.Unix(0, 0)
	}
	return time.Unix(stateObject.data.NextValidationTime.Int64(), 0)
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr
func (s *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	stateObject := s.GetOrNewAccountObject(addr)
	if stateObject != nil {
		stateObject.AddBalance(amount)
	}
}

// SubBalance subtracts amount from the account associated with addr
func (s *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	stateObject := s.GetOrNewAccountObject(addr)
	if stateObject != nil {
		stateObject.SubBalance(amount)
	}
}

func (s *StateDB) SetBalance(addr common.Address, amount *big.Int) {
	stateObject := s.GetOrNewAccountObject(addr)
	if stateObject != nil {
		stateObject.SetBalance(amount)
	}
}

func (s *StateDB) SetNonce(addr common.Address, nonce uint32) {
	stateObject := s.GetOrNewAccountObject(addr)
	if stateObject != nil {
		stateObject.SetNonce(nonce)
	}
}

func (s *StateDB) SetEpoch(addr common.Address, epoch uint16) {
	stateObject := s.GetOrNewAccountObject(addr)
	if stateObject != nil {
		stateObject.SetEpoch(epoch)
	}
}

func (s *StateDB) SetNextValidationTime(t time.Time) {
	s.GetOrNewGlobalObject().SetNextValidationTime(t.Unix())
}

func (s *StateDB) SetValidationPeriod(period ValidationPeriod) {
	s.GetOrNewGlobalObject().SetValidationPeriod(period)
}

func (s *StateDB) SetGodAddress(godAddress common.Address) {
	s.GetOrNewGlobalObject().SetGodAddress(godAddress)
}

func (s *StateDB) GodAddress() common.Address {
	return s.GetOrNewGlobalObject().GodAddress()
}

func (s *StateDB) AddStake(address common.Address, intStake *big.Int) {
	s.GetOrNewIdentityObject(address).AddStake(intStake)
}

func (s *StateDB) SetState(address common.Address, state IdentityState) {
	s.GetOrNewIdentityObject(address).SetState(state)
}

func (s *StateDB) SetGeneticCode(address common.Address, generation uint32, code []byte) {
	s.GetOrNewIdentityObject(address).SetGeneticCode(generation, code)
}

func (s *StateDB) GeneticCode(address common.Address) (generation uint32, code []byte) {
	gen, c := s.GetOrNewIdentityObject(address).GeneticCode()
	if gen == 0 {
		return gen, address[:GeneticCodeSize]
	}
	return gen, c
}

func (s *StateDB) AddInvite(address common.Address, amount uint8) {
	s.GetOrNewIdentityObject(address).AddInvite(amount)
}

func (s *StateDB) SetInvites(address common.Address, amount uint8) {
	s.GetOrNewIdentityObject(address).SetInvites(amount)
}

func (s *StateDB) SubInvite(address common.Address, amount uint8) {
	s.GetOrNewIdentityObject(address).SubInvite(amount)
}
func (s *StateDB) SetPubKey(address common.Address, pubKey []byte) {
	s.GetOrNewIdentityObject(address).SetPubKey(pubKey)
}

func (s *StateDB) GetRequiredFlips(addr common.Address) uint8 {
	return s.GetOrNewIdentityObject(addr).GetRequiredFlips()
}

func (s *StateDB) SetRequiredFlips(addr common.Address, amount uint8) {
	s.GetOrNewIdentityObject(addr).SetRequiredFlips(amount)
}

func (s *StateDB) GetMadeFlips(addr common.Address) uint8 {
	return s.GetOrNewIdentityObject(addr).GetMadeFlips()
}

func (s *StateDB) AddFlip(addr common.Address, cid []byte) {
	s.GetOrNewIdentityObject(addr).AddFlip(cid)
}

func (s *StateDB) ClearFlips(addr common.Address) {
	s.GetOrNewIdentityObject(addr).ClearFlips()
}

func (s *StateDB) AddQualifiedFlipsCount(address common.Address, qualifiedFlips uint32) {
	s.GetOrNewIdentityObject(address).AddQualifiedFlipsCount(qualifiedFlips)
}

func (s *StateDB) AddShortFlipPoints(address common.Address, shortFlipPoints float32) {
	s.GetOrNewIdentityObject(address).AddShortFlipPoints(shortFlipPoints)
}

func (s *StateDB) IncEpoch() {
	s.GetOrNewGlobalObject().IncEpoch()
}

func (s *StateDB) ValidationPeriod() ValidationPeriod {
	return s.GetOrNewGlobalObject().ValidationPeriod()
}

func (s *StateDB) FlipWordsSeed() types.Seed {
	return s.GetOrNewGlobalObject().FlipWordsSeed()
}

func (s *StateDB) SetFlipWordsSeed(seed types.Seed) {
	s.GetOrNewGlobalObject().SetFlipWordsSeed(seed)
}

//
// Setting, updating & deleting state object methods
//

// updateStateAccountObject writes the given object to the trie.
func (s *StateDB) updateStateAccountObject(stateObject *stateAccount) {
	addr := stateObject.Address()
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}

	s.tree.Set(append(addressPrefix, addr[:]...), data)
}

// updateStateAccountObject writes the given object to the trie.
func (s *StateDB) updateStateIdentityObject(stateObject *stateIdentity) {
	addr := stateObject.Address()
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}

	s.tree.Set(append(identityPrefix, addr[:]...), data)
}

// updateStateAccountObject writes the given object to the trie.
func (s *StateDB) updateStateGlobalObject(stateObject *stateGlobal) {
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object, %v", err))
	}

	s.tree.Set(globalPrefix, data)
}

// deleteStateAccountObject removes the given object from the state trie.
func (s *StateDB) deleteStateAccountObject(stateObject *stateAccount) {
	stateObject.deleted = true
	addr := stateObject.Address()

	s.tree.Remove(append(addressPrefix, addr[:]...))
}

// deleteStateAccountObject removes the given object from the state trie.
func (s *StateDB) deleteStateIdentityObject(stateObject *stateIdentity) {
	stateObject.deleted = true
	addr := stateObject.Address()

	s.tree.Remove(append(identityPrefix, addr[:]...))
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
		s.log.Error("Failed to decode state account object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newAccountObject(s, addr, data, s.MarkStateAccountObjectDirty)
	s.setStateAccountObject(obj)
	return obj
}

// Retrieve a state account given my the address. Returns nil if not found.
func (s *StateDB) getStateIdentity(addr common.Address) (stateObject *stateIdentity) {
	// Prefer 'live' objects.
	if obj := s.stateIdentities[addr]; obj != nil {
		if obj.deleted {
			return nil
		}
		return obj
	}

	// Load the object from the database.
	_, enc := s.tree.Get(append(identityPrefix, addr[:]...))
	if len(enc) == 0 {
		return nil
	}
	var data Identity
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		s.log.Error("Failed to decode state identity object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newIdentityObject(s, addr, data, s.MarkStateIdentityObjectDirty)
	s.setStateIdentityObject(obj)
	return obj
}

// Retrieve a state account given my the address. Returns nil if not found.
func (s *StateDB) getStateGlobal() (stateObject *stateGlobal) {
	// Prefer 'live' objects.
	if obj := s.stateGlobal; obj != nil {
		return obj
	}

	// Load the object from the database.
	_, enc := s.tree.Get(globalPrefix)
	if len(enc) == 0 {
		return nil
	}
	var data Global
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		s.log.Error("Failed to decode state global object", "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newGlobalObject(s, data, s.MarkStateGlobalObjectDirty)
	s.setStateGlobalObject(obj)
	return obj
}

func (s *StateDB) setStateAccountObject(object *stateAccount) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateAccounts[object.Address()] = object
}

func (s *StateDB) setStateIdentityObject(object *stateIdentity) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateIdentities[object.Address()] = object
}

func (s *StateDB) setStateGlobalObject(object *stateGlobal) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateGlobal = object
}

// Retrieve a state object or create a new state object if nil
func (s *StateDB) GetOrNewAccountObject(addr common.Address) *stateAccount {
	stateObject := s.getStateAccount(addr)
	if stateObject == nil || stateObject.deleted {
		stateObject, _ = s.createAccount(addr)
	}
	return stateObject
}

// Retrieve a state object or create a new state object if nil
func (s *StateDB) GetOrNewIdentityObject(addr common.Address) *stateIdentity {
	stateObject := s.getStateIdentity(addr)
	if stateObject == nil || stateObject.deleted {
		stateObject, _ = s.createIdentity(addr)
	}
	return stateObject
}

// Retrieve a state object or create a new state object if nil
func (s *StateDB) GetOrNewGlobalObject() *stateGlobal {
	stateObject := s.getStateGlobal()
	if stateObject == nil {
		stateObject = s.createGlobal()
	}
	return stateObject
}

// MarkStateAccountObjectDirty adds the specified object to the dirty map to avoid costly
// state object cache iteration to find a handful of modified ones.
func (s *StateDB) MarkStateAccountObjectDirty(addr common.Address) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateAccountsDirty[addr] = struct{}{}
}

// MarkStateAccountObjectDirty adds the specified object to the dirty map to avoid costly
// state object cache iteration to find a handful of modified ones.
func (s *StateDB) MarkStateIdentityObjectDirty(addr common.Address) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateIdentitiesDirty[addr] = struct{}{}
}

// MarkStateAccountObjectDirty adds the specified object to the dirty map to avoid costly
// state object cache iteration to find a handful of modified ones.
func (s *StateDB) MarkStateGlobalObjectDirty() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateGlobalDirty = true
}

func (s *StateDB) createAccount(addr common.Address) (newobj, prev *stateAccount) {
	prev = s.getStateAccount(addr)
	newobj = newAccountObject(s, addr, Account{}, s.MarkStateAccountObjectDirty)
	newobj.setNonce(0) // sets the object to dirty
	s.setStateAccountObject(newobj)
	return newobj, prev
}

func (s *StateDB) createIdentity(addr common.Address) (newobj, prev *stateIdentity) {
	prev = s.getStateIdentity(addr)
	newobj = newIdentityObject(s, addr, Identity{}, s.MarkStateIdentityObjectDirty)
	newobj.touch()
	s.setStateIdentityObject(newobj)
	return newobj, prev
}

func (s *StateDB) createGlobal() (stateObject *stateGlobal) {
	stateObject = newGlobalObject(s, Global{}, s.MarkStateGlobalObjectDirty)
	stateObject.touch()
	s.setStateGlobalObject(stateObject)
	return stateObject
}

// Commit writes the state to the underlying in-memory trie database.
func (s *StateDB) Commit(deleteEmptyObjects bool) (root []byte, version int64, err error) {

	s.Precommit(deleteEmptyObjects)

	hash, version, err := s.tree.SaveVersion()
	//TODO: snapshots
	if version > MaxSavedStatesCount {
		if s.tree.ExistVersion(version - MaxSavedStatesCount) {
			err = s.tree.DeleteVersion(version - MaxSavedStatesCount)

			if err != nil {
				panic(err)
			}
		}
	}

	s.Clear()

	return hash, version, err
}

func (s *StateDB) Precommit(deleteEmptyObjects bool) {
	// Commit account objects to the trie.
	for _, addr := range getOrderedObjectsKeys(s.stateAccountsDirty) {
		stateObject := s.stateAccounts[addr]
		if deleteEmptyObjects && stateObject.empty() {
			s.deleteStateAccountObject(stateObject)
		} else {
			s.updateStateAccountObject(stateObject)
		}
		delete(s.stateAccountsDirty, addr)
	}

	// Commit identity objects to the trie.
	for _, addr := range getOrderedObjectsKeys(s.stateIdentitiesDirty) {
		stateObject := s.stateIdentities[addr]
		if deleteEmptyObjects && stateObject.empty() {
			s.deleteStateIdentityObject(stateObject)
		} else {
			s.updateStateIdentityObject(stateObject)
		}
		delete(s.stateIdentitiesDirty, addr)
	}

	// if epoch has changed
	if s.stateGlobalDirty {
		currentEpoch := s.Epoch()
		s.updateStateGlobalObject(s.stateGlobal)
		s.stateGlobalDirty = false

		// remove account if epoch is lower
		s.IterateAccounts(func(key []byte, value []byte) bool {
			if key == nil {
				return true
			}
			addr := common.Address{}
			addr.SetBytes(key[1:])

			var data Account
			if err := rlp.DecodeBytes(value, &data); err != nil {
				return false
			}

			if data.Epoch < currentEpoch && data.Balance.Sign() == 0 {
				s.deleteStateAccountObject(newAccountObject(s, addr, data, s.MarkStateAccountObjectDirty))
			}

			return false
		})

	}
}

func (s *StateDB) Reset() {
	s.Clear()
	s.tree.Rollback()
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

func (s *StateDB) IterateIdentities(fn func(key []byte, value []byte) bool) bool {
	start := append(identityPrefix, common.MinAddr...)
	end := append(identityPrefix, common.MaxAddr...)
	return s.tree.GetImmutable().IterateRange(start, end, true, fn)
}

func (s *StateDB) IterateAccounts(fn func(key []byte, value []byte) bool) bool {
	start := append(addressPrefix, common.MinAddr...)
	end := append(addressPrefix, common.MaxAddr...)
	return s.tree.GetImmutable().IterateRange(start, end, true, fn)
}

func (s *StateDB) GetInvites(addr common.Address) uint8 {
	stateObject := s.getStateIdentity(addr)
	if stateObject != nil {
		return stateObject.Invites()
	}
	return 0
}

func (s *StateDB) GetQualifiedFlipsCount(addr common.Address) uint32 {
	stateObject := s.getStateIdentity(addr)
	if stateObject != nil {
		return stateObject.QualifiedFlipsCount()
	}
	return 0
}

func (s *StateDB) GetShortFlipPoints(addr common.Address) float32 {
	stateObject := s.getStateIdentity(addr)
	if stateObject != nil {
		return stateObject.ShortFlipPoints()
	}
	return 0
}

func (s *StateDB) GetIdentityState(addr common.Address) IdentityState {
	stateObject := s.getStateIdentity(addr)
	if stateObject != nil {
		return stateObject.State()
	}
	return Undefined
}

func (s *StateDB) ResetTo(height uint64) error {
	s.Clear()
	_, err := s.tree.LoadVersionForOverwriting(int64(height))
	return err
}

func (s *StateDB) GetIdentity(addr common.Address) Identity {
	stateObject := s.getStateIdentity(addr)
	if stateObject != nil {
		return stateObject.data
	}
	return Identity{}
}

func (s *StateDB) IterateOverIdentities(callback func(addr common.Address, identity Identity)) {
	s.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])

		if obj := s.stateIdentities[addr]; obj != nil {
			callback(addr, obj.data)
			return false
		}
		var data Identity
		if err := rlp.DecodeBytes(value, &data); err != nil {
			return false
		}
		callback(addr, data)
		return false
	})
}
