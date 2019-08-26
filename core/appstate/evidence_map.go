package appstate

import (
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/events"
	"sync"
	"time"
)

var (
	ShortSessionFlipKeyDeadline = time.Second * 30
)

type EvidenceMap struct {
	answersSet           mapset.Set
	keysSet              mapset.Set
	bus                  eventbus.Bus
	shortSessionTime     *time.Time
	shortSessionDuration time.Duration
	mutex                *sync.Mutex
}

func NewEvidenceMap(bus eventbus.Bus) *EvidenceMap {
	m := &EvidenceMap{
		bus:        bus,
		answersSet: mapset.NewSet(),
		keysSet:    mapset.NewSet(),
	}
	bus.Subscribe(events.NewTxEventID, func(e eventbus.Event) {
		newTxEvent := e.(*events.NewTxEvent)
		m.newTx(newTxEvent.Tx)
	})
	return m
}

func (m *EvidenceMap) newTx(tx *types.Transaction) {
	if tx.Type != types.SubmitAnswersHashTx {
		return
	}

	//TODO : m.shortSessionTime == nil ?
	if m.shortSessionTime == nil || m.shortSessionTime != nil && time.Now().UTC().Sub(*m.shortSessionTime) < m.shortSessionDuration {
		sender, _ := types.Sender(tx)
		m.answersSet.Add(sender)
	}
}

func (m *EvidenceMap) NewFlipsKey(author common.Address) {
	if m.shortSessionTime == nil || time.Now().UTC().Sub(*m.shortSessionTime) < ShortSessionFlipKeyDeadline {
		m.keysSet.Add(author)
	}
}

func (m *EvidenceMap) CalculateApprovedCandidates(candidates []common.Address, maps [][]byte) []common.Address {
	score := make(map[uint32]int)
	minScore := len(maps)/2 + 1

	for _, bm := range maps {
		bitmap := common.NewBitmap(uint32(len(candidates)))
		bitmap.Read(bm)

		for _, v := range bitmap.ToArray() {
			score[v]++
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

func (m *EvidenceMap) CalculateBitmap(candidates []common.Address, additional []common.Address, reqFlips func(common.Address) uint8) *common.Bitmap {
	additionalSet := mapset.NewSet()

	for _, add := range additional {
		additionalSet.Add(add)
	}
	rmap := common.NewBitmap(uint32(len(candidates)))
	for i, candidate := range candidates {
		if !m.keysSet.Contains(candidate) && reqFlips(candidate) > 0 {
			continue
		}
		if additionalSet.Contains(candidate) {
			rmap.Add(uint32(i))
			continue
		}
		if m.answersSet.Contains(candidate) {
			rmap.Add(uint32(i))
		}
	}
	return rmap
}

func (m *EvidenceMap) ContainsAnswer(candidate common.Address) bool {
	return m.answersSet.Contains(candidate)
}

func (m *EvidenceMap) ContainsKey(candidate common.Address) bool {
	return m.keysSet.Contains(candidate)
}

func (m *EvidenceMap) SetShortSessionTime(timestamp *time.Time, shortSessionDuration time.Duration) {
	m.shortSessionTime = timestamp
	m.shortSessionDuration = shortSessionDuration
}

func (m *EvidenceMap) GetShortSessionBeginningTime() *time.Time {
	return m.shortSessionTime
}

func (m *EvidenceMap) GetShortSessionEndingTime() *time.Time {
	if m.shortSessionTime == nil {
		return nil
	}
	endTime := m.shortSessionTime.Add(m.shortSessionDuration)
	return &endTime
}

func (m *EvidenceMap) IsCompleted() bool {
	endTime := m.GetShortSessionEndingTime()
	if endTime == nil {
		return false
	}
	return time.Now().UTC().After(*endTime)
}

func (m *EvidenceMap) Clear() {
	m.shortSessionTime = nil
	m.answersSet = mapset.NewSet()
	m.keysSet = mapset.NewSet()
}
