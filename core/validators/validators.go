package validators

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/state"
	"idena-go/crypto/sha3"
	"idena-go/rlp"
	"math/big"
	"sort"
)

type ValidatorsCache struct {
	s          *state.StateDB
	validNodes []*common.Address
}

func NewValidatorsCache(sdb *state.StateDB) *ValidatorsCache {
	return &ValidatorsCache{
		s: sdb,
	}
}

func (v *ValidatorsCache) Load() {
	v.loadValidNodes()
}

func (v *ValidatorsCache) GetActualValidators(seed types.Seed, round uint64, step uint16, limit int) mapset.Set {
	set := mapset.NewSet()
	cnt := new(big.Int).SetInt64(int64(len(v.validNodes)))
	for i := uint32(0); i < uint32(limit*3) && set.Cardinality() < limit; i++ {
		set.Add(*v.validNodes[indexGenerator(seed, round, step, i, cnt)])
	}
	if set.Cardinality() < limit {
		return nil
	}
	return set
}

func (v *ValidatorsCache) GetCountOfValidNodes() int {
	return len(v.validNodes)
}

func (v *ValidatorsCache) Contains(addr common.Address) bool {
	// TODO: we should use O(1) structure
	for _, p := range v.validNodes {
		if bytes.Compare(p[:], addr[:]) == 0 {
			return true
		}
	}
	return false
}
func (v *ValidatorsCache) RefreshIfUpdated(transactions []*types.Transaction) {
	shouldRefresh := false
	for _, tx := range transactions {
		if tx.Type == types.ApprovingTx || tx.Type == types.RevokeTx {
			shouldRefresh = true
			break
		}
	}
	if shouldRefresh {
		v.loadValidNodes()
	}
}

func (v *ValidatorsCache) loadValidNodes() {
	var nodes []*common.Address
	v.s.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])

		var data state.Identity
		if err := rlp.DecodeBytes(value, &data); err != nil {
			return false
		}

		if data.State == state.Verified {
			nodes = append(nodes, &addr)
		}

		return false
	})

	v.validNodes = sortValidNodes(nodes)
}

func sortValidNodes(nodes []*common.Address) []*common.Address {
	sort.SliceStable(nodes, func(i, j int) bool {
		return bytes.Compare(nodes[i][:], nodes[j][:]) > 0
	})
	return nodes
}

func indexGenerator(seed types.Seed, round uint64, step uint16, iteration uint32, maxValue *big.Int) int64 {
	data := rlpHash([]interface{}{
		seed, round, step, iteration,
	})
	var hash = new(big.Int).SetBytes(data[:])
	return new(big.Int).Mod(hash, maxValue).Int64()
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}
