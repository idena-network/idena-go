package protocol

import "github.com/idena-network/idena-go/blockchain/types"

type batch struct {
	p       *peer
	from    uint64
	to      uint64
	headers chan *types.Header
}

type blockRange struct {
	BatchId uint32
	Headers []*types.Header
}
