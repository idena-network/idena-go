package ceremony

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
)

type candidate struct {
	Address    common.Address
	PubKey     []byte
	Generation uint32
	Code       []byte
	IsAuthor   bool
}

type FlipStatus byte

const (
	NotQualified    FlipStatus = 0
	Qualified       FlipStatus = 1
	WeaklyQualified FlipStatus = 2
	QualifiedByNone FlipStatus = 3
)

type FlipQualification struct {
	status FlipStatus
	answer types.Answer
	grade  types.Grade
}
