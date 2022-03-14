package state

import (
	"fmt"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/events"
)

type NodeState struct {
	info string
}

func NewNodeState(eventBus eventbus.Bus) *NodeState {
	nodeState := &NodeState{}
	eventBus.Subscribe(events.IpfsMigrationProgressEventID, func(event eventbus.Event) {
		message := event.(*events.IpfsMigrationProgressEvent).Message
		if len(message) > 0 {
			message = fmt.Sprintf("IPFS migration: %s", message)
		}
		nodeState.info = message
	})
	eventBus.Subscribe(events.IpfsMigrationCompletedEventID, func(event eventbus.Event) {
		nodeState.info = ""
	})
	return nodeState
}

func (nodeState *NodeState) Info() string {
	return nodeState.info
}
