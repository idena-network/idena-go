package state

import (
	"context"
	"fmt"
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

type SnapshotVersion byte

const SnapshotVersionV2 = SnapshotVersion(2)

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

func createSnapshotFile(datadir string, height uint64, version SnapshotVersion) (fileName string, file *os.File, err error) {
	newpath := filepath.Join(datadir, SnapshotsFolder)
	if err := os.MkdirAll(newpath, os.ModePerm); err != nil {
		return "", nil, err
	}

	filePath := filepath.Join(newpath, strconv.FormatUint(height, 10)+"."+strconv.FormatInt(int64(version), 10)+".tar")
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

func (m *SnapshotManager) createShapshotForVersion(height uint64, version SnapshotVersion) (cidBytes []byte, root common.Hash, filePath string) {
	filePath, file, err := createSnapshotFile(m.cfg.DataDir, height, version)
	if err != nil {
		m.log.Error("Cannot create file for snapshot", "err", err)
		return nil, common.Hash{}, ""
	}

	if root, err = m.state.WriteSnapshot2(height, file); err != nil {
		file.Close()
		m.log.Error("Cannot write snapshot to file", "err", err)
		return nil, common.Hash{}, ""
	}

	file.Close()
	var f *os.File
	var cid cid.Cid
	if f, err = os.Open(filePath); err != nil {
		m.log.Error("Cannot open snapshot file", "err", err)
		os.Remove(filePath)
		return nil, common.Hash{}, ""
	}
	stat, _ := f.Stat()
	if cid, err = m.ipfs.AddFile(f.Name(), f, stat); err != nil {
		m.log.Error("Cannot add snapshot file to ipfs", "err", err)
		f.Close()
		if err = os.Remove(filePath); err != nil {
			m.log.Error("Cannot remove file", "err", err)
		}
		return nil, common.Hash{}, ""
	}
	return cid.Bytes(), root, filePath
}

func (m *SnapshotManager) createSnapshot(height uint64) (root common.Hash) {

	cidV2, _, filePath := m.createShapshotForVersion(height, SnapshotVersionV2)
	if cidV2 != nil {
		m.clearFs([]string{filePath})
		m.writeLastManifest(cidV2, root, height, filePath)
	}
	return root
}

func (m *SnapshotManager) clearFs(excludedFiles []string) {
	if prevCidV2, _, _, _ := m.repo.LastSnapshotManifest(); prevCidV2 != nil {
		m.ipfs.Unpin(prevCidV2)
	}
	m.clearSnapshotFolder(excludedFiles)
}

func (m *SnapshotManager) clearSnapshotFolder(excludedFiles []string) {
	directory := filepath.Join(m.cfg.DataDir, SnapshotsFolder)

	var files []string

	var contains = func(arr []string, value string) bool {
		for _, s := range arr {
			if s == value {
				return true
			}
		}
		return false
	}

	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && !contains(excludedFiles, path) {
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

func (m *SnapshotManager) writeLastManifest(snapshotCidV2 []byte, root common.Hash, height uint64, fileV2 string) {
	m.repo.WriteLastSnapshotManifest(snapshotCidV2, root, height, fileV2)
}

func (m *SnapshotManager) DownloadSnapshot(snapshot *snapshot.Manifest) (filePath string, version SnapshotVersion, err error) {
	version = SnapshotVersionV2
	filePath, file, err := createSnapshotFile(m.cfg.DataDir, snapshot.Height, version)
	if err != nil {
		return "", 0, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	lastLoad := time.Now()
	lock := sync.Mutex{}
	logLevels := []float32{0.15, 0.3, 0.5, 0.75}
	onLoading := func(size, read int64) {
		lock.Lock()
		lastLoad = time.Now()
		lock.Unlock()
		if size > 0 && len(logLevels) > 0 && float32(read)/float32(size) >= logLevels[0] {
			m.log.Info("Snapshot loading", "progress", fmt.Sprintf("%v%%", logLevels[0]*100))
			logLevels = logLevels[1:]
		}
	}
	var loadToErr error

	wg := sync.WaitGroup{}
	wg.Add(1)
	//done := false
	done := make(chan struct{})

	go func() {
		loadToErr = m.ipfs.LoadTo(snapshot.CidV2, file, ctx, onLoading)
		wg.Done()
		close(done)
	}()

	go func() {
		for {
			select {
			case <-done:
				return
			default:
				time.Sleep(1 * time.Second)
				lock.Lock()
				idleDuration := time.Now().Sub(lastLoad)
				lock.Unlock()
				if idleDuration > time.Minute {
					cancel()
				}

			}
		}
	}()

	wg.Wait()

	if loadToErr == nil {
		m.clearFs([]string{filePath})
		var filePath2 string

		if version == SnapshotVersionV2 {
			filePath2 = filePath
		}
		m.writeLastManifest(snapshot.CidV2, snapshot.Root, snapshot.Height, filePath2)
	}

	return filePath, version, loadToErr
}

func (m *SnapshotManager) StartSync() {
	m.isSyncing = true
}

func (m *SnapshotManager) StopSync() {
	m.isSyncing = false
}

func (m *SnapshotManager) IsInvalidManifest(cid []byte) bool {
	key := append(InvalidManifestPrefix, cid...)
	if has, err := m.db.Has(key); err != nil || !has {
		return false
	}
	v, err := m.db.Get(key)
	if err != nil {
		return false
	}
	return v[0] >= MaxManifestTimeouts
}

func (m *SnapshotManager) AddInvalidManifest(cid []byte) {
	key := append(InvalidManifestPrefix, cid...)
	m.db.Set(key, []byte{MaxManifestTimeouts})
}
func (m *SnapshotManager) AddTimeoutManifest(cid []byte) {
	key := append(InvalidManifestPrefix, cid...)
	value := []byte{0x1}
	if has, err := m.db.Has(key); err == nil && has {
		if value, err = m.db.Get(key); err == nil {
			value[0]++
		}
	}
	m.db.Set(key, value)
}
