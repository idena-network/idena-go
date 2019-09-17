package cache

import (
	"github.com/pkg/errors"
)

type BlockSizesCache struct {
	getBlockSizeFn BlockSizeLoader
	limit          int
	lastHeight     uint64
	sizesPerHeight map[uint64]int
}

type BlockSizeLoader func(height uint64) (size int, ok bool)

func NewBlockSizesCache(limit int) *BlockSizesCache {
	return &BlockSizesCache{
		limit:          limit,
		sizesPerHeight: make(map[uint64]int, limit),
	}
}

func (cache *BlockSizesCache) Initialize(getBlockSizeFn BlockSizeLoader) {
	cache.getBlockSizeFn = getBlockSizeFn
}

func (cache *BlockSizesCache) Get(height uint64) (int, error) {
	if size, ok := cache.sizesPerHeight[height]; ok {
		return size, nil
	}
	size, ok := cache.getBlockSizeFn(height)
	if !ok {
		return 0, errors.Errorf("failed to get block %d", height)
	}
	if err := cache.Add(height, size); err != nil {
		return 0, err
	}
	return size, nil
}

func (cache *BlockSizesCache) Add(height uint64, size int) error {
	if err := cache.check(height); err != nil {
		return err
	}
	cache.prepare(height)
	cache.addBlockSize(height, size)
	return nil
}

func (cache *BlockSizesCache) Reset() {
	cache.sizesPerHeight = make(map[uint64]int, cache.limit)
	cache.lastHeight = 0
}

func (cache *BlockSizesCache) check(height uint64) error {
	if len(cache.sizesPerHeight) == 0 {
		return nil
	}
	if cache.lastHeight+1 != height {
		return errors.Errorf("wrong block height to add to sizes cache, actual: %d, expected: %d",
			height, cache.lastHeight+1)
	}
	return nil
}

func (cache *BlockSizesCache) prepare(height uint64) {
	if len(cache.sizesPerHeight) == cache.limit {
		delete(cache.sizesPerHeight, height-uint64(cache.limit))
	}
}

func (cache *BlockSizesCache) addBlockSize(height uint64, size int) {
	cache.sizesPerHeight[height] = size
	cache.lastHeight = height
}
