package events

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/libp2p/go-libp2p-core"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"time"
)

const (
	NewTxEventID                  = eventbus.EventID("transaction-new")
	AddBlockEventID               = eventbus.EventID("block-add")
	NewFlipKeyID                  = eventbus.EventID("flip-key-new")
	FastSyncCompleted             = eventbus.EventID("fast-sync-completed")
	NewFlipEventID                = eventbus.EventID("flip-new")
	NewFlipKeysPackageID          = eventbus.EventID("flip-keys-package-new")
	IpfsPortChangedEventId        = eventbus.EventID("ipfs-port-changed")
	DeleteFlipEventID             = eventbus.EventID("flip-delete")
	PeersEventID                  = eventbus.EventID("peers")
	BlockchainResetEventID        = eventbus.EventID("chain-reset")
	IpfsMigrationProgressEventID  = eventbus.EventID("ipfs-migration-progress")
	IpfsMigrationCompletedEventID = eventbus.EventID("ipfs-migration-completed")
	DatabaseInitEventId           = eventbus.EventID("db-init")
	DatabaseInitCompletedEventId  = eventbus.EventID("db-init-completed")
)

type NewTxEvent struct {
	Tx       *types.Transaction
	Own      bool
	ShardId  common.ShardId
	Deferred bool
}

func (e *NewTxEvent) EventID() eventbus.EventID {
	return NewTxEventID
}

type NewBlockEvent struct {
	Block *types.Block
}

func (e *NewBlockEvent) EventID() eventbus.EventID {
	return AddBlockEventID
}

type NewFlipKeyEvent struct {
	Key     *types.PublicFlipKey
	ShardId common.ShardId
	Own     bool
}

func (e *NewFlipKeyEvent) EventID() eventbus.EventID {
	return NewFlipKeyID
}

type FastSyncCompletedEvent struct {
}

func (FastSyncCompletedEvent) EventID() eventbus.EventID {
	return FastSyncCompleted
}

type NewFlipEvent struct {
	Flip *types.Flip
}

func (NewFlipEvent) EventID() eventbus.EventID {
	return NewFlipEventID
}

type NewFlipKeysPackageEvent struct {
	Key     *types.PrivateFlipKeysPackage
	ShardId common.ShardId
	Own     bool
}

func (e *NewFlipKeysPackageEvent) EventID() eventbus.EventID {
	return NewFlipKeysPackageID
}

type IpfsPortChangedEvent struct {
	Host   core.Host
	PubSub *pubsub.PubSub
}

func (i IpfsPortChangedEvent) EventID() eventbus.EventID {
	return IpfsPortChangedEventId
}

type DeleteFlipEvent struct {
	FlipCid []byte
}

func (DeleteFlipEvent) EventID() eventbus.EventID {
	return DeleteFlipEventID
}

type PeersEvent struct {
	PeersData []iface.ConnectionInfo
	Time      time.Time
}

func (e *PeersEvent) EventID() eventbus.EventID {
	return PeersEventID
}

type BlockchainResetEvent struct {
	Header      *types.Header
	RevertedTxs []*types.Transaction
}

func (e *BlockchainResetEvent) EventID() eventbus.EventID {
	return BlockchainResetEventID
}

type IpfsMigrationProgressEvent struct {
	Message string
}

func (e *IpfsMigrationProgressEvent) EventID() eventbus.EventID {
	return IpfsMigrationProgressEventID
}

type IpfsMigrationCompletedEvent struct {
}

func (e *IpfsMigrationCompletedEvent) EventID() eventbus.EventID {
	return IpfsMigrationCompletedEventID
}

type DatabaseInitEvent struct {
	Message string
}

func (e *DatabaseInitEvent) EventID() eventbus.EventID {
	return DatabaseInitEventId
}

type DatabaseInitCompletedEvent struct {
}

func (e *DatabaseInitCompletedEvent) EventID() eventbus.EventID {
	return DatabaseInitCompletedEventId
}
