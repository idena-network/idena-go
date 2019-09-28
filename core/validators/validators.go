package validators

import (
	"bytes"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"math/big"
	"sort"
)

type ValidatorsCache struct {
	identityState    *state.IdentityStateDB
	validOnlineNodes []*common.Address
	nodesSet         mapset.Set
	onlineNodesSet   mapset.Set
	log              log.Logger
	god              common.Address
}

func NewValidatorsCache(identityState *state.IdentityStateDB, godAddress common.Address) *ValidatorsCache {
	return &ValidatorsCache{
		identityState:  identityState,
		nodesSet:       mapset.NewSet(),
		onlineNodesSet: mapset.NewSet(),
		log:            log.New(),
		god:            godAddress,
	}
}

func (v *ValidatorsCache) Load() {
	v.nodesSet.Clear()
	v.onlineNodesSet.Clear()
	v.loadValidNodes()
}

func (v *ValidatorsCache) GetOnlineValidators(seed types.Seed, round uint64, step uint16, limit int) mapset.Set {
	set := mapset.NewSet()
	if v.OnlineSize() == 0 {
		set.Add(v.god)
		return set
	}
	cnt := new(big.Int).SetInt64(int64(len(v.validOnlineNodes)))
	for i := uint32(0); i < uint32(limit*3) && set.Cardinality() < limit; i++ {
		set.Add(*v.validOnlineNodes[indexGenerator(seed, round, step, i, cnt)])
	}
	if set.Cardinality() < limit {
		return nil
	}
	return set
}

func (v *ValidatorsCache) NetworkSize() int {
	return v.nodesSet.Cardinality()
}

func (v *ValidatorsCache) OnlineSize() int {
	return v.onlineNodesSet.Cardinality()
}

func (v *ValidatorsCache) Contains(addr common.Address) bool {
	return v.nodesSet.Contains(addr)
}

func (v *ValidatorsCache) IsOnlineIdentity(addr common.Address) bool {
	return v.onlineNodesSet.Contains(addr)
}

func (v *ValidatorsCache) GetAllOnlineValidators() mapset.Set {
	return v.onlineNodesSet.Clone()
}

func (v *ValidatorsCache) RefreshIfUpdated(godAddress common.Address, block *types.Block) {
	v.god = godAddress
	shouldUpdate := block.Header.Flags().HasFlag(types.IdentityUpdate)

	if !shouldUpdate {
		for _, tx := range block.Body.Transactions {
			if tx.Type == types.OnlineStatusTx {
				shouldUpdate = true
				break
			}
		}
	}

	if shouldUpdate {
		v.loadValidNodes()
		v.log.Info("Validators updated", "total", v.nodesSet.Cardinality(), "online", v.onlineNodesSet.Cardinality())
	}
}

func (v *ValidatorsCache) loadValidNodes() {
	var onlineNodes []*common.Address
	v.nodesSet.Clear()
	v.onlineNodesSet.Clear()

	v.identityState.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])

		var data state.ApprovedIdentity
		if err := rlp.DecodeBytes(value, &data); err != nil {
			return false
		}

		if data.Online {
			v.onlineNodesSet.Add(addr)
			onlineNodes = append(onlineNodes, &addr)
		}

		v.nodesSet.Add(addr)

		return false
	})

	v.validOnlineNodes = sortValidNodes(onlineNodes)
}

func sortValidNodes(nodes []*common.Address) []*common.Address {
	sort.SliceStable(nodes, func(i, j int) bool {
		return bytes.Compare(nodes[i][:], nodes[j][:]) > 0
	})
	return nodes
}

func indexGenerator(seed types.Seed, round uint64, step uint16, iteration uint32, maxValue *big.Int) int64 {
	data := rlp.Hash([]interface{}{
		seed, round, step, iteration,
	})
	var hash = new(big.Int).SetBytes(data[:])
	return new(big.Int).Mod(hash, maxValue).Int64()
}
