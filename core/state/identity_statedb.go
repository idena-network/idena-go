package state

import (
	"fmt"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
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
	tree := NewMutableTreeWithOpts(pdb, dbm.NewMemDB(), DefaultTreeKeepEvery, DefaultTreeKeepRecent)
	return &IdentityStateDB{
		db:                   pdb,
		original:             db,
		tree:                 tree,
		stateIdentities:      make(map[common.Address]*stateApprovedIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}
}

func (s *IdentityStateDB) ForCheckWithOverwrite(height uint64) (*IdentityStateDB, error) {
	db := database.NewBackedMemDb(s.db)
	tree := NewMutableTreeWithOpts(db, database.NewBackedMemDb(s.tree.RecentDb()), s.tree.KeepEvery(), s.tree.KeepRecent())
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

func (s *IdentityStateDB) ForCheck(height uint64) (*IdentityStateDB, error) {
	db := database.NewBackedMemDb(s.db)
	tree := NewMutableTreeWithOpts(db, database.NewBackedMemDb(s.tree.RecentDb()), s.tree.KeepEvery(), s.tree.KeepRecent())
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

func (s *IdentityStateDB) Readonly(height uint64) (*IdentityStateDB, error) {
	tree := NewMutableTreeWithOpts(s.db, s.tree.RecentDb(), s.tree.KeepEvery(), s.tree.KeepRecent())
	if _, err := tree.LazyLoad(int64(height)); err != nil {
		return nil, err
	}

	return &IdentityStateDB{
		db:                   s.db,
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
	version, err := tree.Load()
	if err != nil {
		return nil, err
	}
	if version != int64(height) {
		loaded := false
		versions := tree.AvailableVersions()

		for i := len(versions) - 1; i >= 0; i-- {
			if versions[i] <= int(height) {
				if _, err := tree.LoadVersion(int64(versions[i])); err != nil {
					return nil, err
				}
				loaded = true
				break
			}
		}
		if !loaded {
			return nil, errors.New("tree version is not found")
		}
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
	hash, version, err := s.CommitTree(s.tree.Version() + 1)
	return hash, version, diff, err
}

func (s *IdentityStateDB) CommitTree(newVersion int64) (root []byte, version int64, err error) {
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

func (s *IdentityStateDB) Precommit(deleteEmptyObjects bool) *IdentityStateDiff {
	// Commit identity objects to the trie.
	diff := new(IdentityStateDiff)
	s.lock.Lock()
	defer s.lock.Unlock()
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
	s.lock.Lock()
	defer s.lock.Unlock()
	s.stateIdentities = make(map[common.Address]*stateApprovedIdentity)
	s.stateIdentitiesDirty = make(map[common.Address]struct{})
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
	newobj = newApprovedIdentityObject(addr, ApprovedIdentity{}, s.MarkStateIdentityObjectDirty)
	newobj.touch()
	s.setStateIdentityObject(newobj)
	return newobj, prev
}

// Retrieve a state account given my the address. Returns nil if not found.
func (s *IdentityStateDB) getStateIdentity(addr common.Address) (stateObject *stateApprovedIdentity) {
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
	var data ApprovedIdentity
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		s.log.Error("Failed to decode state identity object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newApprovedIdentityObject(addr, data, s.MarkStateIdentityObjectDirty)
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

func (s *IdentityStateDB) Version() uint64 {
	return uint64(s.tree.Version())
}

func (s *IdentityStateDB) AddDiff(height uint64, diff *IdentityStateDiff) {
	if diff.Empty() {
		return
	}

	s.tree.SetVirtualVersion(int64(height) - 1)

	for _, v := range diff.Values {
		stateObject := s.GetOrNewIdentityObject(v.Address)
		if v.Deleted {
			s.deleteStateIdentityObject(stateObject)
		} else {
			s.updateStateIdentityObjectRaw(stateObject, v.Value)
		}
	}
}

func (s *IdentityStateDB) SaveForcedVersion(height uint64) error {
	if s.tree.Version() == int64(height) {
		return nil
	}
	s.tree.SetVirtualVersion(int64(height) - 1)
	_, _, err := s.CommitTree(int64(height))
	return err
}

func (s *IdentityStateDB) SwitchToPreliminary(height uint64) (batch dbm.Batch, dropDb dbm.DB, err error) {

	prefix := loadIdentityPrefix(s.original, true)
	if prefix == nil {
		return nil, nil, errors.New("preliminary prefix is not found")
	}
	pdb := dbm.NewPrefixDB(s.original, prefix)
	tree := NewMutableTree(pdb)
	if _, err := tree.LoadVersion(int64(height)); err != nil {
		return nil, nil, err
	}

	batch = s.original.NewBatch()
	setIdentityPrefix(batch, prefix, false)
	setIdentityPrefix(batch, nil, true)
	dropDb = s.db

	s.db = pdb
	s.tree = tree
	return batch, dropDb, nil
}

func (s *IdentityStateDB) DropPreliminary() {
	pdb := dbm.NewPrefixDB(s.original, loadIdentityPrefix(s.original, true))
	common.ClearDb(pdb)
	b := s.original.NewBatch()
	setIdentityPrefix(b, nil, true)
	b.WriteSync()
}

func (s *IdentityStateDB) CreatePreliminaryCopy(height uint64) (*IdentityStateDB, error) {
	preliminaryPrefix := identityStatePrefix(height + 1)
	pdb := dbm.NewPrefixDB(s.original, preliminaryPrefix)
	it, err := s.db.Iterator(nil, nil)
	defer it.Close()
	if err != nil {
		return nil, err
	}
	for ; it.Valid(); it.Next() {
		if err := pdb.Set(it.Key(), it.Value()); err != nil {
			return nil, err
		}
	}
	b := s.original.NewBatch()
	setIdentityPrefix(b, preliminaryPrefix, true)
	if err := b.WriteSync(); err != nil {
		return nil, err
	}
	return s.LoadPreliminary(height)
}

func (s *IdentityStateDB) SetPredefinedIdentities(state *PredefinedState) {
	for _, identity := range state.ApprovedIdentities {
		stateObj := s.GetOrNewIdentityObject(identity.Address)
		stateObj.data.Online = false
		stateObj.data.Approved = identity.Approved
		stateObj.touch()
	}
}

func (s *IdentityStateDB) FlushToDisk() error {
	return common.Copy(s.tree.RecentDb(), s.db)
}

func (s *IdentityStateDB) SwitchTree(keepEvery, keepRecent int64) error {
	version := s.tree.Version()
	s.tree = NewMutableTreeWithOpts(s.db, s.tree.RecentDb(), keepEvery, keepRecent)
	if _, err := s.tree.LoadVersion(version); err != nil {
		return err
	}
	s.Clear()
	return nil
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
	p, _ := db.Get(key)
	if p == nil {
		p = identityStatePrefix(0)
		b := db.NewBatch()
		setIdentityPrefix(b, p, preliminary)
		b.WriteSync()
		return p
	}
	return p
}

func setIdentityPrefix(batch dbm.Batch, prefix []byte, preliminary bool) {
	key := currentIdentityStateDbPrefixKey
	if preliminary {
		key = preliminaryIdentityStateDbPrefixKey
	}
	batch.Set(key, prefix)
}
