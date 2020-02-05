package protocol

import (
	"github.com/idena-network/idena-go/common/entry"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/patrickmn/go-cache"
	"sync"
	"sync/atomic"
	"time"
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
	mutex         sync.Mutex
	requests      chan pullRequest
	entryHolders  map[pushType]entry.Holder
}

func NewPushPullManager() *PushPullManager {
	return &PushPullManager{
		pendingPushes: cache.New(time.Minute*3, time.Minute*5),
		requests:      make(chan pullRequest, 2000),
		entryHolders:  make(map[pushType]entry.Holder),
	}
}

func (m *PushPullManager) AddEntryHolder(pushId pushType, holder entry.Holder) {
	m.entryHolders[pushId] = holder
}

func (m *PushPullManager) addPush(id peer.ID, hash pushPullHash) {

	key := hash.String()

	holder := m.entryHolders[hash.Type]
	if holder == nil {
		panic("entry holder is not found")
	}
	if holder.Has(hash.Hash) {
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
	if pendingPush.cnt >= holder.MaxPulls() {
		return
	}
	cnt := atomic.AddUint32(&pendingPush.cnt, 1)
	if cnt >= holder.MaxPulls() {
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
	m.entryHolders[key.Type].Add(key.Hash, entry)
}

func (m *PushPullManager) GetEntry(hash pushPullHash) (interface{}, bool) {
	return m.entryHolders[hash.Type].Get(hash.Hash)
}

func (m *PushPullManager) Requests() chan pullRequest {
	return m.requests
}
