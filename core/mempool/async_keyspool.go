package mempool

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
)

type AsyncKeysPool struct {
	inner *KeysPool
	queue chan *types.PrivateFlipKeysPackage
}

func NewAsyncKeysPool(inner *KeysPool) FlipKeysPool {
	pool := &AsyncKeysPool{
		inner: inner,
		queue: make(chan *types.PrivateFlipKeysPackage, 20000),
	}
	go pool.loop()
	return pool
}

func (pool *AsyncKeysPool) AddPrivateKeysPackage(keysPackage *types.PrivateFlipKeysPackage, own bool) error {
	select {
	case pool.queue <- keysPackage:
	default:
	}
	return nil
}

func (pool *AsyncKeysPool) AddPublicFlipKey(key *types.PublicFlipKey, own bool) error {
	return pool.inner.AddPublicFlipKey(key, own)
}

func (pool *AsyncKeysPool) GetFlipPackagesHashes() []common.Hash128 {
	return pool.inner.GetFlipPackagesHashes()
}

func (pool *AsyncKeysPool) GetFlipKeys() []*types.PublicFlipKey {
	return pool.inner.GetFlipKeys()
}

func (pool *AsyncKeysPool) loop() {
	for {
		select {
		case p := <-pool.queue:
			pool.inner.AddPrivateKeysPackage(p, false)
		}
	}
}
