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
	sortedValidators []common.Address

	pools       map[common.Address][]common.Address
	delegations map[common.Address]common.Address

	nodesSet       mapset.Set
	onlineNodesSet mapset.Set
	log            log.Logger
	god            common.Address
	mutex          sync.Mutex
	height         uint64
}

func NewValidatorsCache(identityState *state.IdentityStateDB, godAddress common.Address) *ValidatorsCache {
	return &ValidatorsCache{
		identityState:  identityState,
		nodesSet:       mapset.NewSet(),
		onlineNodesSet: mapset.NewSet(),
		log:            log.New(),
		god:            godAddress,
		pools:          map[common.Address][]common.Address{},
		delegations:    map[common.Address]common.Address{},
	}
}

func (v *ValidatorsCache) Load() {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	v.loadValidNodes()
}

func (v *ValidatorsCache) replaceByDelegatee(set mapset.Set) (netSet mapset.Set, votePowers map[common.Address]int) {
	mapped := mapset.NewSet()
	votePowers = make(map[common.Address]int)
	for _, item := range set.ToSlice() {
		addr := item.(common.Address)
		if d, ok := v.delegations[addr]; ok {
			mapped.Add(d)
			votePowers[d] ++
		} else {
			mapped.Add(addr)
		}
	}
	// increment votePowers for pools which address is in original set, those pools are approved identities
	for addr := range votePowers {
		if set.Contains(addr) {
			votePowers[addr] ++
		}
	}
	return mapped, votePowers
}

func (v *ValidatorsCache) GetOnlineValidators(seed types.Seed, round uint64, step uint8, limit int) *StepValidators {

	set := mapset.NewSet()
	if v.OnlineSize() == 0 {
		set.Add(v.god)
		return &StepValidators{Original: set, Addresses: set, Size: 1}
	}
	if len(v.sortedValidators) == limit {
		for _, n := range v.sortedValidators {
			set.Add(n)
		}
		newSet, votePowers := v.replaceByDelegatee(set)
		return &StepValidators{Original: set, Addresses: newSet, ExtraVotePowers: votePowers, Size: newSet.Cardinality()}
	}

	if len(v.sortedValidators) < limit {
		return nil
	}

	rndSeed := crypto.Hash([]byte(fmt.Sprintf("%v-%v-%v", common.Bytes2Hex(seed[:]), round, step)))
	randSeed := binary.LittleEndian.Uint64(rndSeed[:])
	random := rand.New(rand.NewSource(int64(randSeed)))

	indexes := random.Perm(len(v.sortedValidators))

	for i := 0; i < limit; i++ {
		set.Add(v.sortedValidators[indexes[i]])
	}
	newSet, votePowers := v.replaceByDelegatee(set)
	return &StepValidators{Original: set, Addresses: newSet, ExtraVotePowers: votePowers, Size: newSet.Cardinality()}
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

	var delegators []common.Address

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
		if data.Delegatee != nil {
			list, ok := v.pools[*data.Delegatee]
			if !ok {
				list = []common.Address{}
				v.pools[*data.Delegatee] = list
			}
			addToSortList(list, addr)
			delegators = append(delegators, addr)
			v.delegations[addr] = *data.Delegatee
		}
		if data.Approved {
			v.nodesSet.Add(addr)
		}

		return false
	})

	var validators []common.Address
	for _, n := range onlineNodes {
		if v.nodesSet.Contains(n) {
			validators = append(validators, n)
		}
		if delegators, ok := v.pools[n]; ok {
			validators = append(validators, delegators...)
		}
	}

	v.sortedValidators = sortValidNodes(validators)
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
		sortedValidators: append(v.sortedValidators[:0:0], v.sortedValidators...),
		nodesSet:         v.nodesSet.Clone(),
		onlineNodesSet:   v.onlineNodesSet.Clone(),
	}
}

func (v *ValidatorsCache) Height() uint64 {
	return v.height
}

func (v *ValidatorsCache) PoolSize(pool common.Address) int {
	if set, ok := v.pools[pool]; ok {
		size := len(set)
		if v.nodesSet.Contains(pool) {
			size++
		}
		return size
	}
	return 1
}

func (v *ValidatorsCache) IsPool(pool common.Address) bool {
	_, ok := v.pools[pool]
	return ok
}

func (v *ValidatorsCache) FindSubIdentity(pool common.Address, nonce uint32) (common.Address, uint32) {
	set := v.pools[pool]
	if v.nodesSet.Contains(pool) {
		set = append([]common.Address{pool}, set...)
	}
	if nonce >= uint32(len(set)) {
		nonce = 0
	}
	return set[nonce], nonce + 1
}

func (v *ValidatorsCache) Delegator(addr common.Address) common.Address {
	return v.delegations[addr]
}

func sortValidNodes(nodes []common.Address) []common.Address {
	sort.SliceStable(nodes, func(i, j int) bool {
		return bytes.Compare(nodes[i][:], nodes[j][:]) > 0
	})
	return nodes
}

func addToSortList(list []common.Address, e common.Address) {
	i := sort.Search(len(list), func(i int) bool {
		return bytes.Compare(list[i].Bytes(), e.Bytes()) > 0
	})
	list = append(list, common.Address{})
	copy(list[i+1:], list[i:])
	list[i] = e
}

type StepValidators struct {
	Original        mapset.Set
	Addresses       mapset.Set
	Size            int
	ExtraVotePowers map[common.Address]int
}

func (sv *StepValidators) Contains(addr common.Address) bool {
	return sv.Addresses.Contains(addr)
}

func (sv *StepValidators) VotesCountSubtrahend() int {
	sum := 0
	for _, v := range sv.ExtraVotePowers {
		sum += v
	}
	return sum - len(sv.ExtraVotePowers)
}
