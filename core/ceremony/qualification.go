package ceremony

import (
	"bytes"
	"idena-go/common"
	"idena-go/database"
	"idena-go/log"
	"idena-go/rlp"
)

type qualification struct {
	shortAnswers map[common.Address][]byte
	longAnswers  map[common.Address][]byte
	epochDb      *database.EpochDb
	log          log.Logger
}

func NewQualification(epochDb *database.EpochDb) *qualification {
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

type dbAnswer struct {
	Addr common.Address
	Ans  []byte
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

	shortBytes, _ := rlp.EncodeToBytes(short)
	longBytes, _ := rlp.EncodeToBytes(long)

	q.epochDb.WriteAnswers(shortBytes, longBytes)
}

func (q *qualification) restoreAnswers() {
	short, long := q.epochDb.ReadAnswers()

	if short != nil {
		var s []dbAnswer
		if err := rlp.Decode(bytes.NewReader(short), s); err != nil {
			q.log.Error("invalid short answers rlp", "err", err)
		} else {
			for _, item := range s {
				q.shortAnswers[item.Addr] = item.Ans
			}
		}
	}

	if long != nil {
		var s []dbAnswer
		if err := rlp.Decode(bytes.NewReader(long), s); err != nil {
			q.log.Error("invalid long answers rlp", "err", err)
		} else {
			for _, item := range s {
				q.longAnswers[item.Addr] = item.Ans
			}
		}
	}
}
