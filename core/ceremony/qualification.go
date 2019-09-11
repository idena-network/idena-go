package ceremony

import (
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"sync"
)

type qualification struct {
	shortAnswers  map[common.Address][]byte
	longAnswers   map[common.Address][]byte
	proofs        map[common.Address][]byte
	epochDb       *database.EpochDb
	log           log.Logger
	hasNewAnswers bool
	lock          sync.RWMutex
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
	var m map[common.Address][]byte

	if short {
		m = q.shortAnswers
	} else {
		m = q.longAnswers
	}

	if _, ok := m[sender]; ok {
		return
	}
	m[sender] = txPayload

	q.hasNewAnswers = true
}

func (q *qualification) addProof(sender common.Address, bytes []byte) {
	if _, ok := q.proofs[sender]; ok {
		return
	}

	q.lock.Lock()
	defer q.lock.Unlock()
	q.proofs[sender] = bytes
}

func (q *qualification) persist() {
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
	var proofs []database.DbProof
	for k, v := range q.proofs {
		proofs = append(proofs, database.DbProof{
			Addr:  k,
			Proof: v,
		})
	}

	q.epochDb.WriteAnswers(short, long)
	q.epochDb.WriteProofs(proofs)

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

	proofs := q.epochDb.ReadProofs()

	for _, item := range proofs {
		q.proofs[item.Addr] = item.Proof
	}
}

func (q *qualification) qualifyFlips(totalFlipsCount uint, candidates []*candidate, flipsPerCandidate [][]int) []FlipQualification {

	data := make([]struct {
		answer     []types.Answer
		wrongWords []bool
	}, totalFlipsCount)

	for i := 0; i < len(flipsPerCandidate); i++ {
		candidate := candidates[i]
		flips := flipsPerCandidate[i]
		answerBytes := q.longAnswers[candidate.Address]

		// candidate didn't send long answers
		if answerBytes == nil {
			continue
		}

		answers := types.NewAnswersFromBits(uint(len(flips)), answerBytes)

		for j := uint(0); j < uint(len(flips)); j++ {
			answer, wrongWords := answers.Answer(j)
			flipIdx := flips[j]

			data[flipIdx].answer = append(data[flipIdx].answer, answer)
			data[flipIdx].wrongWords = append(data[flipIdx].wrongWords, wrongWords)
		}
	}

	result := make([]FlipQualification, totalFlipsCount)

	for i := 0; i < len(data); i++ {
		result[i] = qualifyOneFlip(data[i].answer)
		result[i].wrongWords = qualifyWrongWords(data[i].wrongWords)
	}

	return result
}

func (q *qualification) qualifyCandidate(candidate common.Address, flipQualificationMap map[int]FlipQualification,
	flipsToSolve []int, shortSession bool, notApprovedFlips mapset.Set) (point float32, qualifiedFlipsCount uint32, flipAnswers map[int]FlipAnswerStats) {

	var answerBytes []byte
	if shortSession {
		hash := q.epochDb.GetAnswerHash(candidate)
		answerBytes = q.shortAnswers[candidate]
		if answerBytes != nil && hash != rlp.Hash(answerBytes) {
			return 0, uint32(len(flipsToSolve)), nil
		}

	} else {
		answerBytes = q.longAnswers[candidate]
	}
	// candidate didn't send answers
	if answerBytes == nil {
		return 0, 0, nil
	}
	answers := types.NewAnswersFromBits(uint(len(flipsToSolve)), answerBytes)
	availableExtraFlips := 0

	if shortSession {
		for i := 0; i < int(common.ShortSessionFlipsCount()) && i < len(flipsToSolve) && availableExtraFlips < int(common.ShortSessionExtraFlipsCount()); i++ {
			answer, _ := answers.Answer(uint(i))
			if notApprovedFlips.Contains(flipsToSolve[i]) && answer == types.None {
				availableExtraFlips++
			}
		}
	}
	flipAnswers = make(map[int]FlipAnswerStats, len(flipsToSolve)+availableExtraFlips)

	for i, flipIdx := range flipsToSolve {
		qual := flipQualificationMap[flipIdx]
		status := getFlipStatusForCandidate(flipIdx, i, qual.status, notApprovedFlips, answers, shortSession)
		answer, _ := answers.Answer(uint(i))

		//extra flip
		if shortSession && i >= int(common.ShortSessionFlipsCount()) {
			if availableExtraFlips > 0 && answer != types.None {
				availableExtraFlips -= 1
			} else {
				continue
			}
		}

		var answerPoint float32
		switch status {
		case Qualified:
			if qual.answer == answer {
				answerPoint = 1
			}
			qualifiedFlipsCount += 1
		case WeaklyQualified:
			if qual.answer == answer {
				answerPoint = 1
				qualifiedFlipsCount += 1
			} else if qual.answer != types.Inappropriate {
				answerPoint = 0.5
				qualifiedFlipsCount += 1
			}
		}
		point += answerPoint
		flipAnswers[flipIdx] = FlipAnswerStats{
			Respondent: candidate,
			Answer:     answer,
			Point:      answerPoint,
		}
	}
	return point, qualifiedFlipsCount, flipAnswers
}

func (q *qualification) GetProof(addr common.Address) []byte {
	q.lock.RLock()
	defer q.lock.RUnlock()

	return q.proofs[addr]
}

func getFlipStatusForCandidate(flipIdx int, flipsToSolveIdx int, baseStatus FlipStatus, notApprovedFlips mapset.Set,
	answers *types.Answers, shortSession bool) FlipStatus {

	if !shortSession || baseStatus == NotQualified || !notApprovedFlips.Contains(flipIdx) {
		return baseStatus
	}
	shortAnswer, _ := answers.Answer(uint(flipsToSolveIdx))
	if shortAnswer == types.None {
		return NotQualified
	}
	return baseStatus
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

func qualifyWrongWords(data []bool) bool {
	wrongCount := 0
	for _, item := range data {
		if item {
			wrongCount += 1
		}
	}
	return float32(wrongCount)/float32(len(data)) >= 0.66
}
