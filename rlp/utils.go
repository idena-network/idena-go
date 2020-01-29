package rlp

import (
	"github.com/idena-network/idena-go/crypto/sha3"
)

func Hash(x interface{}) (h [32]byte) {
	hw := sha3.NewKeccak256()
	Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

func Hash128(x interface{}) (h [16]byte) {
	hw := sha3.NewShake128()
	Encode(hw, x)
	hw.Read(h[:])
	return h
}
