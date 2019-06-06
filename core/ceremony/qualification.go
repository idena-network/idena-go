package ceremony

import (
	mapset "github.com/deckarep/golang-set"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/crypto"
	"idena-go/database"
	"idena-go/log"
)

type qualification struct {
	shortAnswers  map[common.Address][]byte
	longAnswers   map[common.Address][]byte
	epochDb       *database.EpochDb
	log           log.Logger
	hasNewAnswers bool
}

func NewQualification(epochDb *database.EpochDb) *qualification {
	return &qualification{
		epochDb:      epochDb,
		log:          log.New(),
		shortAnswers: make(map[common.Address][]byte),
		longAnswers:  make(map[common.Address][]byte),
	}
}

func (q *qualification) addAnswers(short bool, sender common.Address, txPayload []byte) {
	q.hasNewAnswers = true
	if short {
		q.shortAnswers[sender] = txPayload
	} else {
		q.longAnswers[sender] = txPayload
	}
}

func (q *qualification) persistAnswers() {
	if !q.hasNewAnswers {
		return
	}

	var short, long []database.DbAnswer
	for k, v := range q.shortAnswers {
		short = append(short, database.DbAnswer{
			Addr: k,
			Ans:  v,
		})
	}
	for k, v := range q.longAnswers {
		long = append(long, database.DbAnswer{
			Addr: k,
			Ans:  v,
		})
	}

	q.epochDb.WriteAnswers(short, long)

	q.hasNewAnswers = false
}

func (q *qualification) restoreAnswers() {
	short, long := q.epochDb.ReadAnswers()

	for _, item := range short {
		q.shortAnswers[item.Addr] = item.Ans
	}

	for _, item := range long {
		q.longAnswers[item.Addr] = item.Ans
	}
}

func (q *qualification) qualifyFlips(totalFlipsCount uint, candidates []*candidate, flipsPerCandidate [][]int) []FlipQualification {

	data := make([]struct {
		answer []types.Answer
		easy   []bool
	}, totalFlipsCount)

	for i := 0; i < len(flipsPerCandidate); i++ {
		candidate := candidates[i]
		flips := flipsPerCandidate[i]
		addr, _ := crypto.PubKeyBytesToAddress(candidate.PubKey)
		answerBytes := q.longAnswers[addr]

		// candidate didn't send long answers
		if answerBytes == nil {
			continue
		}

		answers := types.NewAnswersFromBits(uint(len(flips)), answerBytes)

		for j := uint(0); j < uint(len(flips)); j++ {
			answer, easy := answers.Answer(j)
			flipIdx := flips[j]

			data[flipIdx].answer = append(data[flipIdx].answer, answer)
			data[flipIdx].easy = append(data[flipIdx].easy, easy)
		}
	}

	result := make([]FlipQualification, totalFlipsCount)

	for i := 0; i < len(data); i++ {
		result[i] = qualifyOneFlip(data[i].answer)
	}

	return result
}

func (q *qualification) qualifyCandidate(candidate common.Address, flipQualificationMap map[int]FlipQualification,
	shortFlipsToSolve []int, longFlipsToSolve []int,
	shortSession bool, notApprovedFlips mapset.Set) (point float32, qualifiedFlipsCount uint32) {

	var answerBytes []byte
	var flipsToSolve []int
	if shortSession {
		answerBytes = q.shortAnswers[candidate]
		flipsToSolve = shortFlipsToSolve
	} else {
		answerBytes = q.longAnswers[candidate]
		flipsToSolve = longFlipsToSolve
	}

	// candidate didn't send answers
	if answerBytes == nil {
		return 0, 0
	}
	answers := types.NewAnswersFromBits(uint(len(flipsToSolve)), answerBytes)
	var shortAnswers *types.Answers
	if shortSession {
		shortAnswers = answers
	} else {
		shortAnswerBytes := q.shortAnswers[candidate]
		if shortAnswerBytes != nil {
			shortAnswers = types.NewAnswersFromBits(uint(len(shortFlipsToSolve)), shortAnswerBytes)
		}
	}

	for i, flipIdx := range flipsToSolve {
		qual := flipQualificationMap[flipIdx]
		status := getFlipStatusForCandidate(flipIdx, qual.status, notApprovedFlips, shortFlipsToSolve, shortAnswers)
		answer, _ := answers.Answer(uint(i))
		switch status {
		case Qualified:
			if qual.answer == answer {
				point += 1
			}
			qualifiedFlipsCount += 1
		case WeaklyQualified:
			if qual.answer == answer {
				point += 1
				qualifiedFlipsCount += 1
			} else if qual.answer != types.Inappropriate {
				point += 0.5
				qualifiedFlipsCount += 1
			}
		}
	}
	return point, qualifiedFlipsCount
}

func getFlipStatusForCandidate(flipIdx int, baseStatus FlipStatus, notApprovedFlips mapset.Set,
	shortFlipsToSolve []int, shortAnswers *types.Answers) FlipStatus {
	if baseStatus == NotQualified || !notApprovedFlips.Contains(flipIdx) {
		return baseStatus
	}
	if shortAnswers == nil {
		return NotQualified
	}
	shortFlipToSolveIdx := pos(shortFlipsToSolve, flipIdx)
	if shortFlipToSolveIdx == -1 {
		return baseStatus
	}
	shortAnswer, _ := shortAnswers.Answer(uint(shortFlipToSolveIdx))
	if shortAnswer == types.None {
		return NotQualified
	}
	return baseStatus
}

func pos(nums []int, num int) int {
	for i, n := range nums {
		if n == num {
			return i
		}
	}
	return -1
}

func getAnswersCount(a []types.Answer) (left uint, right uint, inappropriate uint) {
	for k := 0; k < len(a); k++ {
		if a[k] == types.Left {
			left++
		}
		if a[k] == types.Right {
			right++
		}
		if a[k] == types.Inappropriate {
			inappropriate++
		}
	}

	return left, right, inappropriate
}

func qualifyOneFlip(a []types.Answer) FlipQualification {
	left, right, inapp := getAnswersCount(a)
	totalAnswersCount := len(a)

	if float32(left)/float32(totalAnswersCount) >= 0.75 {
		return FlipQualification{
			answer: types.Left,
			status: Qualified,
		}
	}

	if float32(right)/float32(totalAnswersCount) >= 0.75 {
		return FlipQualification{
			answer: types.Right,
			status: Qualified,
		}
	}

	if float32(inapp)/float32(totalAnswersCount) >= 0.75 {
		return FlipQualification{
			answer: types.Inappropriate,
			status: Qualified,
		}
	}

	if float32(left)/float32(totalAnswersCount) >= 0.66 {
		return FlipQualification{
			answer: types.Left,
			status: WeaklyQualified,
		}
	}

	if float32(right)/float32(totalAnswersCount) >= 0.66 {
		return FlipQualification{
			answer: types.Right,
			status: WeaklyQualified,
		}
	}

	if float32(inapp)/float32(totalAnswersCount) >= 0.5 {
		return FlipQualification{
			answer: types.Inappropriate,
			status: WeaklyQualified,
		}
	}

	return FlipQualification{
		answer: types.None,
		status: NotQualified,
	}
}
