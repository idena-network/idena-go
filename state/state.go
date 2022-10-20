package state

import (
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/events"
)

type NodeState struct {
	info    string
	syncing bool
}

func NewNodeState(eventBus eventbus.Bus) *NodeState {
	nodeState := &NodeState{}
	eventBus.Subscribe(events.DatabaseInitEventId, func(event eventbus.Event) {
		nodeState.info = "Initializing database..."
	})
	eventBus.Subscribe(events.DatabaseInitCompletedEventId, func(event eventbus.Event) {
		nodeState.info = ""
	})
	eventBus.Subscribe(events.IpfsGcEventId, func(event eventbus.Event) {
		e := event.(*events.IpfsGcEvent)
		if e.Completed {
			nodeState.info = ""
			nodeState.syncing = false
		} else {
			nodeState.info = e.Message
			nodeState.syncing = true
		}
	})
	return nodeState
}

func (nodeState *NodeState) Info() string {
	return nodeState.info
}

func (nodeState *NodeState) Syncing() bool {
	return nodeState.syncing
}
