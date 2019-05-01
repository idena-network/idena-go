package ceremony

import (
	"idena-go/blockchain/types"
	"idena-go/common"
)

type dbAnswer struct {
	Addr common.Address
	Ans  []byte
}

type candidate struct {
	PubKey []byte
}

type FlipStatus byte

const (
	NotQualified    FlipStatus = 0
	Qualified       FlipStatus = 1
	WeaklyQualified FlipStatus = 2
)

type FlipQualification struct {
	status FlipStatus
	answer types.Answer
}
