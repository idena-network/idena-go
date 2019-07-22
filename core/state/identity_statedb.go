package state

import (
	"fmt"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tendermint/libs/db"
	"strconv"
	"sync"
)

type IdentityStateDB struct {
	db       dbm.DB
	original dbm.DB
	tree     Tree

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateIdentities      map[common.Address]*stateApprovedIdentity
	stateIdentitiesDirty map[common.Address]struct{}

	log  log.Logger
	lock sync.Mutex
}

func NewLazyIdentityState(db dbm.DB) *IdentityStateDB {
	pdb := dbm.NewPrefixDB(db, loadIdentityPrefix(db, false))
	tree := NewMutableTree(pdb)
	return &IdentityStateDB{
		db:                   pdb,
		original:             db,
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
		original:             s.original,
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
		original:             s.original,
		tree:                 tree,
		stateIdentities:      make(map[common.Address]*stateApprovedIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}, nil
}

func (s *IdentityStateDB) LoadPreliminary(height uint64) (*IdentityStateDB, error) {
	pdb := dbm.NewPrefixDB(s.original, loadIdentityPrefix(s.original, true))
	tree := NewMutableTree(pdb)
	if _, err := tree.LoadVersion(int64(height)); err != nil {
		return nil, err
	}

	return &IdentityStateDB{
		db:                   pdb,
		original:             s.original,
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
func (s *IdentityStateDB) Commit(deleteEmptyObjects bool) (root []byte, version int64, diff *IdentityStateDiff, err error) {
	diff = s.Precommit(deleteEmptyObjects)
	hash, version := s.CommitTree()
	return hash, version, diff, err
}

func (s *IdentityStateDB) CommitTree() (root []byte, version int64) {
	hash, version, err := s.tree.SaveVersion()
	if version > MaxSavedStatesCount {
		if s.tree.ExistVersion(version - MaxSavedStatesCount) {
			err = s.tree.DeleteVersion(version - MaxSavedStatesCount)

			if err != nil {
				panic(err)
			}
		}
	}

	s.Clear()
	return hash, version
}

func (s *IdentityStateDB) Precommit(deleteEmptyObjects bool) *IdentityStateDiff {
	// Commit identity objects to the trie.
	diff := new(IdentityStateDiff)
	for _, addr := range getOrderedObjectsKeys(s.stateIdentitiesDirty) {
		stateObject := s.stateIdentities[addr]
		if deleteEmptyObjects && stateObject.empty() {
			s.deleteStateIdentityObject(stateObject)
			diff.Values = append(diff.Values, &IdentityStateDiffValue{
				Address: addr,
				Deleted: true,
			})
		} else {
			encoded := s.updateStateIdentityObject(stateObject)
			diff.Values = append(diff.Values, &IdentityStateDiffValue{
				Address: addr,
				Deleted: false,
				Value:   encoded,
			})
		}
		delete(s.stateIdentitiesDirty, addr)
	}
	return diff
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
func (s *IdentityStateDB) updateStateIdentityObject(stateObject *stateApprovedIdentity) (encoded []byte) {
	addr := stateObject.Address()
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}

	s.tree.Set(append(identityPrefix, addr[:]...), data)
	return data
}

func (s *IdentityStateDB) updateStateIdentityObjectRaw(stateObject *stateApprovedIdentity, value []byte) {
	addr := stateObject.Address()
	s.tree.Set(append(identityPrefix, addr[:]...), value)
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

func (s *IdentityStateDB) HasVersion(height uint64) bool {
	return s.tree.ExistVersion(int64(height))
}
func (s *IdentityStateDB) IterateIdentities(fn func(key []byte, value []byte) bool) bool {
	return s.tree.GetImmutable().IterateRange(nil, nil, true, fn)
}

func (s *IdentityStateDB) AddDiff(diff *IdentityStateDiff) {
	if diff.Empty() {
		return
	}
	for _, v := range diff.Values {
		stateObject := s.GetOrNewIdentityObject(v.Address)
		if v.Deleted {
			s.deleteStateIdentityObject(stateObject)
		} else {
			s.updateStateIdentityObjectRaw(stateObject, v.Value)
		}
	}
}

func (s *IdentityStateDB) SwitchToPreliminary(heigth uint64) error {
	prefix := loadIdentityPrefix(s.original, true)
	if prefix == nil {
		return errors.New("preliminary head is not found")
	}
	pdb := dbm.NewPrefixDB(s.original, prefix)
	tree := NewMutableTree(pdb)
	if _, err := tree.LoadVersion(int64(heigth)); err != nil {
		clearDb(pdb)
		return err
	}
	setIdentityPrefix(s.original, prefix, false)
	setIdentityPrefix(s.original, nil, true)
	clearDb(s.db)

	s.db = pdb
	s.tree = tree
	return nil
}

func (s *IdentityStateDB) DropPreliminary() {
	clearDb(s.db)
	setIdentityPrefix(s.original, nil, true)
}

func (s *IdentityStateDB) CreatePreliminaryCopy(height uint64) (*IdentityStateDB, error) {
	preliminaryPrefix := identityStatePrefix(height + 1)
	pdb := dbm.NewPrefixDB(s.original, preliminaryPrefix)
	it := s.db.Iterator(nil, nil)
	for ; it.Valid(); it.Next() {
		pdb.Set(it.Key(), it.Value())
	}
	setIdentityPrefix(s.original, preliminaryPrefix, true)
	return s.LoadPreliminary(height)
}

type IdentityStateDiffValue struct {
	Address common.Address
	Deleted bool
	Value   []byte
}

type IdentityStateDiff struct {
	Values []*IdentityStateDiffValue
}

func (diff *IdentityStateDiff) Empty() bool {
	return diff == nil || len(diff.Values) == 0
}

func (diff IdentityStateDiff) Bytes() []byte {
	enc, _ := rlp.EncodeToBytes(diff)
	return enc
}

func identityStatePrefix(height uint64) []byte {
	return []byte("aid-" + strconv.FormatUint(height, 16))
}

func loadIdentityPrefix(db dbm.DB, preliminary bool) []byte {
	key := currentIdentityStateDbPrefixKey
	if preliminary {
		key = preliminaryIdentityStateDbPrefixKey
	}
	p := db.Get(key)
	if p == nil {
		p = identityStatePrefix(0)
		setIdentityPrefix(db, p, preliminary)
		return p
	}
	return p
}

func setIdentityPrefix(db dbm.DB, prefix []byte, preliminary bool) {
	key := currentIdentityStateDbPrefixKey
	if preliminary {
		key = preliminaryIdentityStateDbPrefixKey
	}
	db.Set(key, prefix)
}
