package types

import "github.com/idena-network/idena-go/common"

type TransactionIndex struct {
	BlockHash common.Hash
	// tx index in block's body
	Idx uint16
}
