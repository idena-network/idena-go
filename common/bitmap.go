package common

import (
	"bytes"
	"github.com/RoaringBitmap/roaring"
	"math/big"
)

const (
	serializeDefault = byte(0x1)
	serializeBigInt  = byte(0x2)
)

type Bitmap struct {
	rmap *roaring.Bitmap
	size uint32
}

func NewBitmap(size uint32) *Bitmap {
	return &Bitmap{size: size, rmap: roaring.NewBitmap()}
}

func (m *Bitmap) Add(value uint32) {
	if value >= m.size {
		panic("value is out of range")
	}
	m.rmap.Add(value)

}

func (m *Bitmap) Contains(value uint32) bool {
	return m.rmap.Contains(value)
}

func (m *Bitmap) WriteTo(buffer *bytes.Buffer) {
	if m.rmap.HasRunCompression() {
		m.rmap.RunOptimize()
	}
	if m.rmap.GetSerializedSizeInBytes() > uint64(m.size/8+1) {
		bits := big.NewInt(0)
		buffer.WriteByte(serializeBigInt)
		for _, v := range m.rmap.ToArray() {
			t := big.NewInt(1)
			bits.Or(bits, t.Lsh(t, uint(v)))
		}
		buffer.Write(bits.Bytes())
	} else {
		buffer.WriteByte(serializeDefault)
		m.rmap.WriteTo(buffer)
	}
}

func (m *Bitmap) Read(data []byte) {
	m.rmap = roaring.NewBitmap()
	if data[0] == serializeDefault {
		buf := bytes.NewBuffer(data[1:])
		m.rmap.ReadFrom(buf)
	} else {
		bits := big.NewInt(0)
		bits.SetBytes(data[1:])
		for i := uint32(0); i < m.size; i++ {
			if bits.Bit(int(i)) == 1 {
				m.rmap.Add(i)
			}
		}
	}
}
