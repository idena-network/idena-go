package common

import (
	"bytes"
	"encoding/binary"
	"github.com/willf/bloom"
)

const (
	K = 3
)

type SerializableBF struct {
	*bloom.BloomFilter
	data []uint64
}

func NewSerializableBF(n int) *SerializableBF {
	data := make([]uint64, bloomLength(n)/64)
	return &SerializableBF{bloom.From(data, K), data}
}

func NewSerializableBFFromData(data []byte) (*SerializableBF, error) {
	buf := make([]uint64, len(data)/binary.Size(uint64(0)))
	if err := binary.Read(bytes.NewReader(data), binary.BigEndian, buf); err != nil {
		return nil, err
	}
	return &SerializableBF{bloom.From(buf, K), buf}, nil
}

func (s *SerializableBF) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, s.data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *SerializableBF) Add(value []byte) {
	s.BloomFilter.Add(value)
}

func (s *SerializableBF) Has(value []byte) bool {
	return s.BloomFilter.Test(value)
}

func bloomLength(n int) (m int) {
	switch {
	case n < 8:
		return 64
	case n < 16:
		return 128
	case n < 32:
		return 256
	case n < 64:
		return 512
	case n < 128:
		return 1024
	case n < 256:
		return 2048
	default:
		return 4096
	}
}
