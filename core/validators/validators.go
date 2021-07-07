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
	math2 "math"
	"math/rand"
	"sort"
	"sync"
)

type ValidatorsCache struct {
	identityState    *state.IdentityStateDB
	sortedValidators *sortedAddresses

	pools       map[common.Address]*sortedAddresses
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
		pools:          map[common.Address]*sortedAddresses{},
		delegations:    map[common.Address]common.Address{},
	}
}

func (v *ValidatorsCache) Load() {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	v.loadValidNodes()
}

func (v *ValidatorsCache) replaceByDelegatee(set mapset.Set) (netSet mapset.Set) {
	mapped := mapset.NewSet()
	for _, item := range set.ToSlice() {
		addr := item.(common.Address)
		if d, ok := v.delegations[addr]; ok {
			mapped.Add(d)
		} else {
			mapped.Add(addr)
		}
	}
	return mapped
}

func (v *ValidatorsCache) GetOnlineValidators(seed types.Seed, round uint64, step uint8, limit int) *StepValidators {

	set := mapset.NewSet()
	if v.OnlineSize() == 0 {
		set.Add(v.god)
		return &StepValidators{Original: set, Addresses: set, Size: 1}
	}
	if len(v.sortedValidators.list) == limit {
		for _, n := range v.sortedValidators.list {
			set.Add(n)
		}
		newSet := v.replaceByDelegatee(set)
		return &StepValidators{Original: set, Addresses: newSet, Size: newSet.Cardinality()}
	}

	if len(v.sortedValidators.list) < limit {
		return nil
	}

	rndSeed := crypto.Hash([]byte(fmt.Sprintf("%v-%v-%v", common.Bytes2Hex(seed[:]), round, step)))
	randSeed := binary.LittleEndian.Uint64(rndSeed[:])
	random := rand.New(rand.NewSource(int64(randSeed)))

	indexes := random.Perm(len(v.sortedValidators.list))

	for i := 0; i < limit; i++ {
		set.Add(v.sortedValidators.list[indexes[i]])
	}
	newSet := v.replaceByDelegatee(set)
	return &StepValidators{Original: set, Addresses: newSet, Size: newSet.Cardinality()}
}

func (v *ValidatorsCache) NetworkSize() int {
	return v.nodesSet.Cardinality()
}

func (v *ValidatorsCache) ForkCommitteeSize() int {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	size := v.NetworkSize() - len(v.delegations)
	for pool, _ := range v.pools {
		if !v.nodesSet.Contains(pool) {
			size++
		}
	}
	return size
}

func (v *ValidatorsCache) OnlineSize() int {
	return v.onlineNodesSet.Cardinality()
}

func (v *ValidatorsCache) ValidatorsSize() int {
	return len(v.sortedValidators.list)
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
	v.delegations = map[common.Address]common.Address{}
	v.pools = map[common.Address]*sortedAddresses{}

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
				list = &sortedAddresses{list: []common.Address{}}
				v.pools[*data.Delegatee] = list
			}
			list.add(addr)
			v.delegations[addr] = *data.Delegatee
		}
		if data.Approved {
			v.nodesSet.Add(addr)
		}

		return false
	})

	v.sortedValidators = &sortedAddresses{descending: true, list: []common.Address{}}

	for _, n := range onlineNodes {
		if v.nodesSet.Contains(n) {
			v.sortedValidators.add(n)
		}
		if delegators, ok := v.pools[n]; ok {
			for _, addr := range delegators.list {
				v.sortedValidators.add(addr)
			}
		}
	}

	v.height = v.identityState.Version()
}

func (v *ValidatorsCache) UpdateFromIdentityStateDiff(diff *state.IdentityStateDiff) {
	v.mutex.Lock()
	defer v.mutex.Unlock()
	v.height = v.identityState.Version()
	for _, d := range diff.Values {

		var data state.ApprovedIdentity
		if err := data.FromBytes(d.Value); err != nil {
			continue
		}
		removeDelegation := func() {
			delegatee, ok := v.delegations[d.Address]
			if ok {
				delete(v.delegations, d.Address)
				v.pools[delegatee].remove(d.Address)
			}
		}

		if d.Deleted {
			v.onlineNodesSet.Remove(d.Address)
			v.nodesSet.Remove(d.Address)
			removeDelegation()
			continue
		}

		if data.Delegatee == nil {
			removeDelegation()
		} else {
			delegator, ok := v.delegations[d.Address]
			if !ok || delegator != *data.Delegatee {
				if ok {
					removeDelegation()
				}
				v.delegations[d.Address] = *data.Delegatee
				list, ok := v.pools[*data.Delegatee]
				if !ok {
					list = &sortedAddresses{list: []common.Address{}}
					v.pools[*data.Delegatee] = list
				}
				list.add(d.Address)
			}
		}

		if data.Online {
			v.onlineNodesSet.Add(d.Address)
		} else {
			v.onlineNodesSet.Remove(d.Address)
		}

		if data.Approved {
			v.nodesSet.Add(d.Address)
		} else {
			v.nodesSet.Remove(d.Address)
			if data.Delegatee != nil {
				removeDelegation()
			}
		}
	}

	v.sortedValidators = &sortedAddresses{descending: true, list: []common.Address{}}

	for _, n := range v.onlineNodesSet.ToSlice() {
		addr := n.(common.Address)
		if v.nodesSet.Contains(addr) {
			v.sortedValidators.add(addr)
		}
		if delegators, ok := v.pools[addr]; ok {
			for _, delegator := range delegators.list {
				v.sortedValidators.add(delegator)
			}
		}
	}
}

