package common

import (
	"bytes"
	"github.com/RoaringBitmap/roaring"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBitmap_Size(t *testing.T) {
	size := uint32(8888)

	rmap := roaring.NewBitmap()
	bitmap := NewBitmap(size)

	for i := uint32(0); i < size; i++ {
		if i%3 == 0 {
			rmap.Add(i)
			bitmap.Add(i)
		}
	}

	buf := new(bytes.Buffer)
	rmap.WriteTo(buf)

	buf2 := new(bytes.Buffer)
	bitmap.WriteTo(buf2)

	require.True(t, buf.Len() >= buf2.Len())
	require.Equal(t, buf2.Len(), int(size/8+1))

	rmap = roaring.NewBitmap()
	bitmap = NewBitmap(size)

	rmap.Add(0)
	bitmap.Add(0)
	rmap.Add(size - 1)
	bitmap.Add(size - 1)

	buf = new(bytes.Buffer)
	rmap.WriteTo(buf)

	buf2 = new(bytes.Buffer)
	bitmap.WriteTo(buf2)
	require.True(t, buf.Len()+1 >= buf2.Len())
}

func TestBitmap_Contains(t *testing.T) {
	size := uint32(8888)
	bitmap := NewBitmap(size)

	for i := uint32(0); i < size; i++ {
		if i%3 == 0 {
			bitmap.Add(i)
		}
	}

	buf := new(bytes.Buffer)
	bitmap.WriteTo(buf)

	bitmap.Read(buf.Bytes())

	for i := uint32(0); i < size; i++ {
		if i%3 == 0 {
			require.True(t, bitmap.Contains(i))
		} else {
			require.False(t, bitmap.Contains(i))
		}
	}

	size = 2
	bitmap = NewBitmap(size)
	bitmap.Add(1)

	require.True(t, bitmap.Contains(1))

	buf = new(bytes.Buffer)
	bitmap.WriteTo(buf)

	bitmap.Read(buf.Bytes())

	require.True(t, bitmap.Contains(1))

	require.False(t, bitmap.Contains(0))
}

func TestBitmap_Serialize(t *testing.T) {
	size := uint32(8)
	bitmap := NewBitmap(size)

	for i := uint32(0); i < size; i++ {
		if i%3 == 0 {
			bitmap.Add(i)
		}
	}

	buf := new(bytes.Buffer)
	bitmap.WriteTo(buf)

	bitmap2 := NewBitmap(size)
	bitmap2.Read(buf.Bytes())

	require.True(t, bitmap.rmap.Equals(bitmap2.rmap))
}
