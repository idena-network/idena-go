package pushpull

import (
	"github.com/idena-network/idena-go/common"
	"github.com/patrickmn/go-cache"
	"time"
)

type Holder interface {
	Add(hash common.Hash128, entry interface{}, shardId common.ShardId, highPriority bool)
	Has(hash common.Hash128) bool
	Get(hash common.Hash128) (entry interface{}, shardId common.ShardId, highPriority bool, present bool)
	MaxParallelPulls() uint32
	SupportPendingRequests() bool
	PushTracker() PendingPushTracker
}

type DefaultHolder struct {
	entryCache  *cache.Cache
	maxPulls    int
	pushTracker PendingPushTracker
}

type entryWrapper struct {
	entry        interface{}
	highPriority bool
	shardId      common.ShardId
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

func (d *DefaultHolder) Add(hash common.Hash128, entry interface{}, shardId common.ShardId, highPriority bool) {
	d.entryCache.SetDefault(hash.String(), &entryWrapper{
		entry:        entry,
		highPriority: highPriority,
		shardId:      shardId,
	})
	if d.pushTracker != nil {
		d.pushTracker.RemovePull(hash)
	}
}

func (d *DefaultHolder) Has(hash common.Hash128) bool {
	_, ok := d.entryCache.Get(hash.String())
	return ok
}

func (d *DefaultHolder) Get(hash common.Hash128) (entry interface{}, id common.ShardId, highPriority bool, present bool) {
	wrapper, ok := d.entryCache.Get(hash.String())
	if !ok {
		return nil, common.MultiShard, false, false
	}
	return wrapper.(*entryWrapper).entry, wrapper.(*entryWrapper).shardId, wrapper.(*entryWrapper).highPriority, true
}

func (d *DefaultHolder) MaxParallelPulls() uint32 {
	return 3
}
