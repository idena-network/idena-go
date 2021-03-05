package validators

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/log"
	"math/rand"
	"sort"
	"sync"
)

type ValidatorsCache struct {
	identityState    *state.IdentityStateDB
	validOnlineNodes []common.Address
	nodesSet         mapset.Set
	onlineNodesSet   mapset.Set
	log              log.Logger
	god              common.Address
	mutex            sync.Mutex
	height           uint64
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
	v.mutex.Lock()
	defer v.mutex.Unlock()

	v.loadValidNodes()
}

func (v *ValidatorsCache) GetOnlineValidators(seed types.Seed, round uint64, step uint8, limit int) mapset.Set {

	set := mapset.NewSet()
	if v.OnlineSize() == 0 {
		set.Add(v.god)
		return set
	}
	if len(v.validOnlineNodes) == limit {
		for _, n := range v.validOnlineNodes {
			set.Add(n)
		}
		return set
	}

	if len(v.validOnlineNodes) < limit {
		return nil
	}

	rndSeed := crypto.Hash([]byte(fmt.Sprintf("%v-%v-%v", common.Bytes2Hex(seed[:]), round, step)))
	randSeed := binary.LittleEndian.Uint64(rndSeed[:])
	random := rand.New(rand.NewSource(int64(randSeed)))

	indexes := random.Perm(len(v.validOnlineNodes))

	for i := 0; i < limit; i++ {
		set.Add(v.validOnlineNodes[indexes[i]])
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
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if block.Header.Flags().HasFlag(types.IdentityUpdate) {
		v.loadValidNodes()
		v.log.Info("Validators updated", "total", v.nodesSet.Cardinality(), "online", v.onlineNodesSet.Cardinality())
	}
	v.god = godAddress
	v.height = block.Height()
}

func (v *ValidatorsCache) loadValidNodes() {

	var onlineNodes []common.Address
	v.nodesSet.Clear()
	v.onlineNodesSet.Clear()

	v.identityState.IterateIdentities(func(key []byte, value []byte) bool {
		if key == nil {
			return true
		}
		addr := common.Address{}
		addr.SetBytes(key[1:])

		var data state.ApprovedIdentity
		if err := data.FromBytes(value); err != nil {
			return false
		}

		if data.Online {
			v.onlineNodesSet.Add(addr)
			onlineNodes = append(onlineNodes, addr)
		}

		v.nodesSet.Add(addr)

		return false
	})

	v.validOnlineNodes = sortValidNodes(onlineNodes)
	v.height = v.identityState.Version()
}

func (v *ValidatorsCache) Clone() *ValidatorsCache {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	return &ValidatorsCache{
		height:           v.height,
		identityState:    v.identityState,
		god:              v.god,
		log:              v.log,
		validOnlineNodes: append(v.validOnlineNodes[:0:0], v.validOnlineNodes...),
		nodesSet:         v.nodesSet.Clone(),
		onlineNodesSet:   v.onlineNodesSet.Clone(),
	}
}

func (v *ValidatorsCache) Height() uint64 {
	return v.height
}

func (v *ValidatorsCache) PoolSize(pool common.Address) int {
	return 0
}

func sortValidNodes(nodes []common.Address) []common.Address {
	sort.SliceStable(nodes, func(i, j int) bool {
		return bytes.Compare(nodes[i][:], nodes[j][:]) > 0
	})
	return nodes
}
