package state

import (
	"github.com/idena-network/idena-go/common"
	"github.com/tendermint/iavl"
	dbm "github.com/tendermint/tm-db"
	"sync"
)

type Tree interface {
	Get(key []byte) (index int64, value []byte)
	Set(key, value []byte) bool
	Remove(key []byte) ([]byte, bool)
	LoadVersion(targetVersion int64) (int64, error)
	Load() (int64, error)
	SaveVersion() ([]byte, int64, error)
	DeleteVersion(version int64) error
	GetImmutable() *ImmutableTree
	Version() int64
	Hash() common.Hash
	WorkingHash() common.Hash
	ExistVersion(version int64) bool
	LoadVersionForOverwriting(targetVersion int64) (int64, error)
	Rollback()
	AvailableVersions() []int
	SaveVersionAt(version int64) ([]byte, int64, error)
	SetVirtualVersion(version int64)
	ValidateTree() bool
	RecentDb() dbm.DB
	KeepRecent() int64
	KeepEvery() int64
}

func NewMutableTree(db dbm.DB) *MutableTree {
	recentDb := dbm.NewMemDB()
	tree, err := iavl.NewMutableTreeWithOpts(db, recentDb, 1024, iavl.PruningOptions(1, 0))
	if err != nil {
		panic(err)
	}
	return &MutableTree{
		tree:       tree,
		recentDb:   recentDb,
		keepRecent: 0,
	}
}

func NewMutableTreeWithOpts(db, recentDb dbm.DB, keepEvery, keepRecent int64) *MutableTree {
	tree, err := iavl.NewMutableTreeWithOpts(db, recentDb, 1024, iavl.PruningOptions(keepEvery, keepRecent))
	if err != nil {
		panic(err)
	}
	return &MutableTree{
		tree:       tree,
		recentDb:   recentDb,
		keepRecent: keepRecent,
		keepEvery:  keepEvery,
	}
}

type MutableTree struct {
	tree       *iavl.MutableTree
	recentDb   dbm.DB
	lock       sync.RWMutex
	keepEvery  int64
	keepRecent int64
}

func (t *MutableTree) KeepEvery() int64 {
	return t.keepEvery
}

func (t *MutableTree) KeepRecent() int64 {
	return t.keepRecent
}

func (t *MutableTree) RecentDb() dbm.DB {
	return t.recentDb
}

func (t *MutableTree) ValidateTree() bool {
	return t.tree.ValidateTree()
}

func (t *MutableTree) SetVirtualVersion(version int64) {
	t.tree.SetVirtualVersion(version)
}

func (t *MutableTree) SaveVersionAt(version int64) ([]byte, int64, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.tree.SaveVersionAt(version)
}

func (t *MutableTree) LoadVersionForOverwriting(targetVersion int64) (int64, error) {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.tree.LoadVersionForOverwriting(targetVersion)
}

func (t *MutableTree) ExistVersion(version int64) bool {
	return t.tree.VersionExists(version)
}

func (t *MutableTree) Hash() common.Hash {
	t.lock.RLock()
	defer t.lock.RUnlock()
	hash := t.tree.Hash()
	var result common.Hash
	copy(result[:], hash)
	return result
}

func (t *MutableTree) WorkingHash() common.Hash {
	t.lock.RLock()
	defer t.lock.RUnlock()
	hash := t.tree.WorkingHash()
	var result common.Hash
	copy(result[:], hash)
	return result
}

func (t *MutableTree) Version() int64 {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.tree.Version()
}

func (t *MutableTree) Load() (int64, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.tree.Load()
}

func (t *MutableTree) GetImmutable() *ImmutableTree {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return &ImmutableTree{
		tree: t.tree.ImmutableTree,
	}
}

func (t *MutableTree) Get(key []byte) (index int64, value []byte) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.tree.Get(key)
}

func (t *MutableTree) Set(key, value []byte) bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.tree.Set(key, value)
}

func (t *MutableTree) Remove(key []byte) ([]byte, bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.tree.Remove(key)
}

