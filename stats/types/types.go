package types

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
)

type ValidationStats struct {
	IdentitiesPerAddr map[common.Address]*IdentityStats
	FlipsPerIdx       map[int]*FlipStats
	FlipCids          [][]byte
	Failed            bool
}

type IdentityStats struct {
	ShortPoint        float32
	ShortFlips        uint32
	LongPoint         float32
	LongFlips         uint32
	Approved          bool
	Missed            bool
	ShortFlipsToSolve []int
	LongFlipsToSolve  []int
}

type FlipStats struct {
	ShortAnswers []FlipAnswerStats
	LongAnswers  []FlipAnswerStats
	Status       byte
	Answer       types.Answer
	WrongWords   bool
}

type FlipAnswerStats struct {
	Respondent common.Address
	Answer     types.Answer
	WrongWords bool
	Point      float32
}

func NewValidationStats() *ValidationStats {
	return &ValidationStats{
		IdentitiesPerAddr: make(map[common.Address]*IdentityStats),
		FlipsPerIdx:       make(map[int]*FlipStats),
	}
}
