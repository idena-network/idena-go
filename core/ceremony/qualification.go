package ceremony

import (
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/crypto"
	"idena-go/log"
)

type qualification struct {
	shortAnswers map[common.Address][]byte
	longAnswers  map[common.Address][]byte
	epochDb      *EpochDb
	log          log.Logger
}

func NewQualification(epochDb *EpochDb) *qualification {
	return &qualification{
		epochDb: epochDb,
		log:     log.New(),
	}
}

func (q *qualification) addAnswers(short bool, sender common.Address, txPayload []byte) {
	if short {
		q.shortAnswers[sender] = txPayload
	} else {
		q.longAnswers[sender] = txPayload
	}
}

func (q *qualification) persistAnswers() {
	var short, long []dbAnswer
	for k, v := range q.shortAnswers {
		short = append(short, dbAnswer{
			Addr: k,
			Ans:  v,
		})
	}
	for k, v := range q.longAnswers {
		long = append(long, dbAnswer{
			Addr: k,
			Ans:  v,
		})
	}

	q.epochDb.WriteAnswers(short, long)
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

func (q *qualification) qualifyFlips(totalFlipsCount uint, flipsPerAddress uint, participants []*participant, flipsPerCandidate [][]int) []FlipQualification {

	data := make([]struct {
		answer []types.Answer
		easy   []bool
	}, totalFlipsCount)

	for i := 0; i < len(flipsPerCandidate); i++ {
		participant := participants[i]
		flips := flipsPerCandidate[i]
		addr, _ := crypto.PubKeyBytesToAddress(participant.PubKey)
		answerBytes := q.longAnswers[addr]

		// candidate didn't send long answers
		if answerBytes == nil {
			continue
		}

		answers := types.NewAnswersFromBits(flipsPerAddress, answerBytes)

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