func (t *MutableTree) LoadVersion(targetVersion int64) (int64, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.tree.LoadVersion(targetVersion)
}

func (t *MutableTree) SaveVersion() ([]byte, int64, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.tree.SaveVersion()
}

func (t *MutableTree) DeleteVersion(version int64) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.tree.DeleteVersion(version)
}

func (t *MutableTree) Rollback() {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.tree.Rollback()
}

func (t *MutableTree) AvailableVersions() []int {
	return t.tree.AvailableVersions()
}

func (t *MutableTree) LazyLoad(version int64) (int64, error) {
	return t.tree.LazyLoadVersion(version)
}

type ImmutableTree struct {
	tree *iavl.ImmutableTree
}

func (t *ImmutableTree) KeepEvery() int64 {
	panic("implement me")
}

func (t *ImmutableTree) KeepRecent() int64 {
	panic("implement me")
}

func (t *ImmutableTree) RecentDb() dbm.DB {
	panic("Not implemented")
}

func (t *ImmutableTree) ValidateTree() bool {
	return t.tree.ValidateTree()
}

func (t *ImmutableTree) SaveVersionAt(version int64) ([]byte, int64, error) {
	panic("Not implemented")
}

func (t *ImmutableTree) AvailableVersions() []int {
	panic("Not implemented")
}

func (t *ImmutableTree) LoadVersionForOverwriting(targetVersion int64) (int64, error) {
	panic("Not implemented")
}

func (t *ImmutableTree) ExistVersion(version int64) bool {
	panic("Not implemented")
}

func (t *ImmutableTree) Hash() common.Hash {
	hash := t.tree.Hash()
	var result common.Hash
	copy(result[:], hash)
	return result
}

func (t *ImmutableTree) WorkingHash() common.Hash {
	hash := t.tree.Hash()
	var result common.Hash
	copy(result[:], hash)
	return result
}

// Iterate iterates over all keys of the tree, in order.
func (t *ImmutableTree) Iterate(fn func(key []byte, value []byte) bool) (stopped bool) {
	return t.tree.Iterate(fn)
}

// IterateRange makes a callback for all nodes with key between start and end non-inclusive.
// If either are nil, then it is open on that side (nil, nil is the same as Iterate)
func (t *ImmutableTree) IterateRange(start, end []byte, ascending bool, fn func(key []byte, value []byte) bool) (stopped bool) {
	return t.tree.IterateRange(start, end, ascending, fn)
}

// IterateRangeInclusive makes a callback for all nodes with key between start and end inclusive.
// If either are nil, then it is open on that side (nil, nil is the same as Iterate)
func (t *ImmutableTree) IterateRangeInclusive(start, end []byte, ascending bool, fn func(key, value []byte, version int64) bool) (stopped bool) {
	return t.tree.IterateRangeInclusive(start, end, ascending, fn)
}

func (t *ImmutableTree) Version() int64 {
	return t.tree.Version()
}

func (t *ImmutableTree) Load() (int64, error) {
	panic("Not implemented")
}

func (t *ImmutableTree) GetImmutable() *ImmutableTree {
	return t
}

func (t *ImmutableTree) Get(key []byte) (index int64, value []byte) {
	return t.tree.Get(key)
}

func (t *ImmutableTree) Set(key, value []byte) bool {
	panic("Not implemented")
}

func (t *ImmutableTree) Remove(key []byte) ([]byte, bool) {
	panic("Not implemented")
}

func (t *ImmutableTree) LoadVersion(targetVersion int64) (int64, error) {
	panic("Not implemented")
}

func (t *ImmutableTree) SaveVersion() ([]byte, int64, error) {
	panic("Not implemented")
}

func (t *ImmutableTree) DeleteVersion(version int64) error {
	panic("Not implemented")
}

func (t *ImmutableTree) Rollback() {
	panic("Not implemented")
}

func (t *ImmutableTree) SetVirtualVersion(version int64) {
	t.tree.SetVirtualVersion(version)
}
