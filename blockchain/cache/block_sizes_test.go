package cache

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_Add(t *testing.T) {
	cache := NewBlockSizesCache(4)

	require.Nil(t, cache.Add(3, 33))
	require.Nil(t, cache.Add(4, 44))
	require.Nil(t, cache.Add(5, 55))
	require.Equal(t, uint64(5), cache.lastHeight)
	require.Equal(t, 3, len(cache.sizesPerHeight))

	require.NotNil(t, cache.Add(5, 55))
	require.Equal(t, uint64(5), cache.lastHeight)
	require.Equal(t, 3, len(cache.sizesPerHeight))

	require.NotNil(t, cache.Add(7, 77))
	require.Equal(t, uint64(5), cache.lastHeight)
	require.Equal(t, 3, len(cache.sizesPerHeight))

	require.Nil(t, cache.Add(6, 66))
	require.Equal(t, uint64(6), cache.lastHeight)
	require.Equal(t, 4, len(cache.sizesPerHeight))

	require.Nil(t, cache.Add(7, 77))
	require.Equal(t, uint64(7), cache.lastHeight)
	require.Equal(t, 4, len(cache.sizesPerHeight))

}

func Test_Get(t *testing.T) {
	cache := NewBlockSizesCache(4)
	cache.Initialize(func(height uint64) (size int, present bool) {
		switch height {
		case 1:
			return 10, true
		case 2:
			return 20, true
		case 3:
			return 30, true
		case 4:
			return 40, true
		case 5:
			return 50, true
		default:
			return 0, false
		}
	})

	size, err := cache.Get(1)
	require.Equal(t, 10, size)
	require.Nil(t, err)

	_, err = cache.Get(3)
	require.NotNil(t, err)
	require.Equal(t, 1, len(cache.sizesPerHeight))

	cache.Get(2)
	cache.Get(3)
	cache.Get(4)
	cache.Get(5)
	require.Equal(t, 4, len(cache.sizesPerHeight))

	_, err = cache.Get(6)
	require.NotNil(t, err)
	require.Equal(t, 4, len(cache.sizesPerHeight))
	require.Equal(t, uint64(5), cache.lastHeight)

	_, err = cache.Get(3)
	require.Nil(t, err)
}

func Test_Reset(t *testing.T) {
	cache := NewBlockSizesCache(4)

	require.Nil(t, cache.Add(3, 33))
	require.Nil(t, cache.Add(4, 44))
	require.Nil(t, cache.Add(5, 55))
	require.Equal(t, uint64(5), cache.lastHeight)
	require.Equal(t, 3, len(cache.sizesPerHeight))

	require.NotNil(t, cache.Add(5, 55))
	require.Equal(t, uint64(5), cache.lastHeight)
	require.Equal(t, 3, len(cache.sizesPerHeight))

	require.NotNil(t, cache.Add(7, 77))
	require.Equal(t, uint64(5), cache.lastHeight)
	require.Equal(t, 3, len(cache.sizesPerHeight))

	cache.Reset()

	require.Nil(t, cache.Add(7, 77))
	require.Equal(t, uint64(7), cache.lastHeight)
	require.Equal(t, 1, len(cache.sizesPerHeight))

}
