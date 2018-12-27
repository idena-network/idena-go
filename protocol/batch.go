package protocol

import "idena-go/blockchain/types"

type batch struct {
	p    *peer
	from uint64
	to   uint64
	blocks chan *types.Block
}
