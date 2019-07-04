package snapshot

import (
	"github.com/idena-network/idena-go/common"
)

const (
	BlockSize = 10000
)

type KeyValue struct {
	Key   []byte
	Value []byte
}

type Block struct {
	Data []*KeyValue
}

type Manifest struct {
	Root   common.Hash
	Height uint64
	Cid    []byte
}

func (sb *Block) Full() bool {
	return len(sb.Data) >= BlockSize
}

func (sb *Block) Add(key, value []byte) {
	sb.Data = append(sb.Data, &KeyValue{
		Key:   key,
		Value: value,
	})
}
