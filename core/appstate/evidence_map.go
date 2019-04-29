package appstate

import (
	"bytes"
	"github.com/asaskevich/EventBus"
	"github.com/deckarep/golang-set"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/constants"
	"sync"
	"time"
)

var (
	ShortSessionDuration = time.Minute * 5
)

type EvidenceMap struct {
	answersSet       mapset.Set
	bus              EventBus.Bus
	shortSessionTime *time.Time
	mutex            *sync.Mutex
}

func NewEvidenceMap(bus EventBus.Bus) *EvidenceMap {
	m := &EvidenceMap{
		bus:        bus,
		answersSet: mapset.NewSet(),
	}
	bus.Subscribe(constants.NewTxEvent, m.newTx)
	return m
}

func (m *EvidenceMap) newTx(tx *types.Transaction) {
	if tx.Type != types.SubmitAnswersHashTx {
		return
	}

	//TODO : m.shortSessionTime == nil ?
	if m.shortSessionTime == nil || m.shortSessionTime != nil && time.Now().Sub(*m.shortSessionTime) < ShortSessionDuration {
		m.answersSet.Add(*tx.To)
	}
}

func (m *EvidenceMap) CalculateApprovedCandidates(candidates []common.Address, maps [][]byte) []common.Address {
	score := make(map[uint32]int)
	minScore := len(candidates)/2 + 1

	for _, bm := range maps {
		bitmap := common.NewBitmap(uint32(len(candidates)))
		bitmap.Read(bm)

		for _, v := range bitmap.ToArray() {
			score[v] ++
		}
	}
	var result []common.Address

	for i, c := range candidates {
		if score[uint32(i)] >= minScore {
			result = append(result, c)
		}
	}
	return result
}

func (m *EvidenceMap) CalculateBitmap(candidates []common.Address, additional []common.Address) []byte {
	additionalSet := mapset.NewSet()

	for _, add := range additional {
		additionalSet.Add(add)
	}
	rmap := common.NewBitmap(uint32(len(candidates)))
	for i, candidate := range candidates {
		if additionalSet.Contains(candidate) {
			rmap.Add(uint32(i))
			continue
		}
		if m.answersSet.Contains(candidate) {
			rmap.Add(uint32(i))
		}
	}

	buf := new(bytes.Buffer)
	rmap.WriteTo(buf)
	return buf.Bytes()
}

func (m *EvidenceMap) SetShortSessionTime(timestamp *time.Time) {
	m.shortSessionTime = timestamp
}

func (m *EvidenceMap) GetShortSessionBeginningTime() time.Time {
	return *m.shortSessionTime
}

func (m *EvidenceMap) GetShortSessionEndingTime() time.Time {
	return m.shortSessionTime.Add(ShortSessionDuration)
}
