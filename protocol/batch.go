package protocol

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/core/state"
	"github.com/libp2p/go-libp2p-core/peer"
)

type batch struct {
	p       *protoPeer
	from    uint64
	to      uint64
	headers chan *block
}

type block struct {
	Header       *types.Header
	Cert         *types.BlockCert     `rlp:"nil"`
	IdentityDiff *state.IdentityStateDiff `rlp:"nil"`
}

type blockPeer struct {
	block
	peerId peer.ID
}

type blockRange struct {
	BatchId uint32
	Blocks  []*block
}