func (v *ValidatorsCache) Clone() *ValidatorsCache {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	return &ValidatorsCache{
		height:           v.height,
		identityState:    v.identityState,
		god:              v.god,
		log:              v.log,
		sortedValidators: &sortedAddresses{descending: true, list: append(v.sortedValidators.list[:0:0], v.sortedValidators.list...)},
		nodesSet:         v.nodesSet.Clone(),
		onlineNodesSet:   v.onlineNodesSet.Clone(),
		pools:            clonePools(v.pools),
		delegations:      cloneDelegations(v.delegations),
	}
}

func (v *ValidatorsCache) Height() uint64 {
	return v.height
}

func (v *ValidatorsCache) PoolSize(pool common.Address) int {
	if set, ok := v.pools[pool]; ok {
		size := len(set.list)
		if v.nodesSet.Contains(pool) {
			size++
		}
		return size
	}
	return 0
}

func (v *ValidatorsCache) PoolSizeExceptNodes(pool common.Address, exceptNodes []common.Address) int {
	if set, ok := v.pools[pool]; ok {
		size := len(set.list)
		if v.nodesSet.Contains(pool) {
			size++
		}
		for _, exceptNode := range exceptNodes {
			if set.index(exceptNode) > 0 {
				size--
			}
		}
		return size
	}
	return 0
}

func (v *ValidatorsCache) IsPool(pool common.Address) bool {
	_, ok := v.pools[pool]
	return ok
}

func (v *ValidatorsCache) FindSubIdentity(pool common.Address, nonce uint32) (common.Address, uint32) {
	set := v.pools[pool].list
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

func clonePools(source map[common.Address]*sortedAddresses) map[common.Address]*sortedAddresses {
	result := map[common.Address]*sortedAddresses{}
	for k, v := range source {
		result[k] = v
	}
	return result
}

func cloneDelegations(source map[common.Address]common.Address) map[common.Address]common.Address {
	result := map[common.Address]common.Address{}
	for k, v := range source {
		result[k] = v
	}
	return result
}

type StepValidators struct {
	Original  mapset.Set
	Addresses mapset.Set
	Size      int
}

func (sv *StepValidators) Contains(addr common.Address) bool {
	return sv.Addresses.Contains(addr)
}

func (sv *StepValidators) VotesCountSubtrahend(agreementThreshold float64) int {
	v := sv.Original.Cardinality() - sv.Size
	return int(math2.Round(float64(v) * agreementThreshold))
}

type sortedAddresses struct {
	descending bool
	list       []common.Address
}

func (s *sortedAddresses) add(addr common.Address) {
	i := sort.Search(len(s.list), func(i int) bool {
		if !s.descending {
			return bytes.Compare(s.list[i].Bytes(), addr.Bytes()) >= 0
		} else {
			return bytes.Compare(s.list[i].Bytes(), addr.Bytes()) <= 0
		}
	})
	if i < len(s.list) && bytes.Compare(s.list[i].Bytes(), addr.Bytes()) == 0 {
		return
	}

	s.list = append(s.list, common.Address{})
	copy(s.list[i+1:], s.list[i:])
	s.list[i] = addr
}

func (s *sortedAddresses) remove(addr common.Address) {
	i := s.index(addr)
	if i >= 0 {
		copy(s.list[i:], s.list[i+1:])
		s.list[len(s.list)-1] = common.Address{}
		s.list = s.list[:len(s.list)-1]
	}
}

func (s *sortedAddresses) index(addr common.Address) int {

	i := sort.Search(len(s.list), func(i int) bool {
		if !s.descending {
			return bytes.Compare(s.list[i].Bytes(), addr.Bytes()) >= 0
		} else {
			return bytes.Compare(s.list[i].Bytes(), addr.Bytes()) <= 0
		}
	})
	if i == len(s.list) {
		return -1
	}
	if bytes.Compare(s.list[i].Bytes(), addr.Bytes()) == 0 {
		return i
	}
	return -1
}
