package protocol

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/patrickmn/go-cache"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MaxVotePullsForOneHash = 3
	MaxFlipPullsForOneHash = 1
)

type pendingPush struct {
	cnt  uint32
	hash pushPullHash
}

type pullRequest struct {
	peer peer.ID
	hash pushPullHash
}

type PushPullManager struct {
	pendingPushes *cache.Cache
	entryCache    *cache.Cache
	mutex         sync.Mutex
	requests      chan pullRequest
}

func NewPushPullManager() *PushPullManager {
	return &PushPullManager{
		entryCache:    cache.New(time.Minute*3, time.Minute*5),
		pendingPushes: cache.New(time.Minute*3, time.Minute*5),
		requests:      make(chan pullRequest, 2000),
	}
}

func maxPulls(t pushType) uint32 {
	if t == pushFlip {
		return MaxFlipPullsForOneHash
	}
	return MaxVotePullsForOneHash
}

func (m *PushPullManager) existInMemory(hash pushPullHash) bool {
	// TODO : check key packages
	return false
}

func (m *PushPullManager) addPush(id peer.ID, hash pushPullHash) {

	key := hash.String()

	if _, ok := m.entryCache.Get(key); ok {
		return
	}
	_, ok := m.pendingPushes.Get(key)

	if !ok {
		m.mutex.Lock()
		_, ok = m.pendingPushes.Get(key)
		if !ok {
			m.pendingPushes.SetDefault(key, &pendingPush{
				cnt:  1,
				hash: hash,
			})
			m.makeRequest(id, hash)
			m.mutex.Unlock()
			return
		}
		m.mutex.Unlock()
	}
	value, _ := m.pendingPushes.Get(key)

	pendingPush := value.(*pendingPush)
	if pendingPush.cnt >= maxPulls(hash.Type) {
		return
	}
	cnt := atomic.AddUint32(&pendingPush.cnt, 1)
	if cnt >= maxPulls(hash.Type) {
		return
	}
	m.makeRequest(id, hash)
}

func (m *PushPullManager) makeRequest(peer peer.ID, hash pushPullHash) {
	select {
	case m.requests <- pullRequest{peer: peer, hash: hash}:
	default:
	}
}

func (m *PushPullManager) AddEntry(key pushPullHash, entry interface{}) {
	m.entryCache.SetDefault(key.String(), entry)
}

func (m *PushPullManager) GetEntry(hash pushPullHash) (interface{}, bool) {
	return m.entryCache.Get(hash.String())
}

func (m *PushPullManager) Requests() chan pullRequest {
	return m.requests
}
