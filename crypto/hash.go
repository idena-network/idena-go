package crypto

import (
	"github.com/idena-network/idena-go/crypto/sha3"
	"hash"
	"sync"
)

type SignatureHasher interface {
	ToSignatureBytes() ([]byte, error)
}

var keccak256Pool = sync.Pool{New: func() interface{} {
	return sha3.NewKeccak256()
}}

func Hash(data []byte) [32]byte {
	h, ok := keccak256Pool.Get().(hash.Hash)
	if !ok {
		h = sha3.NewKeccak256()
	}
	defer keccak256Pool.Put(h)
	h.Reset()

	var b [32]byte

	h.Write(data)
	h.Sum(b[:0])

	return b
}

func SignatureHash(h SignatureHasher) [32]byte {
	b, err := h.ToSignatureBytes()
	if err != nil {
		return [32]byte{}
	}
	return Hash(b)
}

var shake128Pool = sync.Pool{New: func() interface{} {
	return sha3.NewShake128()
}}

func Hash128(data []byte) [16]byte {
	h, ok := shake128Pool.Get().(sha3.ShakeHash)
	if !ok {
		h = sha3.NewShake128()
	}
	defer shake128Pool.Put(h)
	h.Reset()

	var b [16]byte

	h.Write(data)
	h.Read(b[:])

	return b
}
