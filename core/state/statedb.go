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
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/mholt/archiver"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"strconv"
	"time"

	"github.com/idena-network/idena-go/common"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"sync"

	"bytes"
	"sort"
)

const (
	MaxSavedStatesCount = 100
	GeneticCodeSize     = 12

	SyncTreeKeepEvery  = int64(200)
	SyncTreeKeepRecent = int64(2)

	DefaultTreeKeepEvery  = int64(1)
	DefaultTreeKeepRecent = int64(0)
)

var (
	addressPrefix                       = []byte("a")
	identityPrefix                      = []byte("i")
	globalKey                           = []byte("global")
	statusSwitchKey                     = []byte("status-switch")
	currentStateDbPrefixKey             = []byte("statedb-prefix")
	currentIdentityStateDbPrefixKey     = []byte("id-statedb-prefix")
	preliminaryIdentityStateDbPrefixKey = []byte("pre-id-statedb-prefix")
)

type StateDB struct {
	original dbm.DB
	db       dbm.DB
	tree     Tree

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateAccounts        map[common.Address]*stateAccount
	stateAccountsDirty   map[common.Address]struct{}
	stateIdentities      map[common.Address]*stateIdentity
	stateIdentitiesDirty map[common.Address]struct{}

	stateGlobal            *stateGlobal
	stateGlobalDirty       bool
	epochDirty             bool
	stateStatusSwitch      *stateStatusSwitch
	stateStatusSwitchDirty bool

	log  log.Logger
	lock sync.Mutex
}

