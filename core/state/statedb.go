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
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/pkg/errors"
	"io"
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
)

var (
	contractStoreMinKey = make([]byte, 0)
	contractStoreMaxKey []byte
)

func init() {
	contractStoreMaxKey = make([]byte, common.MaxContractStoreKeyLength)
	for i := 0; i < len(contractStoreMaxKey); i++ {
		contractStoreMaxKey[i] = 0xFF
	}
}

type StateDB struct {
	original dbm.DB
	db       dbm.DB
	tree     Tree

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateAccounts        map[common.Address]*stateAccount
	stateAccountsDirty   map[common.Address]struct{}
	stateIdentities      map[common.Address]*stateIdentity
	stateIdentitiesDirty map[common.Address]struct{}

	contractStoreCache map[string]*contractStoreValue

	stateGlobal            *stateGlobal
	stateGlobalDirty       bool
	stateStatusSwitch      *stateStatusSwitch
	stateStatusSwitchDirty bool

	stateDelegationSwitch      *stateDelegationSwitch
	stateDelegationSwitchDirty bool

	stateDelayedOfflinePenalties      *stateDelayedOfflinePenalties
	stateDelayedOfflinePenaltiesDirty bool

	log  log.Logger
	lock sync.Mutex
}

func NewLazy(db dbm.DB) (*StateDB, error) {
	prefix, err := StateDbKeys.LoadDbPrefix(db)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load db prefix")
	}
	pdb := dbm.NewPrefixDB(db, prefix)
	tree := NewMutableTree(pdb)
	return &StateDB{
		original:           db,
		db:                 pdb,
		tree:               tree,
		stateAccounts:      make(map[common.Address]*stateAccount),
		stateAccountsDirty: make(map[common.Address]struct{}), stateIdentities: make(map[common.Address]*stateIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		contractStoreCache:   make(map[string]*contractStoreValue),
		log:                  log.New(),
	}, nil
}

func (s *StateDB) ForCheckWithOverwrite(height uint64) (*StateDB, error) {
	db := database.NewBackedMemDb(s.db)
	tree := NewMutableTree(db)
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
		contractStoreCache:   make(map[string]*contractStoreValue),
		log:                  log.New(),
	}, nil
}

func (s *StateDB) ForCheck(height uint64) (*StateDB, error) {
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
		contractStoreCache:   make(map[string]*contractStoreValue),
		log:                  log.New(),
	}, nil
}

