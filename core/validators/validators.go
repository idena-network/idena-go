package validators

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"idena-go/blockchain"
	"idena-go/common"
	"idena-go/crypto/sha3"
	"idena-go/idenadb"
	"idena-go/rlp"
	"math/big"
	"sort"
)

type ValidatorsSet struct {
	db         *Validatorsdb
	validNodes ValidNodes
}

type iterationData struct {
	Seed      blockchain.Seed
	Round     uint64
	Step      uint16
	Iteration uint32
}

func NewValidatorsSet(db idenadb.Database) *ValidatorsSet {
	validatorsDb := NewValidatorsDb(db)
	return &ValidatorsSet{
		db:         validatorsDb,
		validNodes: sortValidNodes(validatorsDb.LoadValidNodes()),
	}
}

func (v *ValidatorsSet) AddValidPubKey(pubKey PubKey) {
	v.validNodes = sortValidNodes(append(v.validNodes, pubKey))
	v.db.WriteValidNodes(v.validNodes)
}

func sortValidNodes(nodes ValidNodes) ValidNodes {
	sort.SliceStable(nodes, func(i, j int) bool {
		return bytes.Compare(nodes[i], nodes[j]) > 0
	})
	return nodes
}

func (v *ValidatorsSet) GetActualValidators(seed blockchain.Seed, round uint64, step uint16, limit int) mapset.Set {
	set := mapset.NewSet()
	cnt := new(big.Int).SetInt64(int64(len(v.validNodes)))
	for i := uint32(0); i < uint32(limit*3) && set.Cardinality() < limit; i++ {
		set.Add(v.validNodes[indexGenerator(seed, round, step, i, cnt)])
	}
	if set.Cardinality() < limit {
		return nil
	}
	return set
}

func (v *ValidatorsSet) GetCountOfValidNodes() int {
	return len(v.validNodes)
}

func indexGenerator(seed blockchain.Seed, round uint64, step uint16, iteration uint32, maxValue *big.Int) int64 {
	data := rlpHash(&iterationData{
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
