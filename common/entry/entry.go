package entry

import (
	"github.com/idena-network/idena-go/common"
	"github.com/patrickmn/go-cache"
	"time"
)

type Holder interface {
	Add(hash common.Hash128, entry interface{})
	Has(hash common.Hash128) bool
	Get(hash common.Hash128) (interface{}, bool)
	MaxPulls() uint32
}

type DefaultHolder struct {
	entryCache *cache.Cache
	maxPulls   int
}

func NewDefaultHolder(maxPulls int) Holder {
	return &DefaultHolder{
		entryCache: cache.New(time.Minute*3, time.Minute*5),
		maxPulls:   maxPulls,
	}
}

func (d *DefaultHolder) Add(hash common.Hash128, entry interface{}) {
	d.entryCache.SetDefault(hash.String(), entry)
}

func (d *DefaultHolder) Has(hash common.Hash128) bool {
	_, ok := d.entryCache.Get(hash.String())
	return ok
}

func (d *DefaultHolder) Get(hash common.Hash128) (interface{}, bool) {
	return d.entryCache.Get(hash.String())
}

func (d *DefaultHolder) MaxPulls() uint32 {
	return 3
}