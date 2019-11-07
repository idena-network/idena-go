package state

import (
	"context"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/log"
	"github.com/ipfs/go-cid"
	dbm "github.com/tendermint/tm-db"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	SnapshotsFolder = "/snapshots"
)

var (
	InvalidManifestPrefix = []byte("im")
	MaxManifestTimeouts   = byte(5)
)

type SnapshotManager struct {
	db        dbm.DB
	state     *StateDB
	ipfs      ipfs.Proxy
	bus       eventbus.Bus
	isSyncing bool
	cfg       *config.Config
	log       log.Logger
	repo      *database.Repo
}

func NewSnapshotManager(db dbm.DB, state *StateDB, bus eventbus.Bus, ipfs ipfs.Proxy, cfg *config.Config) *SnapshotManager {
	pdb := dbm.NewPrefixDB(db, database.SnapshotDbPrefix)
	m := &SnapshotManager{
		db:    pdb,
		state: state,
		repo:  database.NewRepo(db),
		bus:   bus,
		cfg:   cfg,
		log:   log.New(),
		ipfs:  ipfs,
	}
	_ = bus.Subscribe(events.AddBlockEventID,
		func(e eventbus.Event) {
			newBlockEvent := e.(*events.NewBlockEvent)
			m.createSnapshotIfNeeded(newBlockEvent.Block.Header)
		})
	return m
}

func createSnapshotFile(datadir string, height uint64) (fileName string, file *os.File, err error) {
	newpath := filepath.Join(datadir, SnapshotsFolder)
	if err := os.MkdirAll(newpath, os.ModePerm); err != nil {
		return "", nil, err
	}

	filePath := filepath.Join(newpath, strconv.FormatUint(height, 10)+".tar")
	f, err := os.Create(filePath)
	if err != nil {
		return "", nil, err
	}
	return filePath, f, nil
}

func (m *SnapshotManager) createSnapshotIfNeeded(block *types.Header) {
	if m.isSyncing {
		return
	}
	if m.state.LastSnapshot() == block.Height() {
		go m.createSnapshot(block.Height())
	}
}

func (m *SnapshotManager) createSnapshot(height uint64) (root common.Hash) {
	filePath, file, err := createSnapshotFile(m.cfg.DataDir, height)
	if err != nil {
		m.log.Error("Cannot create file for snapshot", "err", err)
		return common.Hash{}
	}

	if root, err = m.state.WriteSnapshot(height, file); err != nil {
		file.Close()
		m.log.Error("Cannot write snapshot to file", "err", err)
		return common.Hash{}
	}
	file.Close()
	var f *os.File
	var cid cid.Cid
	if f, err = os.Open(filePath); err != nil {
		m.log.Error("Cannot open snapshot file", "err", err)
		os.Remove(filePath)
		return
	}
	stat, _ := f.Stat()
	if cid, err = m.ipfs.AddFile(f.Name(), f, stat); err != nil {
		m.log.Error("Cannot add snapshot file to ipfs", "err", err)
		f.Close()
		if err = os.Remove(filePath); err != nil {
			m.log.Error("Cannot remove file", "err", err)
		}
		return
	}
	m.clearFs(filePath)
	m.writeLastManifest(cid.Bytes(), root, height, filePath)
	return root
}

func (m *SnapshotManager) clearFs(excludedFile string) {
	if prevCid, _, _, _ := m.repo.LastSnapshotManifest(); prevCid != nil {
		m.ipfs.Unpin(prevCid)
	}
	m.clearSnapshotFolder(excludedFile)
}

func (m *SnapshotManager) clearSnapshotFolder(excludedFile string) {
	directory := filepath.Join(m.cfg.DataDir, SnapshotsFolder)

	var files []string

	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && path != excludedFile {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		m.log.Error("Cannot walk through snapshot directory", "err", err)
	}
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			if err != nil {
				m.log.Error("Cannot remove file from snapshot directory", "err", err)
			}
		}
	}
}

func (m *SnapshotManager) writeLastManifest(snapshotCid []byte, root common.Hash, height uint64, file string) {
	m.repo.WriteLastSnapshotManifest(snapshotCid, root, height, file)
}

func (m *SnapshotManager) DownloadSnapshot(snapshot *snapshot.Manifest) (filePath string, err error) {
	filePath, file, err := createSnapshotFile(m.cfg.DataDir, snapshot.Height)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())

	lastLoad := time.Now()
	done := false
	onLoading := func(size, read int64) {
		lastLoad = time.Now()
	}
	var loadToErr error

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		loadToErr = m.ipfs.LoadTo(snapshot.Cid, file, ctx, onLoading)
		wg.Done()
		done = true
	}()

	go func() {
		for !done {
			time.Sleep(1 * time.Second)
			if time.Now().Sub(lastLoad) > time.Minute {
				cancel()
			}
		}
	}()

	wg.Wait()

	if loadToErr == nil {
		m.clearFs(filePath)
		m.writeLastManifest(snapshot.Cid, snapshot.Root, snapshot.Height, filePath)
	}

	return filePath, loadToErr
}

func (m *SnapshotManager) StartSync() {
	m.isSyncing = true
}

func (m *SnapshotManager) StopSync() {
	m.isSyncing = false
}

func (m *SnapshotManager) IsInvalidManifest(cid []byte) bool {
	key := append(InvalidManifestPrefix, cid...)
	if !m.db.Has(key) {
		return false
	}
	return m.db.Get(key)[0] >= MaxManifestTimeouts
}

func (m *SnapshotManager) AddInvalidManifest(cid []byte) {
	key := append(InvalidManifestPrefix, cid...)
	m.db.Set(key, []byte{MaxManifestTimeouts})
}
func (m *SnapshotManager) AddTimeoutManifest(cid []byte) {
	key := append(InvalidManifestPrefix, cid...)
	value := []byte{0x1}
	if m.db.Has(key) {
		value = m.db.Get(key)
		value[0]++
	}
	m.db.Set(key, value)
}
