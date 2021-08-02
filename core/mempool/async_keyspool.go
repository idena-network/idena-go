package mempool

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
)

type AsyncKeysPool struct {
	inner        *KeysPool
	privateQueue chan *types.PrivateFlipKeysPackage
	publicQueue  chan *types.PublicFlipKey
}

func NewAsyncKeysPool(inner *KeysPool) FlipKeysPool {
	pool := &AsyncKeysPool{
		inner:        inner,
		privateQueue: make(chan *types.PrivateFlipKeysPackage, 20000),
		publicQueue:  make(chan *types.PublicFlipKey, 20000),
	}
	go pool.readPrivateQueue()
	go pool.readPublicQueue()
	return pool
}

func (pool *AsyncKeysPool) AddPrivateKeysPackage(keysPackage *types.PrivateFlipKeysPackage, _ bool) error {
	select {
	case pool.privateQueue <- keysPackage:
	default:
		return KeySkipped
	}
	return nil
}

func (pool *AsyncKeysPool) AddPublicFlipKey(key *types.PublicFlipKey, _ bool) error {
	select {
	case pool.publicQueue <- key:
	default:
		return KeySkipped
	}
	return nil
}

func (pool *AsyncKeysPool) GetFlipPackagesHashesForSync(noFilter bool) []common.Hash128 {
	return pool.inner.GetFlipPackagesHashesForSync(noFilter)
}

func (pool *AsyncKeysPool) GetFlipKeysForSync(noFilter bool) []*types.PublicFlipKey {
	return pool.inner.GetFlipKeysForSync(noFilter)
}

func (pool *AsyncKeysPool) readPrivateQueue() {
	for {

		batch := make([]*types.PrivateFlipKeysPackage, 1)
		batch[0] = <-pool.privateQueue

	batchLoop:
		for i := 0; i < batchSize-1; i++ {
			select {
			case tx := <-pool.privateQueue:
				batch = append(batch, tx)
			default:
				break batchLoop
			}
		}
		pool.inner.AddPrivateFlipKeysPackages(batch)
	}
}

func (pool *AsyncKeysPool) readPublicQueue() {
	for {

		batch := make([]*types.PublicFlipKey, 1)
		batch[0] = <-pool.publicQueue

	batchLoop:
		for i := 0; i < batchSize-1; i++ {
			select {
			case tx := <-pool.publicQueue:
				batch = append(batch, tx)
			default:
				break batchLoop
			}
		}
		pool.inner.AddPublicFlipKeys(batch)
	}
}
