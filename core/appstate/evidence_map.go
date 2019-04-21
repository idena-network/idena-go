package appstate

import (
	"bytes"
	"errors"
	"github.com/asaskevich/EventBus"
	"github.com/deckarep/golang-set"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/constants"
)

type EvidenceMap struct {
	answersHashes map[common.Address]common.Hash
	bus           EventBus.Bus
}

func NewEvidenceMap(bus EventBus.Bus) *EvidenceMap {
	m := &EvidenceMap{bus: bus, answersHashes: make(map[common.Address]common.Hash)}
	bus.Subscribe(constants.NewTxEvent, m.newTx)
	return m
}

func (m *EvidenceMap) ValidateTx(tx *types.Transaction) error {
	if v, ok := m.answersHashes[*tx.To]; ok {
		if bytes.Compare(v.Bytes(), tx.Payload) != 0 {
			return errors.New("another answer was published already")
		}
	}
	return nil
}

func (m *EvidenceMap) newTx(tx *types.Transaction) {
	if tx.Type != types.SubmitAnswers {
		return
	}
	if err := m.ValidateTx(tx); err != nil {
		return
	}
	m.answersHashes[*tx.To] = common.BytesToHash(tx.Payload)
}

func (m *EvidenceMap) CalculateBitmap(candidates []common.Address, ignored []common.Address) []byte {
	ignoredSet := mapset.NewSet()

	for _, ignore := range ignored {
		ignoredSet.Add(ignore)
	}
	rmap := common.NewBitmap(uint32(len(candidates)))
	for i, candidate := range candidates {
		if ignoredSet.Contains(candidate) {
			continue
		}
		if _, ok := m.answersHashes[candidate]; ok {
			rmap.Add(uint32(i))
		}
	}

	buf := new(bytes.Buffer)
	rmap.WriteTo(buf)
	return buf.Bytes()
}
