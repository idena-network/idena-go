package state

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
	"io"
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

func NewLazyIdentityState(db dbm.DB) (*IdentityStateDB, error) {
	prefix, err := IdentityStateDbKeys.LoadDbPrefix(db, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load db prefix")
	}
	pdb := dbm.NewPrefixDB(db, prefix)
	tree := NewMutableTree(pdb)
	return &IdentityStateDB{
		db:                   pdb,
		original:             db,
		tree:                 tree,
		stateIdentities:      make(map[common.Address]*stateApprovedIdentity),
		stateIdentitiesDirty: make(map[common.Address]struct{}),
		log:                  log.New(),
	}, nil
}

func (s *IdentityStateDB) ForCheckWithOverwrite(height uint64) (*IdentityStateDB, error) {
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

func (s *IdentityStateDB) ForCheck(height uint64) (*IdentityStateDB, error) {
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

func (s *IdentityStateDB) Readonly(height uint64) (*IdentityStateDB, error) {
	tree := NewMutableTree(s.db)
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
	prefix, err := IdentityStateDbKeys.LoadDbPrefix(s.original, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load db prefix")
	}
	pdb := dbm.NewPrefixDB(s.original, prefix)
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
	s.GetOrNewIdentityObject(identity).SetOnline(false)
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
	_, enc := s.tree.Get(StateDbKeys.IdentityKey(addr))
	if len(enc) == 0 {
		return nil
	}
	var data ApprovedIdentity
	if err := data.FromBytes(enc); err != nil {
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
	data, err := stateObject.data.ToBytes()
	if err != nil {
		panic(fmt.Errorf("can't encode approved identity object at %x: %v", addr[:], err))
	}

	s.tree.Set(StateDbKeys.IdentityKey(addr), data)
	return data
}

func (s *IdentityStateDB) updateStateIdentityObjectRaw(stateObject *stateApprovedIdentity, value []byte) {
	addr := stateObject.Address()
	s.tree.Set(StateDbKeys.IdentityKey(addr), value)
}

// deleteStateAccountObject removes the given object from the state trie.
func (s *IdentityStateDB) deleteStateIdentityObject(stateObject *stateApprovedIdentity) {
	stateObject.deleted = true
	addr := stateObject.Address()

	s.tree.Remove(StateDbKeys.IdentityKey(addr))
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

func (s *IdentityStateDB) Delegatee(addr common.Address) *common.Address {
	stateObject := s.getStateIdentity(addr)
	if stateObject != nil {
		return stateObject.data.Delegatee
	}
	return nil
}

func (s *IdentityStateDB) SetOnline(addr common.Address, online bool) {
	s.GetOrNewIdentityObject(addr).SetOnline(online)
}

func (s *IdentityStateDB) SetDelegatee(addr common.Address, delegatee common.Address) {
	s.GetOrNewIdentityObject(addr).SetDelegatee(delegatee)
}

func (s *IdentityStateDB) RemoveDelegatee(addr common.Address) {
	s.GetOrNewIdentityObject(addr).RemoveDelegatee()
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

	prefix, err := IdentityStateDbKeys.LoadDbPrefix(s.original, true)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to load db prefix")
	}
	if prefix == nil {
		return nil, nil, errors.New("preliminary prefix is not found")
	}
	pdb := dbm.NewPrefixDB(s.original, prefix)
	tree := NewMutableTree(pdb)
	if _, err := tree.LoadVersion(int64(height)); err != nil {
		return nil, nil, err
	}

	batch = s.original.NewBatch()
	IdentityStateDbKeys.SaveDbPrefix(batch, prefix, false)
	IdentityStateDbKeys.SaveDbPrefix(batch, []byte{}, true)
	dropDb = s.db

	s.db = pdb
	s.tree = tree
	return batch, dropDb, nil
}

func (s *IdentityStateDB) DropPreliminary() {
	if prefix, err := IdentityStateDbKeys.LoadDbPrefix(s.original, true); err != nil {
		s.log.Error("failed to load db prefix", "err", err)
	} else {
		pdb := dbm.NewPrefixDB(s.original, prefix)
		common.ClearDb(pdb)
	}
	b := s.original.NewBatch()
	IdentityStateDbKeys.SaveDbPrefix(b, []byte{}, true)
	b.WriteSync()
}

func (s *IdentityStateDB) CreatePreliminaryCopy(height uint64) (*IdentityStateDB, error) {
	preliminaryPrefix := IdentityStateDbKeys.buildDbPrefix(height + 1)
	pdb := dbm.NewPrefixDB(s.original, preliminaryPrefix)

	if err := common.Copy(s.db, pdb); err != nil {
		return nil, err
	}

	b := s.original.NewBatch()
	IdentityStateDbKeys.SaveDbPrefix(b, preliminaryPrefix, true)
	if err := b.WriteSync(); err != nil {
		return nil, err
	}
	return s.LoadPreliminary(height)
}

func (s *IdentityStateDB) SetPredefinedIdentities(state *models.ProtoPredefinedState) {
	for _, identity := range state.ApprovedIdentities {
		stateObj := s.GetOrNewIdentityObject(common.BytesToAddress(identity.Address))
		stateObj.data.Online = false
		stateObj.data.Approved = identity.Approved
		if identity.Delegatee != nil {
			d := common.BytesToAddress(identity.Delegatee)
			stateObj.data.Delegatee = &d
		}
		stateObj.touch()
	}
}

func (s *IdentityStateDB) RecoverSnapshot(height uint64, treeRoot common.Hash, from io.Reader) error {
	pdb := dbm.NewPrefixDB(s.original, IdentityStateDbKeys.buildDbPrefix(height))
	return ReadTreeFrom(pdb, height, treeRoot, from)
}

func (s *IdentityStateDB) CommitSnapshot(height uint64) (dropDb dbm.DB) {
	pdb := dbm.NewPrefixDB(s.original, IdentityStateDbKeys.buildDbPrefix(height))
	batch := s.original.NewBatch()
	IdentityStateDbKeys.SaveDbPrefix(batch, IdentityStateDbKeys.buildDbPrefix(height), false)
	batch.Write()
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

func (diff *IdentityStateDiff) ToProto() *models.ProtoIdentityStateDiff {
	protoDiff := new(models.ProtoIdentityStateDiff)
	for _, item := range diff.Values {
		protoDiff.Values = append(protoDiff.Values, &models.ProtoIdentityStateDiff_IdentityStateDiffValue{
			Address: item.Address[:],
			Deleted: item.Deleted,
			Value:   item.Value,
		})
	}
	return protoDiff
}

func (diff *IdentityStateDiff) ToBytes() ([]byte, error) {
	return proto.Marshal(diff.ToProto())
}

func (diff *IdentityStateDiff) FromProto(protoDiff *models.ProtoIdentityStateDiff) *IdentityStateDiff {
	for _, item := range protoDiff.Values {
		diff.Values = append(diff.Values, &IdentityStateDiffValue{
			Address: common.BytesToAddress(item.Address),
			Deleted: item.Deleted,
			Value:   item.Value,
		})
	}
	return diff
}

func (diff *IdentityStateDiff) FromBytes(data []byte) error {
	protoDiff := new(models.ProtoIdentityStateDiff)
	if err := proto.Unmarshal(data, protoDiff); err != nil {
		return err
	}
	diff.FromProto(protoDiff)
	return nil
}
