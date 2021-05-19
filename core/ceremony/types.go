package ceremony

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
)

type candidate struct {
	Address  common.Address
	PubKey   []byte
	IsAuthor bool
}

type candidatesOfShard struct {
	candidates     []*candidate
	flips          [][]byte
	flipsPerAuthor map[int][][]byte
	flipAuthorMap  map[string]common.Address
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
