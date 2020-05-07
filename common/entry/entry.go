package entry

import (
	"github.com/idena-network/idena-go/common"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/patrickmn/go-cache"
	"sort"
	"sync"
	"time"
)

const (
	maxPendingPushes = 20000
	defaultPullDelay = time.Second * 10
)

type PendingPulls struct {
	Id   peer.ID
	Hash common.Hash128
}

type pendingRequestTime struct {
	req  PendingPulls
	time time.Time
}

type PendingPushTracker interface {
	RegisterPull(hash common.Hash128)
	AddPendingPush(id peer.ID, hash common.Hash128)
	Requests() chan PendingPulls
	Run()
	SetHolder(holder Holder)
	RemovePull(hash common.Hash128)
}

type Holder interface {
	Add(hash common.Hash128, entry interface{})
	Has(hash common.Hash128) bool
	Get(hash common.Hash128) (interface{}, bool)
	MaxParallelPulls() uint32
	SupportPendingRequests() bool
	PushTracker() PendingPushTracker
}

type DefaultHolder struct {
	entryCache  *cache.Cache
	maxPulls    int
	pushTracker PendingPushTracker
}

func (d *DefaultHolder) PushTracker() PendingPushTracker {
	return d.pushTracker
}

func (d *DefaultHolder) SupportPendingRequests() bool {
	return d.pushTracker != nil
}

func NewDefaultHolder(maxPulls int, pushTracker PendingPushTracker) Holder {
	holder := &DefaultHolder{
		entryCache:  cache.New(time.Minute*3, time.Minute*5),
		maxPulls:    maxPulls,
		pushTracker: pushTracker,
	}
	if pushTracker != nil {
		pushTracker.SetHolder(holder)
		pushTracker.Run()
	}
	return holder
}

func (d *DefaultHolder) Add(hash common.Hash128, entry interface{}) {
	d.entryCache.SetDefault(hash.String(), entry)
	if d.pushTracker != nil {
		d.pushTracker.RemovePull(hash)
	}
}

func (d *DefaultHolder) Has(hash common.Hash128) bool {
	_, ok := d.entryCache.Get(hash.String())
	return ok
}

func (d *DefaultHolder) Get(hash common.Hash128) (interface{}, bool) {
	return d.entryCache.Get(hash.String())
}

func (d *DefaultHolder) MaxParallelPulls() uint32 {
	return 3
}

type DefaultPushTracker struct {
	// After that delay a node will try to pull entry if it hasn't been downloaded previously
	pullDelay time.Duration
	ppMutex   sync.Mutex

	//outgoing pulls to another peers
	requests chan PendingPulls

	//the map contains a time of sending the last pull request
	activePulls map[common.Hash128]time.Time

	//pending pushes
	pendingPushes *sortedPendingPushes

	holder Holder
}

func NewDefaultPushTracker(pullDelay time.Duration) *DefaultPushTracker {
	if pullDelay == 0 {
		pullDelay = defaultPullDelay
	}
	return &DefaultPushTracker{
		activePulls:   make(map[common.Hash128]time.Time),
		pendingPushes: newSortedPendingPushes(),
		requests:      make(chan PendingPulls, 1000),
		pullDelay:     pullDelay,
	}
}

func (d *DefaultPushTracker) RemovePull(hash common.Hash128) {
	d.ppMutex.Lock()
	delete(d.activePulls, hash)
	d.ppMutex.Unlock()
}

func (d *DefaultPushTracker) SetHolder(holder Holder) {
	d.holder = holder
}

func (d *DefaultPushTracker) RegisterPull(hash common.Hash128) {
	d.ppMutex.Lock()
	defer d.ppMutex.Unlock()
	d.activePulls[hash] = time.Now()
}

func (d *DefaultPushTracker) AddPendingPush(id peer.ID, hash common.Hash128) {
	if d.holder.Has(hash) || len(d.pendingPushes.list) > maxPendingPushes {
		return
	}
	d.ppMutex.Lock()
	defer d.ppMutex.Unlock()
	if timestamp, ok := d.activePulls[hash]; ok {
		newReq := pendingRequestTime{PendingPulls{Id: id, Hash: hash}, timestamp}
		d.pendingPushes.Add(newReq)
	}
}

func (d *DefaultPushTracker) Requests() chan PendingPulls {
	return d.requests
}

func (d *DefaultPushTracker) Run() {
	go d.loop()
}

// infinity loop for processing pending pushes
func (d *DefaultPushTracker) loop() {
	for {
		if len(d.pendingPushes.list) == 0 {
			time.Sleep(time.Millisecond * 100)
			continue
		}
		obj := d.pendingPushes.list[0]
		since := time.Now().Sub(obj.time)
		if since < d.pullDelay {
			time.Sleep(d.pullDelay - since)
		}
		if d.holder.Has(obj.req.Hash) {
			d.ppMutex.Lock()
			d.pendingPushes.Remove(0)
			d.ppMutex.Unlock()
			continue
		}
		d.ppMutex.Lock()
		if activePullTime, ok := d.activePulls[obj.req.Hash]; !ok {
			d.pendingPushes.Remove(0)
			d.ppMutex.Unlock()
			continue
		} else {
			if activePullTime.After(obj.time) {
				d.pendingPushes.MoveWithNewTime(0, activePullTime)
				d.ppMutex.Unlock()
				continue
			}
			d.pendingPushes.Remove(0)
			d.ppMutex.Unlock()
		}

		d.requests <- obj.req
		d.RegisterPull(obj.req.Hash)
	}
}

type sortedPendingPushes struct {
	list []pendingRequestTime
}

func newSortedPendingPushes() *sortedPendingPushes {
	return &sortedPendingPushes{list: make([]pendingRequestTime, 0)}
}

func (s *sortedPendingPushes) Add(req pendingRequestTime) {
	i := sort.Search(len(s.list), func(i int) bool {
		return s.list[i].time.After(req.time)
	})
	s.list = append(s.list, pendingRequestTime{})
	copy(s.list[i+1:], s.list[i:])
	s.list[i] = req
}

func (s *sortedPendingPushes) Remove(i int) {
	copy(s.list[i:], s.list[i+1:])
	s.list[len(s.list)-1] = pendingRequestTime{}
	s.list = s.list[:len(s.list)-1]
}

func (s *sortedPendingPushes) MoveWithNewTime(i int, newTime time.Time) {
	copy := pendingRequestTime{
		req:  s.list[i].req,
		time: newTime,
	}
	s.Remove(i)
	s.Add(copy)
}
