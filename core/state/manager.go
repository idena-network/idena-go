package state

import (
	"bufio"
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
	dbm "github.com/tendermint/tendermint/libs/db"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

const (
	SnapshotBlocksRange = 10000
)

type SnapshotManager struct {
	db        dbm.DB
	state     *StateDB
	ipfs      ipfs.Proxy
	lastSaved *snapshot.Manifest
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
		repo:  database.NewRepo(pdb),
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
	m.loadState()
	return m
}

func (m *SnapshotManager) loadState() {
	cid, root, height, _ := m.repo.LastSnapshotManifest()
	m.lastSaved = &snapshot.Manifest{
		Cid:    cid,
		Root:   root,
		Height: height,
	}
}

func createSnapshotFile(datadir string, height uint64) (fileName string, file io.Writer, err error) {
	newpath := filepath.Join(datadir, "snapshot")
	if err := os.MkdirAll(newpath, os.ModePerm); err != nil {
		return "", nil, err
	}

	filePath := filepath.Join(newpath, strconv.FormatUint(height, 10)+".tar")
	f, err := os.Create(filePath)
	if err != nil {
		return "", nil, err
	}
	return filePath, bufio.NewWriter(f), nil
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
		return
	}

	if root, err := m.state.WriteSnapshot(height, file); err != nil {
		m.log.Error("Cannot write snapshot to file", "err", err)
		return
	}
	var f *os.File
	var cid cid.Cid
	if f, err = os.Open(filePath); err != nil {
		m.log.Error("Cannot open snapshot file", "err", err)
		os.Remove(filePath)
		return
	}
	if cid, err := m.ipfs.AddFile(bufio.NewReader(f)); err != nil {
		m.log.Error("Cannot add snapshot file to ipfs", "err", err)
		os.Remove(filePath)
		return
	}
	if prevCid, _, _, prevSnapshotFile := m.repo.LastSnapshotManifest(); prevCid != nil {
		m.ipfs.Unpin(prevCid)
		if prevSnapshotFile != "" {
			os.Remove(prevSnapshotFile)
		}
	}
	m.writeLastManifest(cid.Bytes(), root, height, filePath)
	return root
}

func (m *SnapshotManager) writeLastManifest(snapshotCid []byte, root common.Hash, height uint64, file string) {
	m.repo.WriteLastSnapshotManifest(snapshotCid, root, height, file)
}

func (m *SnapshotManager) DownloadSnapshot(snapshot *snapshot.Manifest) error {

}
