package protocol

import (
	"errors"
	"github.com/idena-network/idena-go/common"
	peer2 "github.com/libp2p/go-libp2p-core/peer"
	"math/rand"
	"sync"
	"time"
)

var (
	errClosed            = errors.New("proto set is closed")
	errAlreadyRegistered = errors.New("proto is already registered")
	errNotRegistered     = errors.New("proto is not registered")
)

type peerSet struct {
	peers      map[peer2.ID]*protoPeer
	ownShardId common.ShardId
	lock       sync.RWMutex
	closed     bool
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[peer2.ID]*protoPeer),
	}
}

func (ps *peerSet) SetOwnShardId(shardId common.ShardId) {
	ps.ownShardId = shardId
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

func (ps *peerSet) SendWithFilter(msgcode uint64, key string, payload interface{}, shardId common.ShardId, highPriority bool) {
	ps.SendWithFilterAndExpiration(msgcode, key, payload, shardId, highPriority, msgCacheAliveTime)
}

func (ps *peerSet) shouldSendToPeer(p *protoPeer, msgShardId common.ShardId, peersCnt int, highPriority bool) bool {
	if msgShardId == common.MultiShard || p.shardId == msgShardId || p.shardId == common.MultiShard || peersCnt < 4 || highPriority {
		return true
	}
	rnd := rand.Float32()
	return rnd > 1-1.8/float32(peersCnt)
}

func (ps *peerSet) SendWithFilterAndExpiration(msgcode uint64, key string, payload interface{}, msgShardId common.ShardId, highPriority bool, expiration time.Duration) {
	peers := ps.Peers()

	sentToExactShard := 0
	if msgShardId != common.MultiShard && msgShardId != ps.ownShardId {
		for _, p := range peers {
			if p.shardId == msgShardId || p.shardId == common.MultiShard {
				if _, ok := p.msgCache.Get(key); !ok {
					p.markKeyWithExpiration(key, expiration)
					p.sendMsg(msgcode, payload, msgShardId, highPriority)
				}
				if p.shardId != common.MultiShard {
					sentToExactShard++
				}
			}
		}
	}
	const minSendingCntToExactShard = 2
	if sentToExactShard >= minSendingCntToExactShard {
		return
	}

	for _, p := range peers {
		if ps.shouldSendToPeer(p, msgShardId, len(peers), highPriority) {
			if _, ok := p.msgCache.Get(key); !ok {
				p.markKeyWithExpiration(key, expiration)
				p.sendMsg(msgcode, payload, msgShardId, highPriority)
			}
		}
	}
}

func (ps *peerSet) Send(msgcode uint64, payload interface{}, msgShardId common.ShardId) {
	peers := ps.Peers()
	for _, p := range peers {
		if ps.shouldSendToPeer(p, msgShardId, len(peers), false) {
			p.sendMsg(msgcode, payload, msgShardId, false)
		}
	}
}

func (ps *peerSet) hasKey(key string) bool {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	for _, p := range ps.peers {
		if _, ok := p.msgCache.Get(key); ok {
			return true
		}
	}
	return false
}

func (ps *peerSet) FromShard(shardId common.ShardId) int {
	var cnt int
	peers := ps.Peers()
	for _, p := range peers {
		if shardId == common.MultiShard || p.shardId == shardId {
			cnt++
		}
	}
	return cnt
}
