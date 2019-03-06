package types

import "idena-go/common"

type TransactionIndex struct {
	BlockHash common.Hash

	// block's cid in ipfs
	Cid []byte
	// tx index in block's body
	Idx uint32
}
