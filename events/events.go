package events

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/eventbus"
)

const (
	NewTxEventID      = eventbus.EventID("transaction-new")
	AddBlockEventID   = eventbus.EventID("block-add")
	NewFlipKeyID      = eventbus.EventID("flip-key-new")
	FastSyncCompleted = eventbus.EventID("fast-sync-completed")
	NewFlipEventID    = eventbus.EventID("flip-new")
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
	FlipCid []byte
}

func (NewFlipEvent) EventID() eventbus.EventID {
	return NewFlipEventID
}
