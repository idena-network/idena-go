package state

import (
	"fmt"
	dbm "github.com/tendermint/tendermint/libs/db"
	"idena-go/common"
	"idena-go/database"
	"idena-go/log"
	"idena-go/rlp"
	"sync"
)

type IdentityStateDB struct {
	db   dbm.DB
	tree Tree

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateIdentities      map[common.Address]*stateApprovedIdentity
	stateIdentitiesDirty map[common.Address]struct{}

	log  log.Logger
	lock sync.Mutex
}

func NewLazyIdentityState(db dbm.DB) *IdentityStateDB {
	pdb := dbm.NewPrefixDB(db, database.ApprovedIdentityDbPrefix)
	tree := NewMutableTree(pdb)
	return &IdentityStateDB{
		db:                   pdb,
		tree:                 tree,
		stateIdentities:      make(map[common.Address]*stateApprovedIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}
}

func (s *IdentityStateDB) ForCheck(height uint64) (*IdentityStateDB, error) {
	db := database.NewBackedMemDb(s.db)
	tree := NewMutableTree(db)
	if _, err := tree.LoadVersionForOverwriting(int64(height)); err != nil {
		return nil, err
	}

	return &IdentityStateDB{
		db:                   db,
		tree:                 tree,
		stateIdentities:      make(map[common.Address]*stateApprovedIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}, nil
}

func (s *IdentityStateDB) Readonly(height uint64) (*IdentityStateDB, error) {
	db := database.NewBackedMemDb(s.db)
	tree := NewMutableTree(db)
	if _, err := tree.LoadVersion(int64(height)); err != nil {
		return nil, err
	}

	return &IdentityStateDB{
		db:                   db,
		tree:                 tree,
		stateIdentities:      make(map[common.Address]*stateApprovedIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}, nil
}

func (s *IdentityStateDB) Load(height uint64) error {
	_, err := s.tree.LoadVersion(int64(height))
	return err
}

func (s *IdentityStateDB) Add(identity common.Address) {
	s.GetOrNewIdentityObject(identity).SetState(true)
}

func (s *IdentityStateDB) Remove(identity common.Address) {
	s.GetOrNewIdentityObject(identity).SetState(false)
}

// Commit writes the state to the underlying in-memory trie database.
func (s *IdentityStateDB) Commit(deleteEmptyObjects bool) (root []byte, version int64, err error) {

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

func (s *IdentityStateDB) Precommit(deleteEmptyObjects bool) {
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
}

func (s *IdentityStateDB) Reset() {
	s.Clear()
	s.tree.Rollback()
}

func (s *IdentityStateDB) Clear() {
	s.stateIdentities = make(map[common.Address]*stateApprovedIdentity)
	s.stateIdentitiesDirty = make(map[common.Address]struct{})
	s.lock = sync.Mutex{}
}

// Retrieve a state object or create a new state object if nil
func (s *IdentityStateDB) GetOrNewIdentityObject(addr common.Address) *stateApprovedIdentity {
	stateObject := s.getStateIdentity(addr)
	if stateObject == nil || stateObject.deleted {
		stateObject, _ = s.createIdentity(addr)
	}
	return stateObject
}

func (s *IdentityStateDB) createIdentity(addr common.Address) (newobj, prev *stateApprovedIdentity) {
	prev = s.getStateIdentity(addr)
	newobj = newApprovedIdentityObject(s, addr, ApprovedIdentity{}, s.MarkStateIdentityObjectDirty)
	newobj.touch()
	s.setStateIdentityObject(newobj)
	return newobj, prev
}

// Retrieve a state account given my the address. Returns nil if not found.
func (s *IdentityStateDB) getStateIdentity(addr common.Address) (stateObject *stateApprovedIdentity) {
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
	var data ApprovedIdentity
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		s.log.Error("Failed to decode state identity object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newApprovedIdentityObject(s, addr, data, s.MarkStateIdentityObjectDirty)
	s.setStateIdentityObject(obj)
	return obj
}

func (s *IdentityStateDB) setStateIdentityObject(object *stateApprovedIdentity) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateIdentities[object.Address()] = object
}

// MarkStateAccountObjectDirty adds the specified object to the dirty map to avoid costly
// state object cache iteration to find a handful of modified ones.
func (s *IdentityStateDB) MarkStateIdentityObjectDirty(addr common.Address) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stateIdentitiesDirty[addr] = struct{}{}
}

// updateStateAccountObject writes the given object to the trie.
func (s *IdentityStateDB) updateStateIdentityObject(stateObject *stateApprovedIdentity) {
	addr := stateObject.Address()
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}

	s.tree.Set(append(identityPrefix, addr[:]...), data)
}

// deleteStateAccountObject removes the given object from the state trie.
func (s *IdentityStateDB) deleteStateIdentityObject(stateObject *stateApprovedIdentity) {
	stateObject.deleted = true
	addr := stateObject.Address()

	s.tree.Remove(append(identityPrefix, addr[:]...))
}

func (s *IdentityStateDB) Root() common.Hash {
	return s.tree.WorkingHash()
}

func (s *IdentityStateDB) IsApproved(addr common.Address) bool {
	stateObject := s.getStateIdentity(addr)
	if stateObject != nil {
		return stateObject.data.Approved
	}
	return false
}

func (s *IdentityStateDB) IsOnline(addr common.Address) bool {
	stateObject := s.getStateIdentity(addr)
	if stateObject != nil {
		return stateObject.data.Online
	}
	return false
}

func (s *IdentityStateDB) SetOnline(addr common.Address, online bool) {
	s.GetOrNewIdentityObject(addr).SetOnline(online)
}

func (s *IdentityStateDB) ResetTo(height uint64) error {
	s.Clear()
	_, err := s.tree.LoadVersionForOverwriting(int64(height))
	return err
}

func (s *IdentityStateDB) IterateIdentities(fn func(key []byte, value []byte) bool) bool {
	start := append(identityPrefix, common.MinAddr...)
	end := append(identityPrefix, common.MaxAddr...)
	return s.tree.GetImmutable().IterateRange(start, end, true, fn)
}
