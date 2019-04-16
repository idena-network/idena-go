package mempool

import (
	"github.com/asaskevich/EventBus"
	"idena-go/blockchain/types"
	"idena-go/common"
	"sync"
)

type KeysPool struct {
	flipKeys map[common.Address]*types.FlipKey
	mutex    *sync.Mutex
	bus      EventBus.Bus
}

func NewKeysPool(bus EventBus.Bus) *KeysPool {
	return &KeysPool{
		bus:   bus,
		mutex: &sync.Mutex{},
	}
}

func (p *KeysPool) Add(key types.FlipKey) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return nil
}
