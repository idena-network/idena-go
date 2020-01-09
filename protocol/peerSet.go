package protocol

import (
	"bytes"
	"errors"
	peer2 "github.com/libp2p/go-libp2p-core/peer"
	"sort"
	"sync"
)

var (
	errClosed            = errors.New("protoPeer set is closed")
	errAlreadyRegistered = errors.New("protoPeer is already registered")
	errNotRegistered     = errors.New("protoPeer is not registered")
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

func (ps *peerSet) SendWithFilter(msgcode uint64, payload interface{}, highPriority bool) {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	key := msgKey(payload)
	for _, p := range ps.peers {
		if _, ok := p.msgCache.Get(key); !ok {
			p.markKey(key)
			p.sendMsg(msgcode, payload, highPriority)
		}
	}
}

func (ps *peerSet) Send(msgcode uint64, payload interface{}) {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	for _, p := range ps.peers {
		p.sendMsg(msgcode, payload, false)
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

func (ps *peerSet) BetterDistance(distance []byte) bool {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	for _, p := range ps.peers {
		if bytes.Compare(distance, p.distance) < 0 {
			return true
		}
	}
	return false
}

func (ps *peerSet) DisconnectExtraPeers(maxPeers int) {
	peers := ps.Peers()
	if len(peers) <= maxPeers {
		return
	}
	sort.SliceStable(peers, func(i, j int) bool {
		return bytes.Compare(peers[i].distance, peers[j].distance) < 0
	})
	for _, p := range peers[maxPeers:] {
		p.disconnect()
	}
}
