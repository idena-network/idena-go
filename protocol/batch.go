package protocol

import "github.com/idena-network/idena-go/blockchain/types"

type batch struct {
	p       *peer
	from    uint64
	to      uint64
	headers chan *block
}

type block struct {
	header       *types.Header
	cert         *types.BlockCert
	identityDiff *state.IdentityStateDiff
}

type blockRange struct {
	BatchId uint32
	Headers []*block
}
