package protocol

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/core/state"
)

type batch struct {
	p       *peer
	from    uint64
	to      uint64
	headers chan *block
}

type block struct {
	Header       *types.Header
	Cert         *types.BlockCert        `rlp:"nil"`
	IdentityDiff *state.IdentityStateDiff `rlp:"nil"`
}

type blockRange struct {
	BatchId uint32
	Blocks  []*block
}
