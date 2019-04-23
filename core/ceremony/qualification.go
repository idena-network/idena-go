package ceremony

import (
	"idena-go/common"
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
