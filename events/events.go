package events

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/libp2p/go-libp2p-core"
)

const (
	NewTxEventID           = eventbus.EventID("transaction-new")
	AddBlockEventID        = eventbus.EventID("block-add")
	NewFlipKeyID           = eventbus.EventID("flip-key-new")
	FastSyncCompleted      = eventbus.EventID("fast-sync-completed")
	NewFlipEventID         = eventbus.EventID("flip-new")
	NewFlipKeysPackageID   = eventbus.EventID("flip-keys-package-new")
	IpfsPortChangedEventId = eventbus.EventID("ipfs-port-changed")
	DeleteFlipEventID      = eventbus.EventID("flip-delete")
)

type NewTxEvent struct {
	Tx  *types.Transaction
	Own bool
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
	Key *types.PublicFlipKey
	Own bool
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
	Key *types.PrivateFlipKeysPackage
	Own bool
}

func (e *NewFlipKeysPackageEvent) EventID() eventbus.EventID {
	return NewFlipKeysPackageID
}

type IpfsPortChangedEvent struct {
	Host core.Host
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
