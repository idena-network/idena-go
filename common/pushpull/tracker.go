package pushpull

import (
	"github.com/idena-network/idena-go/common"
	"github.com/libp2p/go-libp2p-core/peer"
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

type DefaultPushTracker struct {
	// After that delay a node will try to pull pushpull if it hasn't been downloaded previously
	pullDelay time.Duration
	ppMutex   sync.Mutex

	//outgoing pulls to another peers
	requests chan PendingPulls

	//the map contains a time of sending the last pull request
	activePulls *sync.Map

	//pending pushes
	pendingPushes *sortedPendingPushes

	holder Holder
}

func NewDefaultPushTracker(pullDelay time.Duration) *DefaultPushTracker {
	if pullDelay == 0 {
		pullDelay = defaultPullDelay
	}
	return &DefaultPushTracker{
		activePulls:   &sync.Map{},
		pendingPushes: newSortedPendingPushes(),
		requests:      make(chan PendingPulls, 1000),
		pullDelay:     pullDelay,
	}
}

func (d *DefaultPushTracker) RemovePull(hash common.Hash128) {
	d.activePulls.Delete(hash)
}

func (d *DefaultPushTracker) SetHolder(holder Holder) {
	d.holder = holder
}

func (d *DefaultPushTracker) RegisterPull(hash common.Hash128) {
	d.activePulls.Store(hash, time.Now())
}

func (d *DefaultPushTracker) AddPendingPush(id peer.ID, hash common.Hash128) {
	if d.holder.Has(hash) || d.pendingPushes.Len() > maxPendingPushes {
		return
	}
	d.ppMutex.Lock()
	defer d.ppMutex.Unlock()
	if timestamp, ok := d.activePulls.Load(hash); ok {
		newReq := pendingRequestTime{PendingPulls{Id: id, Hash: hash}, timestamp.(time.Time)}
		d.pendingPushes.Add(newReq)
	}
}

func (d *DefaultPushTracker) Requests() chan PendingPulls {
	return d.requests
}

func (d *DefaultPushTracker) Run() {
	go d.loop()
	go d.gc()
}

// infinity loop for processing pending pushes
func (d *DefaultPushTracker) loop() {
	for {
		if d.pendingPushes.Len() == 0 {
			time.Sleep(time.Millisecond * 10)
			continue
		}
		obj := d.pendingPushes.Peek(0)
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
		if activePullTime, ok := d.activePulls.Load(obj.req.Hash); !ok {
			d.pendingPushes.Remove(0)
			d.ppMutex.Unlock()
			continue
		} else {
			t := activePullTime.(time.Time)
			if t.After(obj.time) {
				d.pendingPushes.MoveWithNewTime(0, t)
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

func (d *DefaultPushTracker) gc() {
	for {
		time.Sleep(time.Minute)
		d.activePulls.Range(func(key, value interface{}) bool {
			if time.Since(value.(time.Time)) > time.Minute*5 {
				d.activePulls.Delete(key)
			}
			return true
		})
	}
}

type sortedPendingPushes struct {
	list []pendingRequestTime
	lock sync.Mutex
}

func newSortedPendingPushes() *sortedPendingPushes {
	return &sortedPendingPushes{list: make([]pendingRequestTime, 0)}
}

func (s *sortedPendingPushes) Add(req pendingRequestTime) {
	s.lock.Lock()

	i := sort.Search(len(s.list), func(i int) bool {
		return s.list[i].time.After(req.time)
	})
	s.list = append(s.list, pendingRequestTime{})
	copy(s.list[i+1:], s.list[i:])
	s.list[i] = req

	s.lock.Unlock()
}

func (s *sortedPendingPushes) Len() int {
	s.lock.Lock()
	l := len(s.list)
	s.lock.Unlock()
	return l
}

func (s *sortedPendingPushes) Remove(i int) {
	s.lock.Lock()
	copy(s.list[i:], s.list[i+1:])
	s.list[len(s.list)-1] = pendingRequestTime{}
	s.list = s.list[:len(s.list)-1]
	s.lock.Unlock()
}

func (s *sortedPendingPushes) MoveWithNewTime(i int, newTime time.Time) {
	copy := pendingRequestTime{
		req:  s.list[i].req,
		time: newTime,
	}
	s.Remove(i)
	s.Add(copy)
}

func (s *sortedPendingPushes) Peek(i int) pendingRequestTime {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.list[i]
}
