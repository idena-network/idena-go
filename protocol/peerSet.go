package protocol

import (
	"errors"
	"github.com/idena-network/idena-go/p2p"
	"sync"
)

var (
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

type peerSet struct {
	peers  map[string]*peer
	lock   sync.RWMutex
	closed bool
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[string]*peer),
	}
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known. If a new peer it registered, its broadcast loop is also
// started.
func (ps *peerSet) Register(p *peer) error {
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
func (ps *peerSet) Unregister(id string) error {
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
func (ps *peerSet) Peer(id string) *peer {
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

func (ps *peerSet) Peers() []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		list = append(list, p)
	}
	return list
}

// Close disconnects all peers.
// No new peers can be registered after Close has returned.
func (ps *peerSet) Close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		p.Disconnect(p2p.DiscQuitting)
	}
	ps.closed = true
}

func (ps *peerSet) SendWithFilter(msgcode uint64, payload interface{}) {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	key := msgKey(payload)
	for _, p := range ps.peers {
		if _, ok := p.msgCache.Get(key); !ok {
			p.markKey(key)
			p.sendMsg(msgcode, payload)
		}
	}
}

func (ps *peerSet) Send(msgcode uint64, payload interface{}) {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	for _, p := range ps.peers {
		p.sendMsg(msgcode, payload)
	}
}

func (ps *peerSet) HasPayload(payload interface{}) bool {
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