func NewLazy(db dbm.DB) *StateDB {
	pdb := dbm.NewPrefixDB(db, loadPrefix(db))
	tree := NewMutableTreeWithOpts(pdb, dbm.NewMemDB(), DefaultTreeKeepEvery, DefaultTreeKeepRecent)
	return &StateDB{
		original:           db,
		db:                 pdb,
		tree:               tree,
		stateAccounts:      make(map[common.Address]*stateAccount),
		stateAccountsDirty: make(map[common.Address]struct{}), stateIdentities: make(map[common.Address]*stateIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}
}

func (s *StateDB) ForCheckWithOverwrite(height uint64) (*StateDB, error) {
	db := database.NewBackedMemDb(s.db)
	tree := NewMutableTreeWithOpts(db, database.NewBackedMemDb(s.tree.RecentDb()), s.tree.KeepEvery(), s.tree.KeepRecent())
	if _, err := tree.LoadVersionForOverwriting(int64(height)); err != nil {
		return nil, err
	}
	return &StateDB{
		original:             s.original,
		db:                   db,
		tree:                 tree,
		stateAccounts:        make(map[common.Address]*stateAccount),
		stateAccountsDirty:   make(map[common.Address]struct{}),
		stateIdentities:      make(map[common.Address]*stateIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}, nil
}

func (s *StateDB) ForCheck(height uint64) (*StateDB, error) {
	db := database.NewBackedMemDb(s.db)
	tree := NewMutableTreeWithOpts(db, database.NewBackedMemDb(s.tree.RecentDb()), s.tree.KeepEvery(), s.tree.KeepRecent())
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

func (s *StateDB) Readonly(height int64) (*StateDB, error) {
	tree := NewMutableTreeWithOpts(s.db, s.tree.RecentDb(), s.tree.KeepEvery(), s.tree.KeepRecent())
	if _, err := tree.LazyLoad(height); err != nil {
		return nil, err
	}
	return &StateDB{
		db:                   s.db,
		tree:                 tree,
		stateAccounts:        make(map[common.Address]*stateAccount),
		stateAccountsDirty:   make(map[common.Address]struct{}),
		stateIdentities:      make(map[common.Address]*stateIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}, nil
}

func (s *StateDB) Load(height uint64) error {
	_, err := s.tree.LoadVersion(int64(height))
	return err
}

func (s *StateDB) Clear() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.stateAccounts = make(map[common.Address]*stateAccount)
	s.stateAccountsDirty = make(map[common.Address]struct{})
	s.stateIdentities = make(map[common.Address]*stateIdentity)
	s.stateIdentitiesDirty = make(map[common.Address]struct{})
	s.stateGlobal = nil
	s.stateGlobalDirty = false
	s.stateStatusSwitch = nil
	s.stateStatusSwitchDirty = false
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

func (s *StateDB) EpochBlock() uint64 {
	stateObject := s.GetOrNewGlobalObject()
	return stateObject.data.EpochBlock
}

func (s *StateDB) LastSnapshot() uint64 {
	stateObject := s.GetOrNewGlobalObject()
	return stateObject.data.LastSnapshot
}

func (s *StateDB) SetLastSnapshot(height uint64) {
	stateObject := s.GetOrNewGlobalObject()
	stateObject.SetLastSnapshot(height)
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

func (s *StateDB) SubStake(addr common.Address, amount *big.Int) {
	s.GetOrNewIdentityObject(addr).SubStake(amount)
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

func (s *StateDB) AddFlip(addr common.Address, cid []byte, pair uint8) {
	s.GetOrNewIdentityObject(addr).AddFlip(cid, pair)
}

func (s *StateDB) DeleteFlip(addr common.Address, cid []byte) {
	s.GetOrNewIdentityObject(addr).DeleteFlip(cid)
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

func (s *StateDB) SetInviter(address, inviterAddress common.Address, txHash common.Hash) {
	s.GetOrNewIdentityObject(address).SetInviter(inviterAddress, txHash)
}

func (s *StateDB) GetInviter(address common.Address) *TxAddr {
	return s.GetOrNewIdentityObject(address).GetInviter()
}

func (s *StateDB) ResetInviter(address common.Address) {
	s.GetOrNewIdentityObject(address).ResetInviter()
}

func (s *StateDB) AddInvitee(address, inviteeAddress common.Address, txHash common.Hash) {
	s.GetOrNewIdentityObject(address).AddInvitee(inviteeAddress, txHash)
}

func (s *StateDB) GetInvitees(address common.Address) []TxAddr {
	return s.GetOrNewIdentityObject(address).GetInvitees()
}

func (s *StateDB) RemoveInvitee(address, inviteeAddress common.Address) {
	s.GetOrNewIdentityObject(address).RemoveInvitee(inviteeAddress)
}

func (s *StateDB) SetBirthday(address common.Address, birthday uint16) {
	s.GetOrNewIdentityObject(address).SetBirthday(birthday)
}

func (s *StateDB) SetPenalty(address common.Address, penalty *big.Int) {
	s.GetOrNewIdentityObject(address).SetPenalty(penalty)
}

func (s *StateDB) SubPenalty(address common.Address, penalty *big.Int) {
	s.GetOrNewIdentityObject(address).SubPenalty(penalty)
}

func (s *StateDB) ClearPenalty(address common.Address) {
	s.GetOrNewIdentityObject(address).SetPenalty(nil)
}

func (s *StateDB) GetPenalty(address common.Address) *big.Int {
	return s.GetOrNewIdentityObject(address).GetPenalty()
}

func (s *StateDB) SetProfileHash(addr common.Address, hash []byte) {
	s.GetOrNewIdentityObject(addr).SetProfileHash(hash)
}

func (s *StateDB) GetProfileHash(addr common.Address) []byte {
	return s.GetOrNewIdentityObject(addr).GetProfileHash()
}

func (s *StateDB) SetValidationTxBit(addr common.Address, txType types.TxType) {
	s.GetOrNewIdentityObject(addr).SetValidationTxBit(txType)
}

func (s *StateDB) ResetValidationTxBits(addr common.Address) {
	s.GetOrNewIdentityObject(addr).ResetValidationTxBits()
}

func (s *StateDB) HasValidationTx(addr common.Address, txType types.TxType) bool {
	return s.GetOrNewIdentityObject(addr).HasValidationTx(txType)
}

func (s *StateDB) SetValidationStatus(addr common.Address, status ValidationStatusFlag) {
	s.GetOrNewIdentityObject(addr).SetValidationStatus(status)
}

func (s *StateDB) IncEpoch() {
	s.GetOrNewGlobalObject().IncEpoch()
}

func (s *StateDB) SetGlobalEpoch(epoch uint16) {
	s.GetOrNewGlobalObject().SetEpoch(epoch)
}

func (s *StateDB) VrfProposerThreshold() float64 {
	return s.GetOrNewGlobalObject().VrfProposerThreshold()
}

func (s *StateDB) SetVrfProposerThreshold(value float64) {
	s.GetOrNewGlobalObject().SetVrfProposerThreshold(value)
}

func (s *StateDB) AddBlockBit(empty bool) {
	s.GetOrNewGlobalObject().AddBlockBit(empty)
}

func (s *StateDB) EmptyBlocksCount() int {
	return s.GetOrNewGlobalObject().EmptyBlocksCount()
}

func (s *StateDB) SetEpochBlock(height uint64) {
	s.GetOrNewGlobalObject().SetEpochBlock(height)
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

func (s *StateDB) SetFeePerByte(fee *big.Int) {
	s.GetOrNewGlobalObject().SetFeePerByte(fee)
}

func (s *StateDB) FeePerByte() *big.Int {
	return s.GetOrNewGlobalObject().FeePerByte()
}

func (s *StateDB) GodAddressInvites() uint16 {
	return s.GetOrNewGlobalObject().GodAddressInvites()
}

func (s *StateDB) SubGodAddressInvite() {
	s.GetOrNewGlobalObject().SubGodAddressInvite()
}

func (s *StateDB) SetGodAddressInvites(count uint16) {
	s.GetOrNewGlobalObject().SetGodAddressInvites(count)
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

	s.tree.Set(globalKey, data)
}

// updateStateAccountObject writes the given object to the trie.
func (s *StateDB) updateStateStatusSwitchObject(stateObject *stateStatusSwitch) {
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object, %v", err))
	}

	s.tree.Set(statusSwitchKey, data)
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
	s.lock.Lock()
	if obj := s.stateAccounts[addr]; obj != nil {
		s.lock.Unlock()
		if obj.deleted {
			return nil
		}
		return obj
	}
	s.lock.Unlock()
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
	obj := newAccountObject(addr, data, s.MarkStateAccountObjectDirty)
	s.setStateAccountObject(obj)
	return obj
}

// Retrieve a state account given my the address. Returns nil if not found.
func (s *StateDB) getStateIdentity(addr common.Address) (stateObject *stateIdentity) {
	// Prefer 'live' objects.
	s.lock.Lock()
	if obj := s.stateIdentities[addr]; obj != nil {
		s.lock.Unlock()
		if obj.deleted {
			return nil
		}
		return obj
	}
	s.lock.Unlock()

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
	obj := newIdentityObject(addr, data, s.MarkStateIdentityObjectDirty)
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
	_, enc := s.tree.Get(globalKey)
	if len(enc) == 0 {
		return nil
	}
	var data Global
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		s.log.Error("Failed to decode state global object", "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newGlobalObject(data, s.MarkStateGlobalObjectDirty)
	s.setStateGlobalObject(obj)
	return obj
}

func (s *StateDB) getStateStatusSwitch() (stateObject *stateStatusSwitch) {
	// Prefer 'live' objects.
	if obj := s.stateStatusSwitch; obj != nil {
		return obj
	}

	// Load the object from the database.
	_, enc := s.tree.Get(statusSwitchKey)
	if len(enc) == 0 {
		return nil
	}
	var data IdentityStatusSwitch
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		s.log.Error("Failed to decode state status switch object", "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newStatusSwitchObject(data, s.MarkStateStatusSwitchObjectDirty)
	s.setStateStatusSwitchObject(obj)
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

func (s *StateDB) setStateStatusSwitchObject(object *stateStatusSwitch) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateStatusSwitch = object
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

func (s *StateDB) GetOrNewStatusSwitchObject() *stateStatusSwitch {
	stateObject := s.getStateStatusSwitch()
	if stateObject == nil {
		stateObject = s.createStatusSwitch()
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
func (s *StateDB) MarkStateGlobalObjectDirty(withEpoch bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateGlobalDirty = true
	s.epochDirty = s.epochDirty || withEpoch
}

func (s *StateDB) MarkStateStatusSwitchObjectDirty() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateStatusSwitchDirty = true
}

func (s *StateDB) createAccount(addr common.Address) (newobj, prev *stateAccount) {
	prev = s.getStateAccount(addr)
	newobj = newAccountObject(addr, Account{}, s.MarkStateAccountObjectDirty)
	newobj.setNonce(0) // sets the object to dirty
	s.setStateAccountObject(newobj)
	return newobj, prev
}

func (s *StateDB) createIdentity(addr common.Address) (newobj, prev *stateIdentity) {
	prev = s.getStateIdentity(addr)
	newobj = newIdentityObject(addr, Identity{}, s.MarkStateIdentityObjectDirty)
	newobj.touch()
	s.setStateIdentityObject(newobj)
	return newobj, prev
}

func (s *StateDB) createGlobal() (stateObject *stateGlobal) {
	stateObject = newGlobalObject(Global{}, s.MarkStateGlobalObjectDirty)
	stateObject.touch(true)
	s.setStateGlobalObject(stateObject)
	return stateObject
}

func (s *StateDB) createStatusSwitch() *stateStatusSwitch {
	stateObject := newStatusSwitchObject(IdentityStatusSwitch{}, s.MarkStateStatusSwitchObjectDirty)
	stateObject.touch()
	s.setStateStatusSwitchObject(stateObject)
	return stateObject
}

// Commit writes the state to the underlying in-memory trie database.
func (s *StateDB) Commit(deleteEmptyObjects bool) (root []byte, version int64, err error) {
	s.Precommit(deleteEmptyObjects)
	return s.CommitTree(s.tree.Version() + 1)
}

func (s *StateDB) SaveForcedVersion(height uint64) (root []byte, version int64, err error) {
	if s.tree.Version() == int64(height) {
		return
	}
	s.tree.SetVirtualVersion(int64(height) - 1)
	return s.CommitTree(int64(height))
}

func (s *StateDB) CommitTree(newVersion int64) (root []byte, version int64, err error) {
	hash, version, err := s.tree.SaveVersionAt(newVersion)
	if version > MaxSavedStatesCount {

		versions := s.tree.AvailableVersions()

		for i := 0; i < len(versions)-MaxSavedStatesCount; i++ {
			if s.tree.ExistVersion(int64(versions[i])) {
				err = s.tree.DeleteVersion(int64(versions[i]))
				if err != nil {
					panic(err)
				}
			}
		}
	}

	s.Clear()
	return hash, version, err
}

func (s *StateDB) Precommit(deleteEmptyObjects bool) {
	s.lock.Lock()
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
	s.lock.Unlock()

	if s.stateGlobalDirty {
		s.updateStateGlobalObject(s.stateGlobal)
		s.stateGlobalDirty = false
	}

	if s.stateStatusSwitchDirty {
		s.updateStateStatusSwitchObject(s.stateStatusSwitch)
		s.stateStatusSwitchDirty = false
	}

	// if epoch has changed
	if s.epochDirty {
		currentEpoch := s.Epoch()
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
				s.deleteStateAccountObject(newAccountObject(addr, data, s.MarkStateAccountObjectDirty))
			}

			return false
		})
		s.epochDirty = false
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

		s.lock.Lock()
		if obj := s.stateIdentities[addr]; obj != nil {
			s.lock.Unlock()
			callback(addr, obj.data)
			return false
		}
		s.lock.Unlock()
		var data Identity
		if err := rlp.DecodeBytes(value, &data); err != nil {
			return false
		}
		callback(addr, data)
		return false
	})
}

func loadPrefix(db dbm.DB) []byte {
	p, _ := db.Get(currentStateDbPrefixKey)
	if p == nil {
		p = prefix(0)
		b := db.NewBatch()
		setPrefix(b, p)
		if err := b.WriteSync(); err != nil {
			panic(err)
		}
		return p
	}
	return p
}

func setPrefix(b dbm.Batch, prefix []byte) {
	b.Set(currentStateDbPrefixKey, prefix)
}

func prefix(height uint64) []byte {
	return []byte("st-" + strconv.FormatUint(height, 16))
}

func (s *StateDB) WriteSnapshot(height uint64, to io.Writer) (root common.Hash, err error) {
	db := database.NewBackedMemDb(s.db)
	tree := NewMutableTree(db)
	if _, err := tree.LoadVersionForOverwriting(int64(height)); err != nil {
		return common.Hash{}, err
	}

	tar := archiver.Tar{
		MkdirAll:               true,
		OverwriteExisting:      false,
		ImplicitTopLevelFolder: false,
	}

	if err := tar.Create(to); err != nil {
		return common.Hash{}, err
	}

	it, err := db.Iterator(nil, nil)
	if err != nil {
		return common.Hash{}, err
	}
	defer it.Close()

	sb := &snapshot.Block{}

	writeBlock := func(sb *snapshot.Block, name string) error {

		data, _ := rlp.EncodeToBytes(sb)

		return tar.Write(archiver.File{
			FileInfo: archiver.FileInfo{
				CustomName: name,
				FileInfo: &fakeFileInfo{
					size: int64(len(data)),
				},
			},
			ReadCloser: &readCloser{r: bytes.NewReader(data)},
		})
	}

	i := 0
	for ; it.Valid(); it.Next() {
		sb.Add(it.Key(), it.Value())
		if sb.Full() {
			if err := writeBlock(sb, strconv.Itoa(i)); err != nil {
				return common.Hash{}, err
			}
			i++
			sb = &snapshot.Block{}
		}
	}
	if len(sb.Data) > 0 {
		if err := writeBlock(sb, strconv.Itoa(i)); err != nil {
			return common.Hash{}, err
		}
	}
	return tree.WorkingHash(), tar.Close()
}

func (s *StateDB) RecoverSnapshot(manifest *snapshot.Manifest, from io.Reader) error {
	pdb := dbm.NewPrefixDB(s.original, prefix(manifest.Height))

	tar := archiver.Tar{
		MkdirAll:               true,
		OverwriteExisting:      false,
		ImplicitTopLevelFolder: false,
	}

	if err := tar.Open(from, 0); err != nil {
		return err
	}

	for file, err := tar.Read(); err == nil; file, err = tar.Read() {
		if data, err := ioutil.ReadAll(file); err != nil {
			common.ClearDb(pdb)
			return err
		} else {
			sb := &snapshot.Block{}
			if err := rlp.DecodeBytes(data, sb); err != nil {
				common.ClearDb(pdb)
				return err
			}
			for _, pair := range sb.Data {
				pdb.Set(pair.Key, pair.Value)
			}
		}
	}
	tree := NewMutableTree(pdb)
	if _, err := tree.LoadVersion(int64(manifest.Height)); err != nil {
		common.ClearDb(pdb)
		return err
	}

	if tree.WorkingHash() != manifest.Root {
		common.ClearDb(pdb)
		return errors.New("wrong manifest root")
	}
	if !tree.ValidateTree() {
		common.ClearDb(pdb)
		return errors.New("corrupted tree")
	}
	return nil
}

func (s *StateDB) CommitSnapshot(manifest *snapshot.Manifest, batch dbm.Batch) (dropDb dbm.DB) {
	pdb := dbm.NewPrefixDB(s.original, prefix(manifest.Height))

	setPrefix(batch, prefix(manifest.Height))
	dropDb = s.db

	s.db = pdb
	tree := NewMutableTree(pdb)
	if _, err := tree.LoadVersion(int64(manifest.Height)); err != nil {
		panic(err)
	}
	s.tree = tree
	s.Clear()
	return dropDb
}

func (s *StateDB) DropSnapshot(manifest *snapshot.Manifest) {
	pdb := dbm.NewPrefixDB(s.original, prefix(manifest.Height))
	common.ClearDb(pdb)
}

func (s *StateDB) SetPredefinedGlobal(state *PredefinedState) {
	stateObject := s.GetOrNewGlobalObject()
	stateObject.data.Epoch = state.Global.Epoch
	stateObject.data.ValidationPeriod = state.Global.ValidationPeriod
	stateObject.data.WordsSeed = state.Global.WordsSeed
	stateObject.data.GodAddress = state.Global.GodAddress
	stateObject.data.LastSnapshot = state.Global.LastSnapshot
	stateObject.data.NextValidationTime = state.Global.NextValidationTime
	stateObject.data.EpochBlock = state.Global.EpochBlock
	stateObject.data.FeePerByte = state.Global.FeePerByte
	stateObject.data.VrfProposerThreshold = state.Global.VrfProposerThreshold
	stateObject.data.EmptyBlocksBits = state.Global.EmptyBlocksBits
	stateObject.data.GodAddressInvites = state.Global.GodAddressInvites
}

func (s *StateDB) SetPredefinedStatusSwitch(state *PredefinedState) {
	stateObject := s.GetOrNewStatusSwitchObject()
	stateObject.data.Addresses = state.StatusSwitch.Addresses
	stateObject.touch()
}

func (s *StateDB) SetPredefinedAccounts(state *PredefinedState) {
	for _, acc := range state.Accounts {
		stateObject := s.GetOrNewAccountObject(acc.Address)
		stateObject.SetBalance(acc.Balance)
		stateObject.SetEpoch(acc.Epoch)
		stateObject.setNonce(acc.Nonce)
	}
}

func (s *StateDB) SetPredefinedIdentities(state *PredefinedState) {
	for _, identity := range state.Identities {

		var flips []IdentityFlip
		for _, item := range identity.Flips {
			flips = append(flips, IdentityFlip{
				Pair: item.Pair,
				Cid:  item.Cid,
			})
		}

		stateObject := s.GetOrNewIdentityObject(identity.Address)
		stateObject.data.Birthday = identity.Birthday
		stateObject.data.Generation = identity.Generation
		stateObject.data.Stake = identity.Stake
		stateObject.data.RequiredFlips = identity.RequiredFlips
		stateObject.data.PubKey = identity.PubKey
		stateObject.data.Invites = identity.Invites
		stateObject.data.State = identity.State
		stateObject.data.ShortFlipPoints = identity.ShortFlipPoints
		stateObject.data.QualifiedFlips = identity.QualifiedFlips
		stateObject.data.ProfileHash = identity.ProfileHash
		stateObject.data.Code = identity.Code
		stateObject.data.Flips = flips
		stateObject.data.Invitees = identity.Invitees
		stateObject.data.Inviter = identity.Inviter
		stateObject.data.Penalty = identity.Penalty
		stateObject.data.ValidationTxsBits = identity.ValidationTxsBits
		stateObject.data.LastValidationStatus = identity.LastValidationStatus
		stateObject.touch()
	}
}

// flush recent version to disk
func (s *StateDB) FlushToDisk() error {
	return common.Copy(s.tree.RecentDb(), s.db)
}

func (s *StateDB) SwitchTree(keepEvery, keepRecent int64) error {
	version := s.tree.Version()
	s.tree = NewMutableTreeWithOpts(s.db, s.tree.RecentDb(), keepEvery, keepRecent)
	if _, err := s.tree.LoadVersion(version); err != nil {
		return err
	}
	s.Clear()
	return nil
}

func (s *StateDB) HasStatusSwitchAddresses(addr common.Address) bool {
	statusSwitch := s.GetOrNewStatusSwitchObject()
	return statusSwitch.HasAddress(addr)
}

func (s *StateDB) StatusSwitchAddresses() []common.Address {
	statusSwitch := s.GetOrNewStatusSwitchObject()
	return statusSwitch.Addresses()
}

func (s *StateDB) ClearStatusSwitchAddresses() {
	statusSwitch := s.GetOrNewStatusSwitchObject()
	statusSwitch.Clear()
}

func (s *StateDB) ToggleStatusSwitchAddress(sender common.Address) {
	statusSwitch := s.GetOrNewStatusSwitchObject()
	statusSwitch.ToggleAddress(sender)
}

type readCloser struct {
	r io.Reader
}

func (rc *readCloser) Read(p []byte) (n int, err error) {
	return rc.r.Read(p)
}

func (rc *readCloser) Close() error {
	return nil
}
