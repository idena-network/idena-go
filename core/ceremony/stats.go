package ceremony

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
)

type Stats struct {
	IdentitiesPerAddr map[common.Address]*IdentityStats
	FlipsPerIdx       map[int]*FlipStats
	FlipCids          [][]byte
}

type IdentityStats struct {
	ShortPoint float32
	ShortFlips uint32
	LongPoint  float32
	LongFlips  uint32
	State      state.IdentityState
}

type FlipStats struct {
	ShortAnswers []FlipAnswerStats
	LongAnswers  []FlipAnswerStats
	Status       FlipStatus
	Answer       types.Answer
}

type FlipAnswerStats struct {
	Respondent common.Address
	Answer     types.Answer
}

func NewStats() *Stats {
	return &Stats{
		IdentitiesPerAddr: make(map[common.Address]*IdentityStats),
		FlipsPerIdx:       make(map[int]*FlipStats),
	}
}
