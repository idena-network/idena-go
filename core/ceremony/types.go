package ceremony

import (
	"idena-go/blockchain/types"
)

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
