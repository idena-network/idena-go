package ceremony

import (
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/vrf"
	"github.com/idena-network/idena-go/database"
	"github.com/idena-network/idena-go/log"
	statsTypes "github.com/idena-network/idena-go/stats/types"
	math2 "math"
	"sync"
)

type qualification struct {
	config       *config.Config
	shortAnswers map[common.Address][]byte
	longAnswers  map[common.Address][]byte
	epochDb      *database.EpochDb
	log          log.Logger
	hasChanges   bool
	lock         sync.RWMutex
}

func NewQualification(config *config.Config, epochDb *database.EpochDb) *qualification {
	return &qualification{
		config:       config,
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

	q.lock.Lock()
	defer q.lock.Unlock()
	if _, ok := m[sender]; ok {
		return
	}
	m[sender] = txPayload

	q.hasChanges = true
}

func (q *qualification) removeAnswers(short bool, sender common.Address) {
	var m map[common.Address][]byte

	if short {
		m = q.shortAnswers
	} else {
		m = q.longAnswers
	}

	q.lock.Lock()
	defer q.lock.Unlock()
	if _, ok := m[sender]; !ok {
		return
	}
	delete(m, sender)
	q.hasChanges = true
}

func (q *qualification) persist() {
	if !q.hasChanges {
		return
	}
	q.lock.RLock()
	defer q.lock.RUnlock()

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

	q.hasChanges = false
}

func (q *qualification) restore() {
	short, long := q.epochDb.ReadAnswers()

	q.lock.Lock()
	defer q.lock.Unlock()
	for _, item := range short {
		q.shortAnswers[item.Addr] = item.Ans
	}

	for _, item := range long {
		q.longAnswers[item.Addr] = item.Ans
	}
}

func (q *qualification) qualifyFlips(totalFlipsCount uint, candidates []*candidate, flipsPerCandidate [][]int) ([]FlipQualification, *reportersToReward) {

	q.lock.RLock()
	defer q.lock.RUnlock()

	data := make([]struct {
		answers     []types.Answer
		totalGrade  int
		gradesCount int
	}, totalFlipsCount)

	reportersToReward := newReportersToReward()

	for candidateIdx := 0; candidateIdx < len(flipsPerCandidate); candidateIdx++ {
		candidate := candidates[candidateIdx]
		flips := flipsPerCandidate[candidateIdx]
		answerBytes := q.longAnswers[candidate.Address]
		attachment := attachments.ParseLongAnswerBytesAttachment(answerBytes)

		// candidate didn't send long answers
		if attachment == nil || len(attachment.Answers) == 0 {
			continue
		}

		flipsCount := len(flips)
		answers := types.NewAnswersFromBits(uint(flipsCount), attachment.Answers)
		for j := uint(0); j < uint(flipsCount); j++ {
			answer, grade := answers.Answer(j)
			flipIdx := flips[j]

			data[flipIdx].answers = append(data[flipIdx].answers, answer)
			if grade == types.GradeReported {
				reportersToReward.addReport(flipIdx, candidate.Address)
			} else if grade != types.GradeNone {
				data[flipIdx].totalGrade += int(grade)
				data[flipIdx].gradesCount++
			}
		}
		if flipsCount > 0 {
			ignoreReports := float32(reportersToReward.getReportedFlipsCountByReporter(candidate.Address))/float32(flipsCount) >= 0.34
			if ignoreReports {
				reportersToReward.deleteReporter(candidate.Address)
			}
		}
	}

	result := make([]FlipQualification, totalFlipsCount)

	for flipIdx := 0; flipIdx < len(data); flipIdx++ {
		result[flipIdx] = q.qualifyOneFlip(data[flipIdx].answers, reportersToReward.getFlipReportsCount(flipIdx), data[flipIdx].totalGrade, data[flipIdx].gradesCount)
		if result[flipIdx].grade != types.GradeReported {
			reportersToReward.deleteFlip(flipIdx)
		}
	}

	return result, reportersToReward
}

func (q *qualification) qualifyCandidate(candidate common.Address, flipQualificationMap map[int]FlipQualification,
	flipsToSolve []int, shortSession bool, notApprovedFlips mapset.Set) (point float32, qualifiedFlipsCount uint32, flipAnswers map[int]statsTypes.FlipAnswerStats, noQual bool, noAnswer bool) {

	q.lock.RLock()
	var answerBytes []byte
	if shortSession {
		answerBytes = q.shortAnswers[candidate]
	} else {
		answerBytes = q.longAnswers[candidate]
	}
	q.lock.RUnlock()

	// candidate didn't send answers
	if answerBytes == nil {
		return 0, 0, nil, false, true
	}

	if shortSession {
		attachment := attachments.ParseShortAnswerBytesAttachment(answerBytes)
		flipsCount := uint32(math.MinInt(int(common.ShortSessionFlipsCount()), len(flipsToSolve)))
		// can't parse
		if attachment == nil {
			return 0, flipsCount, nil, false, false
		}
		answerBytes = attachment.Answers
	} else {
		attachment := attachments.ParseLongAnswerBytesAttachment(answerBytes)
		flipsCount := uint32(len(flipsToSolve))
		// can't parse
		if attachment == nil {
			return 0, flipsCount, nil, false, false
		}
		answerBytes = attachment.Answers
		hash := q.epochDb.GetAnswerHash(candidate)
		shortAttachment := attachments.ParseShortAnswerBytesAttachment(q.shortAnswers[candidate])
		h, _ := vrf.HashFromProof(attachment.Proof)
		if shortAttachment == nil || hash != crypto.Hash(append(shortAttachment.Answers, attachment.Salt...)) || getWordsRnd(h) != shortAttachment.Rnd {
			return 0, flipsCount, nil, false, false
		}
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
	flipAnswers = make(map[int]statsTypes.FlipAnswerStats, len(flipsToSolve)+availableExtraFlips)

	for i, flipIdx := range flipsToSolve {
		qual := flipQualificationMap[flipIdx]
		status := getFlipStatusForCandidate(flipIdx, i, qual.status, notApprovedFlips, answers, shortSession)
		answer, grade := answers.Answer(uint(i))

		//extra flip
		if shortSession && i >= int(common.ShortSessionFlipsCount()) {
			if availableExtraFlips > 0 && answer != types.None {
				availableExtraFlips -= 1
			} else {
				continue
			}
		}

		var answerPoint float32
		if !shortSession || qual.grade != types.GradeReported {
			switch status {
			case Qualified:
				if qual.answer == answer {
					answerPoint = 1
				}
				qualifiedFlipsCount += 1
			case WeaklyQualified:
				switch {
				case qual.answer == answer:
					answerPoint = 1
					qualifiedFlipsCount += 1
					break
				case answer == types.None:
					qualifiedFlipsCount += 1
					break
				default:
					answerPoint = 0.5
					qualifiedFlipsCount += 1
				}
			}
			point += answerPoint
		}
		flipAnswers[flipIdx] = statsTypes.FlipAnswerStats{
			Respondent: candidate,
			Answer:     answer,
			Point:      answerPoint,
			Grade:      grade,
		}
	}
	return point, qualifiedFlipsCount, flipAnswers, qualifiedFlipsCount == 0, false
}

func (q *qualification) GetWordsRnd(addr common.Address) uint64 {
	q.lock.RLock()
	defer q.lock.RUnlock()

	attachment := attachments.ParseShortAnswerBytesAttachment(q.shortAnswers[addr])
	if attachment == nil {
		return 0
	}
	return attachment.Rnd
}

func getFlipStatusForCandidate(flipIdx int, flipsToSolveIdx int, baseStatus FlipStatus, notApprovedFlips mapset.Set,
	answers *types.Answers, shortSession bool) FlipStatus {

	if !shortSession || baseStatus == NotQualified || !notApprovedFlips.Contains(flipIdx) || baseStatus == QualifiedByNone {
		return baseStatus
	}
	shortAnswer, _ := answers.Answer(uint(flipsToSolveIdx))
	if shortAnswer == types.None {
		return NotQualified
	}
	return baseStatus
}

func getAnswersCount(a []types.Answer) (left uint, right uint, none uint) {
	for k := 0; k < len(a); k++ {
		if a[k] == types.Left {
			left++
		}
		if a[k] == types.Right {
			right++
		}
		if a[k] == types.None {
			none++
		}
	}

	return left, right, none
}

func (q *qualification) qualifyOneFlip(answers []types.Answer, reportsCount int, totalGrade int, gradesCount int) FlipQualification {
	left, right, none := getAnswersCount(answers)
	totalAnswersCount := float32(len(answers))

	reported := false
	switch len(answers) {
	case 1, 2, 3:
		reported = reportsCount >= len(answers)
	case 4:
		reported = reportsCount >= 3
	case 5:
		reported = reportsCount >= 4
	default:
		reported = float32(reportsCount)/float32(len(answers)) > 0.5
	}

	graded := float32(gradesCount)/float32(len(answers)) > 0.33
	var grade types.Grade
	switch {
	case reported:
		grade = types.GradeReported
		break
	case graded:
		grade = types.Grade(math2.Round(float64(totalGrade) / float64(gradesCount)))
		break
	default:
		grade = types.GradeD
	}

	if float32(left)/totalAnswersCount >= 0.75 {
		return FlipQualification{
			answer: types.Left,
			status: Qualified,
			grade:  grade,
		}
	}

	if float32(right)/totalAnswersCount >= 0.75 {
		return FlipQualification{
			answer: types.Right,
			status: Qualified,
			grade:  grade,
		}
	}

	if float32(left)/totalAnswersCount >= 0.66 {
		return FlipQualification{
			answer: types.Left,
			status: WeaklyQualified,
			grade:  grade,
		}
	}

	if float32(right)/totalAnswersCount >= 0.66 {
		return FlipQualification{
			answer: types.Right,
			status: WeaklyQualified,
			grade:  grade,
		}
	}

	if float32(none)/totalAnswersCount >= 0.66 {
		return FlipQualification{
			answer: types.None,
			status: QualifiedByNone,
			grade:  grade,
		}
	}

	return FlipQualification{
		answer: types.None,
		status: NotQualified,
		grade:  grade,
	}
}
