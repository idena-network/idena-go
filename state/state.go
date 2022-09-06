package state

import (
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/events"
)

type NodeState struct {
	info string
}

func NewNodeState(eventBus eventbus.Bus) *NodeState {
	nodeState := &NodeState{}
	eventBus.Subscribe(events.DatabaseInitEventId, func(event eventbus.Event) {
		nodeState.info = "Initializing database..."
	})
	eventBus.Subscribe(events.DatabaseInitCompletedEventId, func(event eventbus.Event) {
		nodeState.info = ""
	})
	return nodeState
}

func (nodeState *NodeState) Info() string {
	return nodeState.info
}