func (s *StateDB) Readonly(height int64) (*StateDB, error) {
	tree := NewMutableTree(s.db)
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
		contractStoreCache:   make(map[string]*contractStoreValue),
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
	s.contractStoreCache = make(map[string]*contractStoreValue)
	s.stateGlobal = nil
	s.stateGlobalDirty = false
	s.stateStatusSwitch = nil
	s.stateStatusSwitchDirty = false
	s.stateDelegationSwitch = nil
	s.stateDelegationSwitchDirty = false
	s.stateDelayedOfflinePenalties = nil
	s.stateDelayedOfflinePenaltiesDirty = false
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

func (s *StateDB) PrevEpochBlocks() []uint64 {
	stateObject := s.GetOrNewGlobalObject()
	return stateObject.data.PrevEpochBlocks
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
	return time.Unix(stateObject.data.NextValidationTime, 0)
}

/*
 * SETTERS
 */

func (s *StateDB) ClearAccount(addr common.Address) {
	stateObject := s.GetOrNewAccountObject(addr)
	stateObject.SetNonce(0)
	stateObject.SetBalance(big.NewInt(0))
}

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

func (s *StateDB) AddNewScore(address common.Address, score byte) {
	s.GetOrNewIdentityObject(address).AddNewScore(score)
}

func (s *StateDB) SetInviter(address, inviterAddress common.Address, txHash common.Hash, epochHeight uint32) {
	s.GetOrNewIdentityObject(address).SetInviter(inviterAddress, txHash, epochHeight)
}

func (s *StateDB) GetInviter(address common.Address) *Inviter {
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

func (s *StateDB) AddPrevEpochBlock(height uint64) {
	s.GetOrNewGlobalObject().AddPrevEpochBlock(height)
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

func (s *StateDB) SetFeePerGas(fee *big.Int) {
	s.GetOrNewGlobalObject().SetFeePerGas(fee)
}

func (s *StateDB) FeePerGas() *big.Int {
	return s.GetOrNewGlobalObject().FeePerGas()
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

func (s *StateDB) CanCompleteEpoch() bool {
	return s.GetOrNewGlobalObject().CanCompleteEpoch()
}

func (s *StateDB) BlocksCntWithoutCeremonialTxs() uint32 {
	return s.GetOrNewGlobalObject().BlocksCntWithoutCeremonialTxs()
}

func (s *StateDB) IncBlocksCntWithoutCeremonialTxs() {
	s.GetOrNewGlobalObject().IncBlocksCntWithoutCeremonialTxs()
}

func (s *StateDB) ResetBlocksCntWithoutCeremonialTxs() {
	s.GetOrNewGlobalObject().ResetBlocksCntWithoutCeremonialTxs()
}

func (s *StateDB) AddEmptyBlockByShard(onlineSize int, shardId common.ShardId, proposer common.Address) {
	s.GetOrNewGlobalObject().AddEmptyBlockByShard(onlineSize, shardId, proposer)
}

func (s *StateDB) ResetEmptyBlockByShard(shardId common.ShardId) {
	s.GetOrNewGlobalObject().ResetEmptyBlockByShard(shardId)
}

//
// Setting, updating & deleting state object methods
//

// updateStateAccountObject writes the given object to the trie.
func (s *StateDB) updateStateAccountObject(stateObject *stateAccount) *StateTreeDiff {
	addr := stateObject.Address()
	data, err := stateObject.data.ToBytes()
	if err != nil {
		panic(fmt.Errorf("can't encode account object at %x: %v", addr[:], err))
	}
	key := StateDbKeys.AddressKey(addr)
	s.tree.Set(key, data)
	return &StateTreeDiff{Key: key, Value: data}
}

// updateStateAccountObject writes the given object to the trie.
func (s *StateDB) updateStateIdentityObject(stateObject *stateIdentity) *StateTreeDiff {
	addr := stateObject.Address()
	data, err := stateObject.data.ToBytes()
	if err != nil {
		panic(fmt.Errorf("can't encode identity object at %x: %v", addr[:], err))
	}
	key := StateDbKeys.IdentityKey(addr)
	s.tree.Set(key, data)
	return &StateTreeDiff{Key: key, Value: data}
}

// updateStateAccountObject writes the given object to the trie.
func (s *StateDB) updateStateGlobalObject(stateObject *stateGlobal) *StateTreeDiff {
	data, err := stateObject.data.ToBytes()
	if err != nil {
		panic(fmt.Errorf("can't encode global object, %v", err))
	}
	key := StateDbKeys.GlobalKey()
	s.tree.Set(key, data)
	return &StateTreeDiff{Key: key, Value: data}
}

// updateStateAccountObject writes the given object to the trie.
func (s *StateDB) updateStateStatusSwitchObject(stateObject *stateStatusSwitch) *StateTreeDiff {
	data, err := stateObject.data.ToBytes()
	if err != nil {
		panic(fmt.Errorf("can't encode status switch object, %v", err))
	}
	key := StateDbKeys.StatusSwitchKey()
	s.tree.Set(key, data)
	return &StateTreeDiff{Key: key, Value: data}
}

func (s *StateDB) updateStateDelegationSwitchObject(stateObject *stateDelegationSwitch) *StateTreeDiff {
	data, err := stateObject.data.ToBytes()
	if err != nil {
		panic(fmt.Errorf("can't encode state delegation switch object, %v", err))
	}
	key := StateDbKeys.DelegationSwitchKey()
	s.tree.Set(key, data)
	return &StateTreeDiff{Key: key, Value: data}
}

func (s *StateDB) updateStateDelayedOfflinePenaltyObject(stateObject *stateDelayedOfflinePenalties) *StateTreeDiff {
	data, err := stateObject.data.ToBytes()
	if err != nil {
		panic(fmt.Errorf("can't encode state delayed offline penalty object, %v", err))
	}
	key := StateDbKeys.DelayedOfflinePenaltyKey()
	s.tree.Set(key, data)
	return &StateTreeDiff{Key: key, Value: data}
}

// deleteStateAccountObject removes the given object from the state trie.
func (s *StateDB) deleteStateAccountObject(stateObject *stateAccount) *StateTreeDiff {
	stateObject.deleted = true
	addr := stateObject.Address()
	key := StateDbKeys.AddressKey(addr)
	s.tree.Remove(key)
	return &StateTreeDiff{Key: key, Deleted: true}
}

// deleteStateAccountObject removes the given object from the state trie.
func (s *StateDB) deleteStateIdentityObject(stateObject *stateIdentity) *StateTreeDiff {
	stateObject.deleted = true
	addr := stateObject.Address()
	key := StateDbKeys.IdentityKey(addr)
	s.tree.Remove(key)
	return &StateTreeDiff{Key: key, Deleted: true}
}

func (s *StateDB) deleteStateStatusSwitchObject(stateObject *stateStatusSwitch) *StateTreeDiff {
	stateObject.deleted = true
	key := StateDbKeys.StatusSwitchKey()
	s.tree.Remove(key)
	return &StateTreeDiff{Key: key, Deleted: true}
}

func (s *StateDB) deleteStateDelegationSwitchObject(stateObject *stateDelegationSwitch) *StateTreeDiff {
	stateObject.deleted = true
	key := StateDbKeys.DelegationSwitchKey()
	s.tree.Remove(key)
	return &StateTreeDiff{Key: key, Deleted: true}
}

func (s *StateDB) deleteStateDelayedOfflinePenaltyObject(stateObject *stateDelayedOfflinePenalties) *StateTreeDiff {
	stateObject.deleted = true
	key := StateDbKeys.DelayedOfflinePenaltyKey()
	s.tree.Remove(key)
	return &StateTreeDiff{Key: key, Deleted: true}
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
	_, enc := s.tree.Get(StateDbKeys.AddressKey(addr))
	if len(enc) == 0 {
		return nil
	}
	var data Account
	if err := data.FromBytes(enc); err != nil {
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
	_, enc := s.tree.Get(StateDbKeys.IdentityKey(addr))
	if len(enc) == 0 {
		return nil
	}
	var data Identity
	if err := data.FromBytes(enc); err != nil {
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
	_, enc := s.tree.Get(StateDbKeys.GlobalKey())
	if len(enc) == 0 {
		return nil
	}
	var data Global
	if err := data.FromBytes(enc); err != nil {
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
		if obj.deleted {
			return nil
		}
		return obj
	}

	// Load the object from the database.
	_, enc := s.tree.Get(StateDbKeys.StatusSwitchKey())
	if len(enc) == 0 {
		return nil
	}
	var data IdentityStatusSwitch
	if err := data.FromBytes(enc); err != nil {
		s.log.Error("Failed to decode state status switch object", "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newStatusSwitchObject(data, s.MarkStateStatusSwitchObjectDirty)
	s.setStateStatusSwitchObject(obj)
	return obj
}

func (s *StateDB) getStateDelegationSwitch() (stateObject *stateDelegationSwitch) {
	// Prefer 'live' objects.
	if obj := s.stateDelegationSwitch; obj != nil {
		if obj.deleted {
			return nil
		}
		return obj
	}

	// Load the object from the database.
	_, enc := s.tree.Get(StateDbKeys.DelegationSwitchKey())
	if len(enc) == 0 {
		return nil
	}
	var data DelegationSwitch
	if err := data.FromBytes(enc); err != nil {
		s.log.Error("Failed to decode state delegation switch object", "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newDelegationSwitchObject(data, s.MarkStateDelegationSwitchObjectDirty)
	s.setStateDelegationSwitchObject(obj)
	return obj
}

func (s *StateDB) getStateDelayedOfflinePenalty() (stateObject *stateDelayedOfflinePenalties) {
	// Prefer 'live' objects.
	if obj := s.stateDelayedOfflinePenalties; obj != nil {
		if obj.deleted {
			return nil
		}
		return obj
	}

	// Load the object from the database.
	_, enc := s.tree.Get(StateDbKeys.DelayedOfflinePenaltyKey())
	if len(enc) == 0 {
		return nil
	}
	var data DelayedPenalties
	if err := data.FromBytes(enc); err != nil {
		s.log.Error("Failed to decode state delayed offline penalty object", "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newDelayedOfflinePenaltiesObject(data, s.MarkStateDelayedOfflinePenaltyObjectDirty)
	s.setStateDelayedOfflinePenaltyObject(obj)
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

func (s *StateDB) setStateDelegationSwitchObject(object *stateDelegationSwitch) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateDelegationSwitch = object
}

func (s *StateDB) setStateDelayedOfflinePenaltyObject(object *stateDelayedOfflinePenalties) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateDelayedOfflinePenalties = object
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
	if stateObject == nil || stateObject.deleted {
		stateObject = s.createStatusSwitch()
	}
	return stateObject
}

func (s *StateDB) GetOrNewDelegationSwitchObject() *stateDelegationSwitch {
	stateObject := s.getStateDelegationSwitch()
	if stateObject == nil || stateObject.deleted {
		stateObject = s.createDelegationSwitch()
	}
	return stateObject
}

func (s *StateDB) GetOrNewDelayedOfflinePenaltyObject() *stateDelayedOfflinePenalties {
	stateObject := s.getStateDelayedOfflinePenalty()
	if stateObject == nil || stateObject.deleted {
		stateObject = s.createDelayedOfflinePenalty()
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

func (s *StateDB) MarkStateStatusSwitchObjectDirty() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateStatusSwitchDirty = true
}

func (s *StateDB) MarkStateDelegationSwitchObjectDirty() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateDelegationSwitchDirty = true
}

func (s *StateDB) MarkStateDelayedOfflinePenaltyObjectDirty() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateDelayedOfflinePenaltiesDirty = true
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
	stateObject = newGlobalObject(Global{
		EmptyBlocksByShards: map[common.ShardId][]common.Address{},
		ShardSizes:          map[common.ShardId]uint32{},
	}, s.MarkStateGlobalObjectDirty)
	stateObject.touch()
	s.setStateGlobalObject(stateObject)
	return stateObject
}

func (s *StateDB) createStatusSwitch() *stateStatusSwitch {
	stateObject := newStatusSwitchObject(IdentityStatusSwitch{}, s.MarkStateStatusSwitchObjectDirty)
	stateObject.touch()
	s.setStateStatusSwitchObject(stateObject)
	return stateObject
}

func (s *StateDB) createDelegationSwitch() *stateDelegationSwitch {
	stateObject := newDelegationSwitchObject(DelegationSwitch{}, s.MarkStateDelegationSwitchObjectDirty)
	stateObject.touch()
	s.setStateDelegationSwitchObject(stateObject)
	return stateObject
}

func (s *StateDB) createDelayedOfflinePenalty() *stateDelayedOfflinePenalties {
	stateObject := newDelayedOfflinePenaltiesObject(DelayedPenalties{}, s.MarkStateDelayedOfflinePenaltyObjectDirty)
	stateObject.touch()
	s.setStateDelayedOfflinePenaltyObject(stateObject)
	return stateObject
}

// Commit writes the state to the underlying in-memory trie database.
func (s *StateDB) Commit(deleteEmptyObjects bool) (diff []*StateTreeDiff, root []byte, version int64, err error) {
	diffs := s.Precommit(deleteEmptyObjects)
	root, version, err = s.CommitTree(s.tree.Version() + 1)
	return diffs, root, version, err
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

func (s *StateDB) AddDiff(diffs []*StateTreeDiff) {
	for _, diff := range diffs {
		if diff.Deleted {
			s.tree.Remove(diff.Key)
		} else {
			s.tree.Set(diff.Key, diff.Value)
		}
	}
}

func (s *StateDB) Precommit(deleteEmptyObjects bool) []*StateTreeDiff {
	s.lock.Lock()
	var diffs []*StateTreeDiff
	// Commit account objects to the trie.
	for _, addr := range getOrderedObjectsKeys(s.stateAccountsDirty) {
		stateObject := s.stateAccounts[addr]
		if deleteEmptyObjects && stateObject.empty() {
			diffs = append(diffs, s.deleteStateAccountObject(stateObject))
		} else {
			diffs = append(diffs, s.updateStateAccountObject(stateObject))
		}
		delete(s.stateAccountsDirty, addr)
	}

	// Commit identity objects to the trie.
	for _, addr := range getOrderedObjectsKeys(s.stateIdentitiesDirty) {
		stateObject := s.stateIdentities[addr]
		if deleteEmptyObjects && stateObject.empty() {
			diffs = append(diffs, s.deleteStateIdentityObject(stateObject))
		} else {
			diffs = append(diffs, s.updateStateIdentityObject(stateObject))
		}
		delete(s.stateIdentitiesDirty, addr)
	}

	var keys []string
	for k := range s.contractStoreCache {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return keys[i] > keys[j]
	})
	for _, k := range keys {
		v := s.contractStoreCache[k]
		if v.removed {
			s.tree.Remove([]byte(k))
			diffs = append(diffs, &StateTreeDiff{Key: []byte(k), Deleted: true})
		} else {
			s.tree.Set([]byte(k), v.value)
			diffs = append(diffs, &StateTreeDiff{Key: []byte(k), Value: v.value})
		}
	}
	s.contractStoreCache = make(map[string]*contractStoreValue)

	s.lock.Unlock()

	if s.stateGlobalDirty {
		diffs = append(diffs, s.updateStateGlobalObject(s.stateGlobal))
		s.stateGlobalDirty = false
	}

	if s.stateStatusSwitchDirty {
		if s.stateStatusSwitch.empty() {
			diffs = append(diffs, s.deleteStateStatusSwitchObject(s.stateStatusSwitch))
		} else {
			diffs = append(diffs, s.updateStateStatusSwitchObject(s.stateStatusSwitch))
		}
		s.stateStatusSwitchDirty = false
	}
	if s.stateDelegationSwitchDirty {
		if s.stateDelegationSwitch.empty() {
			diffs = append(diffs, s.deleteStateDelegationSwitchObject(s.stateDelegationSwitch))
		} else {
			diffs = append(diffs, s.updateStateDelegationSwitchObject(s.stateDelegationSwitch))
		}
		s.stateDelegationSwitchDirty = false
	}

	if s.stateDelayedOfflinePenaltiesDirty {
		if s.stateDelayedOfflinePenalties.empty() {
			diffs = append(diffs, s.deleteStateDelayedOfflinePenaltyObject(s.stateDelayedOfflinePenalties))
		} else {
			diffs = append(diffs, s.updateStateDelayedOfflinePenaltyObject(s.stateDelayedOfflinePenalties))
		}
		s.stateDelayedOfflinePenaltiesDirty = false
	}
	return diffs
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
	start := StateDbKeys.IdentityKey(common.MinAddr)
	end := StateDbKeys.IdentityKey(common.MaxAddr)
	return s.tree.GetImmutable().IterateRange(start, end, true, fn)
}

func (s *StateDB) IterateAccounts(fn func(key []byte, value []byte) bool) bool {
	start := StateDbKeys.AddressKey(common.MinAddr)
	end := StateDbKeys.AddressKey(common.MaxAddr)
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

func (s *StateDB) GetScores(addr common.Address) []byte {
	stateObject := s.getStateIdentity(addr)
	if stateObject != nil {
		return stateObject.Scores()
	}
	return []byte{}
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
		if err := data.FromBytes(value); err != nil {
			return false
		}
		callback(addr, data)
		return false
	})
}

func (s *StateDB) IterateOverAccounts(callback func(addr common.Address, account Account)) {
	usedAccounts := make(map[common.Address]Account)
	s.lock.Lock()
	for addr, item := range s.stateAccounts {
		usedAccounts[addr] = item.data
	}
	s.lock.Unlock()

	for addr, item := range usedAccounts {
		callback(addr, item)
	}

	s.IterateAccounts(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])

		if _, ok := usedAccounts[addr]; ok {
			return false
		}

		var data Account
		if err := data.FromBytes(value); err != nil {
			return false
		}
		callback(addr, data)
		return false
	})
}


func (s *StateDB) WriteSnapshot2(height uint64, to io.Writer) (root common.Hash, err error) {
	return WriteTreeTo2(s.db, height, to)
}

func (s *StateDB) RecoverSnapshot2(height uint64, treeRoot common.Hash, from io.Reader) error {
	pdb := dbm.NewPrefixDB(s.original, StateDbKeys.BuildDbPrefix(height))
	common.ClearDb(pdb)
	return ReadTreeFrom2(pdb, height, treeRoot, from)
}


func (s *StateDB) WriteSnapshot(height uint64, to io.Writer) (root common.Hash, err error) {
	return WriteTreeTo(s.db, height, to)
}

func (s *StateDB) RecoverSnapshot(height uint64, treeRoot common.Hash, from io.Reader) error {
	pdb := dbm.NewPrefixDB(s.original, StateDbKeys.BuildDbPrefix(height))
	return ReadTreeFrom(pdb, height, treeRoot, from)
}

func (s *StateDB) CommitSnapshot(height uint64, batch dbm.Batch) (dropDb dbm.DB) {
	pdb := dbm.NewPrefixDB(s.original, StateDbKeys.BuildDbPrefix(height))

	if batch != nil {
		StateDbKeys.SaveDbPrefix(batch, StateDbKeys.BuildDbPrefix(height))
	} else {
		batch := s.original.NewBatch()
		StateDbKeys.SaveDbPrefix(batch, StateDbKeys.BuildDbPrefix(height))
		batch.Write()
	}
	dropDb = s.db

	s.db = pdb
	tree := NewMutableTree(pdb)
	if _, err := tree.LoadVersion(int64(height)); err != nil {
		panic(err)
	}
	s.tree = tree
	s.Clear()
	return dropDb
}

func (s *StateDB) DropSnapshot(manifest *snapshot.Manifest) {
	pdb := dbm.NewPrefixDB(s.original, StateDbKeys.BuildDbPrefix(manifest.Height))
	common.ClearDb(pdb)
}

func (s *StateDB) SetPredefinedGlobal(state *models.ProtoPredefinedState) {
	stateObject := s.GetOrNewGlobalObject()
	stateObject.data.Epoch = uint16(state.Global.Epoch)
	stateObject.data.ValidationPeriod = ValidationPeriod(state.Global.ValidationPeriod)
	stateObject.data.WordsSeed = types.BytesToSeed(state.Global.WordsSeed)
	stateObject.data.GodAddress = common.BytesToAddress(state.Global.GodAddress)
	stateObject.data.LastSnapshot = state.Global.LastSnapshot
	stateObject.data.NextValidationTime = state.Global.NextValidationTime
	stateObject.data.EpochBlock = state.Global.EpochBlock
	stateObject.data.PrevEpochBlocks = state.Global.PrevEpochBlocks
	stateObject.data.FeePerGas = common.BigIntOrNil(state.Global.FeePerGas)
	stateObject.data.VrfProposerThreshold = state.Global.VrfProposerThreshold
	stateObject.data.EmptyBlocksBits = common.BigIntOrNil(state.Global.EmptyBlocksBits)
	stateObject.data.GodAddressInvites = uint16(state.Global.GodAddressInvites)
	stateObject.data.BlocksCntWithoutCeremonialTxs = state.Global.BlocksCntWithoutCeremonialTxs
}

func (s *StateDB) SetPredefinedStatusSwitch(state *models.ProtoPredefinedState) {
	stateObject := s.GetOrNewStatusSwitchObject()
	for _, item := range state.StatusSwitch.Addresses {
		stateObject.data.Addresses = append(stateObject.data.Addresses, common.BytesToAddress(item))
	}
	stateObject.touch()
}

func (s *StateDB) SetPredefinedAccounts(state *models.ProtoPredefinedState) {
	for _, acc := range state.Accounts {
		stateObject := s.GetOrNewAccountObject(common.BytesToAddress(acc.Address))
		stateObject.SetBalance(common.BigIntOrNil(acc.Balance))
		stateObject.SetEpoch(uint16(acc.Epoch))
		stateObject.setNonce(acc.Nonce)
		if acc.ContractData != nil {
			stateObject.data.Contract = &ContractData{}
			stateObject.data.Contract.CodeHash.SetBytes(acc.ContractData.CodeHash)
			stateObject.data.Contract.Stake = big.NewInt(0).SetBytes(acc.ContractData.Stake)
		}
	}
}

func (s *StateDB) SetPredefinedIdentities(state *models.ProtoPredefinedState) {
	for _, identity := range state.Identities {

		var flips []IdentityFlip
		for _, item := range identity.Flips {
			flips = append(flips, IdentityFlip{
				Pair: uint8(item.Pair),
				Cid:  item.Cid,
			})
		}

		stateObject := s.GetOrNewIdentityObject(common.BytesToAddress(identity.Address))
		stateObject.data.Birthday = uint16(identity.Birthday)
		stateObject.data.Generation = identity.Generation
		stateObject.data.Stake = common.BigIntOrNil(identity.Stake)
		stateObject.data.RequiredFlips = uint8(identity.RequiredFlips)
		stateObject.data.PubKey = identity.PubKey
		stateObject.data.Invites = uint8(identity.Invites)
		stateObject.data.State = IdentityState(identity.State)
		stateObject.data.ShortFlipPoints = identity.ShortFlipPoints
		stateObject.data.QualifiedFlips = identity.QualifiedFlips
		stateObject.data.ProfileHash = identity.ProfileHash
		stateObject.data.Code = identity.Code
		stateObject.data.Flips = flips
		stateObject.data.Penalty = common.BigIntOrNil(identity.Penalty)
		stateObject.data.ValidationTxsBits = byte(identity.ValidationBits)
		stateObject.data.LastValidationStatus = ValidationStatusFlag(identity.ValidationStatus)
		stateObject.data.Scores = identity.Scores
		stateObject.data.DelegationEpoch = uint16(identity.DelegationEpoch)
		stateObject.data.DelegationNonce = identity.DelegationNonce

		if identity.Inviter != nil {
			stateObject.data.Inviter = &Inviter{
				TxHash:      common.BytesToHash(identity.Inviter.Hash),
				Address:     common.BytesToAddress(identity.Inviter.Address),
				EpochHeight: identity.Inviter.EpochHeight,
			}
		}
		if identity.Delegatee != nil {
			addr := common.BytesToAddress(identity.Delegatee)
			stateObject.data.Delegatee = &addr
		}
		for _, item := range identity.Invitees {
			stateObject.data.Invitees = append(stateObject.data.Invitees, TxAddr{
				TxHash:  common.BytesToHash(item.Hash),
				Address: common.BytesToAddress(item.Address),
			})
		}

		stateObject.touch()
	}
}

func (s *StateDB) SetPredefinedContractValues(state *models.ProtoPredefinedState) {
	for _, kv := range state.ContractValues {
		s.tree.Set(kv.Key, kv.Value)
	}
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

func (s *StateDB) ToggleDelegationAddress(sender common.Address, delegatee common.Address) {
	delegationSwitch := s.GetOrNewDelegationSwitchObject()
	delegationSwitch.ToggleDelegation(sender, delegatee)
}

func (s *StateDB) DelegationSwitch(sender common.Address) *Delegation {
	delegationSwitch := s.GetOrNewDelegationSwitchObject()
	return delegationSwitch.DelegationSwitch(sender)
}

func (s *StateDB) ClearDelegations() {
	statusSwitch := s.GetOrNewDelegationSwitchObject()
	statusSwitch.Clear()
}

func (s *StateDB) SetContractValue(addr common.Address, key []byte, value []byte) {
	s.contractStoreCache[string(StateDbKeys.ContractStoreKey(addr, key))] = &contractStoreValue{
		value:   value,
		removed: false,
	}
}

func (s *StateDB) GetContractValue(addr common.Address, key []byte) []byte {

	storeKey := StateDbKeys.ContractStoreKey(addr, key)

	if v, ok := s.contractStoreCache[string(storeKey)]; ok {
		if v.removed {
			return nil
		}
		return v.value
	}
	_, value := s.tree.Get(storeKey)
	return value
}

func (s *StateDB) RemoveContractValue(addr common.Address, key []byte) {
	s.contractStoreCache[string(StateDbKeys.ContractStoreKey(addr, key))] = &contractStoreValue{
		value:   nil,
		removed: true,
	}
}

func (s *StateDB) IterateContractStore(addr common.Address, minKey []byte, maxKey []byte, f func(key []byte, value []byte) bool) {

	iteratedKeys := make(map[string]struct{})

	if minKey == nil {
		minKey = contractStoreMinKey
	}
	if maxKey == nil {
		maxKey = contractStoreMaxKey
	}

	for key, value := range s.contractStoreCache {
		keyBytes := []byte(key)
		if (bytes.Compare(keyBytes, StateDbKeys.ContractStoreKey(addr, minKey)) >= 0) && (bytes.Compare(keyBytes, StateDbKeys.ContractStoreKey(addr, maxKey)) <= 0) {
			iteratedKeys[key] = struct{}{}
			if !value.removed && f(keyBytes[common.AddressLength+len(contractStorePrefix):], value.value) {
				return
			}
		}
	}

	s.tree.GetImmutable().IterateRangeInclusive(StateDbKeys.ContractStoreKey(addr, minKey), StateDbKeys.ContractStoreKey(addr, maxKey), true,
		func(key []byte, value []byte, version int64) (stopped bool) {

			if _, ok := iteratedKeys[string(key)]; ok {
				return false
			}

			return f(key[common.AddressLength+len(contractStorePrefix):], value)
		})
}

// Iterate over all stored contract data
func (s *StateDB) IterateContractValues(f func(key []byte, value []byte) bool) {
	s.tree.GetImmutable().IterateRange(StateDbKeys.ContractStoreKey(common.MinAddr, contractStoreMinKey), StateDbKeys.ContractStoreKey(common.MaxAddr, contractStoreMaxKey), true,
		func(key []byte, value []byte) (stopped bool) {
			return f(key, value)
		})
}

func (s *StateDB) DeployContract(addr common.Address, codeHash common.Hash, stake *big.Int) {
	contract := s.GetOrNewAccountObject(addr)
	contract.SetCodeHash(codeHash)
	contract.SetContractStake(stake)
}

func (s *StateDB) GetCodeHash(addr common.Address) *common.Hash {
	stateObject := s.getStateAccount(addr)
	if stateObject != nil && stateObject.data.Contract != nil {
		return &stateObject.data.Contract.CodeHash
	}
	return nil
}

func (s *StateDB) GetContractStake(addr common.Address) *big.Int {
	stateObject := s.getStateAccount(addr)
	if stateObject != nil && stateObject.data.Contract != nil {
		return stateObject.data.Contract.Stake
	}
	return nil
}

func (s *StateDB) DropContract(addr common.Address) {
	stateObject := s.getStateAccount(addr)
	stateObject.data.Contract = nil
	stateObject.touch()
}

func (s *StateDB) SetContractStake(addr common.Address, stake *big.Int) {
	contract := s.GetOrNewAccountObject(addr)
	contract.SetContractStake(stake)
}

func (s *StateDB) Delegations() []*Delegation {
	return s.GetOrNewDelegationSwitchObject().data.Delegations
}

func (s *StateDB) SetDelegatee(addr common.Address, delegatee common.Address) {
	s.GetOrNewIdentityObject(addr).SetDelegatee(delegatee)
}

func (s *StateDB) RemoveDelegatee(addr common.Address) {
	s.GetOrNewIdentityObject(addr).RemoveDelegatee()
}

func (s *StateDB) SetDelegationNonce(addr common.Address, nonce uint32) {
	s.GetOrNewIdentityObject(addr).SetDelegationNonce(nonce)
}

func (s *StateDB) Delegatee(addr common.Address) *common.Address {
	return s.GetOrNewIdentityObject(addr).Delegatee()
}

func (s *StateDB) SetDelegationEpoch(addr common.Address, epoch uint16) {
	s.GetOrNewIdentityObject(addr).SetDelegationEpoch(epoch)
}

func (s *StateDB) DelegationEpoch(addr common.Address) uint16 {
	return s.GetOrNewIdentityObject(addr).DelegationEpoch()
}

func (s *StateDB) CollectKilledDelegators() []common.Address {
	s.lock.Lock()
	defer s.lock.Unlock()
	var result []common.Address
	for _, identity := range s.stateIdentities {
		if identity.Delegatee() != nil && !identity.State().NewbieOrBetter() {
			result = append(result, identity.address)
		}
	}
	return result
}

func (s *StateDB) AddDelayedPenalty(addr common.Address) {
	s.GetOrNewDelayedOfflinePenaltyObject().Add(addr)
}

func (s *StateDB) DelayedOfflinePenalties() []common.Address {
	return s.GetOrNewDelayedOfflinePenaltyObject().data.Identities
}

func (s *StateDB) ClearDelayedOfflinePenalties() {
	s.GetOrNewDelayedOfflinePenaltyObject().Clear()
}

func (s *StateDB) HasDelayedOfflinePenalty(addr common.Address) bool {
	return s.GetOrNewDelayedOfflinePenaltyObject().Has(addr)
}

func (s *StateDB) RemoveDelayedOfflinePenalty(addr common.Address) {
	s.GetOrNewDelayedOfflinePenaltyObject().Remove(addr)
}

func (s *StateDB) ShardId(address common.Address) common.ShardId {
	return s.GetOrNewIdentityObject(address).ShardId()
}
func (s *StateDB) HasVersion(h uint64) bool {
	return s.tree.ExistVersion(int64(h))
}

func (s *StateDB) ShardsNum() uint32 {
	return s.GetOrNewGlobalObject().ShardsNum()
}

func (s *StateDB) SetShardId(addr common.Address, shardId common.ShardId) {
	s.GetOrNewIdentityObject(addr).SetShardId(shardId)
}

func (s *StateDB) SetShardsNum(num uint32) {
	s.GetOrNewGlobalObject().SetShardsNum(num)
}

func (s *StateDB) EmptyBlocksByShard() map[common.ShardId][]common.Address {
	return s.GetOrNewGlobalObject().data.EmptyBlocksByShards
}

func (s *StateDB) ClearEmptyBlocksByShard(){
	s.GetOrNewGlobalObject().ClearEmptyBlocksByShard()
}

func (s *StateDB) ShardSizes() map[common.ShardId]uint32 {
	return s.GetOrNewGlobalObject().data.ShardSizes
}

func (s *StateDB) IncreaseShardSize(shardId common.ShardId) {
	s.GetOrNewGlobalObject().IncreaseShardSize(shardId)
}

func (s *StateDB) DecreaseShardSize(shardId common.ShardId) {
	s.GetOrNewGlobalObject().DecreaseShardSize(shardId)
}

func (s *StateDB) SetShardSize(shardId common.ShardId, size uint32) {
	s.GetOrNewGlobalObject().SetShardSize(shardId, size)
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
