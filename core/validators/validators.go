package validators

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/crypto"
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

func NewValidatorsSet(db idenadb.Database) *ValidatorsSet {
	validatorsDb := NewValidatorsDb(db)
	return &ValidatorsSet{
		db:         validatorsDb,
		validNodes: sortValidNodes(validatorsDb.LoadValidNodes()),
	}
}

func (v *ValidatorsSet) AddValidPubKey(pubKey []byte) error {
	addr, err := crypto.PubKeyBytesToAddress(pubKey)
	if err != nil {
		return err
	}

	v.validNodes = sortValidNodes(append(v.validNodes, addr))
	v.db.WriteValidNodes(v.validNodes)
	return nil
}

func sortValidNodes(nodes ValidNodes) ValidNodes {
	sort.SliceStable(nodes, func(i, j int) bool {
		return bytes.Compare(nodes[i][:], nodes[j][:]) > 0
	})
	return nodes
}

func (v *ValidatorsSet) GetActualValidators(seed types.Seed, round uint64, step uint16, limit int) mapset.Set {
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
func (v *ValidatorsSet) Contains(pubKey []byte) bool {

	addr, err := crypto.PubKeyBytesToAddress(pubKey)
	if err != nil {
		return false
	}
	for _, p := range v.validNodes {
		if p == addr {
			return true
		}
	}
	return false
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
