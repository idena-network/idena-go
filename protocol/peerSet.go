package protocol

import (
	"errors"
	peer2 "github.com/libp2p/go-libp2p-core/peer"
	"sync"
)

var (
	errClosed            = errors.New("proto set is closed")
	errAlreadyRegistered = errors.New("proto is already registered")
	errNotRegistered     = errors.New("proto is not registered")
)

type peerSet struct {
	peers  map[peer2.ID]*protoPeer
	lock   sync.RWMutex
	closed bool
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[peer2.ID]*protoPeer),
	}
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known. If a new peer it registered, its broadcast loop is also
// started.
func (ps *peerSet) Register(p *protoPeer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errClosed
	}
	if _, ok := ps.peers[p.id]; ok {
		return errAlreadyRegistered
	}
	ps.peers[p.id] = p

	return nil
}

// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *peerSet) Unregister(id peer2.ID) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	_, ok := ps.peers[id]
	if !ok {
		return errNotRegistered
	}
	delete(ps.peers, id)

	return nil
}

// Peer retrieves the registered peer with the given id.
func (ps *peerSet) Peer(id peer2.ID) *protoPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}

// Len returns if the current number of peers in the set.
func (ps *peerSet) Len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}

func (ps *peerSet) Peers() []*protoPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	list := make([]*protoPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		list = append(list, p)
	}
	return list
}

func (ps *peerSet) SendWithFilter(msgcode uint64, key string, payload interface{}, highPriority bool) {
	peers := ps.Peers()

	for _, p := range peers {
		if _, ok := p.msgCache.Get(key); !ok {
			p.markKey(key)
			p.sendMsg(msgcode, payload, highPriority)
		}
	}
}

func (ps *peerSet) Send(msgcode uint64, payload interface{}) {
	peers := ps.Peers()

	for _, p := range peers {
		p.sendMsg(msgcode, payload, false)
	}
}

func (ps *peerSet) HasPayload(payload []byte) bool {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	key := msgKey(payload)
	for _, p := range ps.peers {
		if _, ok := p.msgCache.Get(key); ok {
			return true
		}
	}
	return false
}
