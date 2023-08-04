package types

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/shopspring/decimal"
)

type ValidationStats struct {
	Shards map[common.ShardId]*ValidationShardStats
	Failed bool
}

type ValidationShardStats struct {
	IdentitiesPerAddr map[common.Address]*IdentityStats
	FlipsPerIdx       map[int]*FlipStats
	FlipCids          [][]byte
	WrongGradeReasons map[common.Address]WrongGradeReason
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
	Grade        types.Grade
	GradeScore   decimal.Decimal
}

type FlipAnswerStats struct {
	Index      int
	Respondent common.Address
	Answer     types.Answer
	Grade      types.Grade
	Point      float32
	Considered bool
}

func NewValidationStats() *ValidationShardStats {
	return &ValidationShardStats{
		IdentitiesPerAddr: make(map[common.Address]*IdentityStats),
		FlipsPerIdx:       make(map[int]*FlipStats),
	}
}

type WrongGradeReason uint32

const (
	TooManyReports WrongGradeReason = 1 << iota
	NoApproves
	TooManyIncreasedApproves
)
