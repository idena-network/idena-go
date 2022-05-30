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

	pools       map[common.Address]*pool
	delegations map[common.Address]common.Address

	validatedAddresses     mapset.Set
	onlineAddresses        mapset.Set
	discriminatedAddresses mapset.Set

	forkCommitteeSizeCache *int

	log    log.Logger
	god    common.Address
	mutex  sync.Mutex
	height uint64
}

func NewValidatorsCache(identityState *state.IdentityStateDB, godAddress common.Address) *ValidatorsCache {
	return &ValidatorsCache{
		identityState:          identityState,
		validatedAddresses:     mapset.NewSet(),
		onlineAddresses:        mapset.NewSet(),
		log:                    log.New(),
		god:                    godAddress,
		pools:                  map[common.Address]*pool{},
		delegations:            map[common.Address]common.Address{},
		discriminatedAddresses: mapset.NewSet(),
	}
}

func (v *ValidatorsCache) Load() {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	v.loadValidNodes()
}

func (v *ValidatorsCache) determineValidators(set mapset.Set) (validators, approvedValidators mapset.Set) {
	validators, approvedValidators = mapset.NewSet(), mapset.NewSet()
	for _, item := range set.ToSlice() {
		addr := item.(common.Address)
		if delegatee, ok := v.delegations[addr]; ok {
			validators.Add(delegatee)
			if pool, ok := v.pools[delegatee]; ok {
				if !pool.discriminated() {
					approvedValidators.Add(delegatee)
				}
			} else {
				v.log.Warn("Pool not found", "delegator", addr.Hex(), "delegatee", delegatee.Hex())
			}
		} else {
			validators.Add(addr)
			if !v.discriminatedAddresses.Contains(addr) {
				approvedValidators.Add(addr)
			}
		}
	}
	return validators, approvedValidators
}

func (v *ValidatorsCache) GetOnlineValidators(seed types.Seed, round uint64, step uint8, limit int) *StepValidators {

	set := mapset.NewSet()
	if v.OnlineSize() == 0 {
		set.Add(v.god)
		return &StepValidators{Original: set, Validators: set, ApprovedValidators: set}
	}
	if len(v.sortedValidators.list) == limit {
		for _, n := range v.sortedValidators.list {
			set.Add(n)
		}
		validators, approvedValidators := v.determineValidators(set)
		return &StepValidators{Original: set, Validators: validators, ApprovedValidators: approvedValidators}
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
	validators, approvedValidators := v.determineValidators(set)
	return &StepValidators{Original: set, Validators: validators, ApprovedValidators: approvedValidators}
}

func (v *ValidatorsCache) NetworkSize() int {
	return v.validatedAddresses.Cardinality()
}

func (v *ValidatorsCache) ForkCommitteeSize() int {
	if fk := v.forkCommitteeSizeCache; fk != nil {
		return *fk
	}
	v.mutex.Lock()
	defer v.mutex.Unlock()
	if fk := v.forkCommitteeSizeCache; fk != nil {
		return *fk
	}
	fk := v.buildForkCommittee()
	v.forkCommitteeSizeCache = &fk
	return fk
}

func (v *ValidatorsCache) buildForkCommittee() int {
	var res int

	v.onlineAddresses.Each(func(value interface{}) bool {
		addr := value.(common.Address)
		pool, isPool := v.pools[addr]
		if isPool {
			if !pool.discriminated() {
				res++
			}
			return false
		}
		if !v.discriminatedAddresses.Contains(addr) {
			res++
		}
		return false
	})

	return res
}

func (v *ValidatorsCache) OnlineSize() int {
	return v.onlineAddresses.Cardinality()
}

func (v *ValidatorsCache) ValidatorsSize() int {
	return len(v.sortedValidators.list)
}

func (v *ValidatorsCache) IsValidated(addr common.Address) bool {
	return v.validatedAddresses.Contains(addr)
}

func (v *ValidatorsCache) IsOnlineIdentity(addr common.Address) bool {
	return v.onlineAddresses.Contains(addr)
}

func (v *ValidatorsCache) IsDiscriminated(addr common.Address) bool {
	if p, ok := v.pools[addr]; ok {
		return p.discriminated()
	}
	return v.discriminatedAddresses.Contains(addr)
}

func (v *ValidatorsCache) GetAllOnlineValidators() mapset.Set {
	return v.onlineAddresses.Clone()
}

func (v *ValidatorsCache) RefreshIfUpdated(godAddress common.Address, block *types.Block, diff *state.IdentityStateDiff) {
	if block.Header.Flags().HasFlag(types.IdentityUpdate) {
		v.UpdateFromIdentityStateDiff(diff)
		v.log.Info("Validators updated", "validated", v.validatedAddresses.Cardinality(), "online", v.onlineAddresses.Cardinality(), "discriminated", v.discriminatedAddresses.Cardinality())
	}
	v.god = godAddress
	v.height = block.Height()
}

func (v *ValidatorsCache) loadValidNodes() {

	var onlineNodes []common.Address
	v.validatedAddresses.Clear()
	v.onlineAddresses.Clear()
	v.delegations = map[common.Address]common.Address{}
	v.pools = map[common.Address]*pool{}
	v.discriminatedAddresses.Clear()
	v.forkCommitteeSizeCache = nil

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
			v.onlineAddresses.Add(addr)
			onlineNodes = append(onlineNodes, addr)
		}
		if data.Delegatee != nil {
			pool, ok := v.pools[*data.Delegatee]
			if !ok {
				poolApproved := v.validatedAddresses.Contains(*data.Delegatee) && !v.discriminatedAddresses.Contains(*data.Delegatee)
				pool = newPool(*data.Delegatee, poolApproved)
				v.pools[*data.Delegatee] = pool
			}
			approved := data.Validated && !data.Discriminated
			pool.add(addr, approved)
			v.delegations[addr] = *data.Delegatee
		}
		if data.Validated {
			v.validatedAddresses.Add(addr)
		}
		if data.Discriminated {
			v.discriminatedAddresses.Add(addr)
		}
		if pool, ok := v.pools[addr]; ok {
			pool.setApproved(addr, data.Validated && !data.Discriminated)
		}

		return false
	})

	v.sortedValidators = newSortedAddresses()

	for _, n := range onlineNodes {
		if v.validatedAddresses.Contains(n) {
			v.sortedValidators.add(n)
		}
		if pool, ok := v.pools[n]; ok {
			for _, addr := range pool.delegators {
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
	v.forkCommitteeSizeCache = nil

	newApprovals := map[common.Address]bool{}
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
				if len(v.pools[delegatee].delegators) == 0 {
					delete(v.pools, delegatee)
				}
			}
		}

		if d.Deleted {
			v.onlineAddresses.Remove(d.Address)
			v.validatedAddresses.Remove(d.Address)
			removeDelegation()
			v.discriminatedAddresses.Remove(d.Address)
			newApprovals[d.Address] = false
			continue
		}

		if data.Delegatee == nil {
			removeDelegation()
		} else {
			delegator, ok := v.delegations[d.Address]
			approved := data.Validated && !data.Discriminated
			if !ok || delegator != *data.Delegatee {
				if ok {
					removeDelegation()
				}
				v.delegations[d.Address] = *data.Delegatee
				pool, ok := v.pools[*data.Delegatee]
				if !ok {
					var poolApproved bool
					if approved, vOk := newApprovals[*data.Delegatee]; vOk {
						poolApproved = approved
					} else {
						poolApproved = v.validatedAddresses.Contains(*data.Delegatee) && !v.discriminatedAddresses.Contains(*data.Delegatee)
					}
					pool = newPool(*data.Delegatee, poolApproved)
					v.pools[*data.Delegatee] = pool
				}
				pool.add(d.Address, approved)
			} else {
				v.pools[*data.Delegatee].setApproved(d.Address, approved)
			}
		}

		if data.Online {
			v.onlineAddresses.Add(d.Address)
		} else {
			v.onlineAddresses.Remove(d.Address)
		}

		if data.Validated {
			v.validatedAddresses.Add(d.Address)
		} else {
			v.validatedAddresses.Remove(d.Address)
			if data.Delegatee != nil {
				removeDelegation()
			}
		}

		if data.Discriminated {
			v.discriminatedAddresses.Add(d.Address)
		} else {
			v.discriminatedAddresses.Remove(d.Address)
		}

		newApprovals[d.Address] = data.Validated && !data.Discriminated
	}

	for address, approved := range newApprovals {
		if pool, ok := v.pools[address]; ok {
			pool.setApproved(address, approved)
		}
	}

	v.sortedValidators = newSortedAddresses()

	for _, n := range v.onlineAddresses.ToSlice() {
		addr := n.(common.Address)
		if v.validatedAddresses.Contains(addr) {
			v.sortedValidators.add(addr)
		}
		if pool, ok := v.pools[addr]; ok {
			for _, delegator := range pool.delegators {
				v.sortedValidators.add(delegator)
			}
		}
	}
}

func (v *ValidatorsCache) Clone() *ValidatorsCache {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	return &ValidatorsCache{
		height:                 v.height,
		identityState:          v.identityState,
		god:                    v.god,
		log:                    v.log,
		sortedValidators:       cloneSortedAddresses(v.sortedValidators),
		validatedAddresses:     v.validatedAddresses.Clone(),
		onlineAddresses:        v.onlineAddresses.Clone(),
		pools:                  clonePools(v.pools),
		delegations:            cloneDelegations(v.delegations),
		discriminatedAddresses: v.discriminatedAddresses.Clone(),
	}
}

func (v *ValidatorsCache) Height() uint64 {
	return v.height
}

func (v *ValidatorsCache) PoolSize(pool common.Address) int {
	if set, ok := v.pools[pool]; ok {
		size := len(set.delegators)
		if v.validatedAddresses.Contains(pool) {
			size++
		}
		return size
	}
	return 0
}

func (v *ValidatorsCache) PoolSizeExceptNodes(pool common.Address, exceptNodes []common.Address, enableUpgrade8 bool) int {
	if set, ok := v.pools[pool]; ok {
		size := len(set.delegators)
		if v.validatedAddresses.Contains(pool) {
			size++
		}
		for _, exceptNode := range exceptNodes {
			if index := set.index(exceptNode); index > 0 || enableUpgrade8 && index == 0 {
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
	set := v.pools[pool].delegators
	if v.validatedAddresses.Contains(pool) {
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

func cloneSortedAddresses(src *sortedAddresses) *sortedAddresses {
	return &sortedAddresses{list: append(src.list[:0:0], src.list...)}
}

func clonePools(source map[common.Address]*pool) map[common.Address]*pool {
	result := make(map[common.Address]*pool, len(source))
	for k, v := range source {
		result[k] = &pool{delegators: append(v.delegators[:0:0], v.delegators...), approved: v.approved.Clone()}
	}
	return result
}

func cloneDelegations(source map[common.Address]common.Address) map[common.Address]common.Address {
	result := make(map[common.Address]common.Address, len(source))
	for k, v := range source {
		result[k] = v
	}
	return result
}

type StepValidators struct {
	Original           mapset.Set
	Validators         mapset.Set
	ApprovedValidators mapset.Set
}

func (sv *StepValidators) CanVote(addr common.Address) bool {
	return sv.Validators.Contains(addr)
}

func (sv *StepValidators) Approved(addr common.Address) bool {
	return sv.ApprovedValidators.Contains(addr)
}

func (sv *StepValidators) VotesCountSubtrahend(agreementThreshold float64) int {
	v := sv.Original.Cardinality() - sv.ApprovedValidators.Cardinality()
	return int(math2.Round(float64(v) * agreementThreshold))
}

type sortedAddresses struct {
	list []common.Address
}

func newSortedAddresses() *sortedAddresses {
	return &sortedAddresses{
		list: []common.Address{},
	}
}

func (s *sortedAddresses) add(addr common.Address) {
	i := sort.Search(len(s.list), func(i int) bool {
		return bytes.Compare(s.list[i].Bytes(), addr.Bytes()) <= 0
	})
	if i < len(s.list) && bytes.Compare(s.list[i].Bytes(), addr.Bytes()) == 0 {
		return
	}

	s.list = append(s.list, common.Address{})
	copy(s.list[i+1:], s.list[i:])
	s.list[i] = addr
}

type pool struct {
	delegators []common.Address
	approved   mapset.Set
}

func newPool(address common.Address, approved bool) *pool {
	res := &pool{
		delegators: []common.Address{},
		approved:   mapset.NewSet(),
	}
	if approved {
		res.approved.Add(address)
	}
	return res
}

func (p *pool) discriminated() bool {
	return p.approved.Cardinality() == 0
}

func (p *pool) add(addr common.Address, approved bool) {
	i := sort.Search(len(p.delegators), func(i int) bool {
		return bytes.Compare(p.delegators[i].Bytes(), addr.Bytes()) >= 0
	})
	if i < len(p.delegators) && bytes.Compare(p.delegators[i].Bytes(), addr.Bytes()) == 0 {
		return
	}
	p.delegators = append(p.delegators, common.Address{})
	copy(p.delegators[i+1:], p.delegators[i:])
	p.delegators[i] = addr
	if approved {
		p.approved.Add(addr)
	}
}

func (p *pool) remove(addr common.Address) {
	p.approved.Remove(addr)
	i := p.index(addr)
	if i >= 0 {
		copy(p.delegators[i:], p.delegators[i+1:])
		p.delegators[len(p.delegators)-1] = common.Address{}
		p.delegators = p.delegators[:len(p.delegators)-1]
	}
}

func (p *pool) index(addr common.Address) int {
	i := sort.Search(len(p.delegators), func(i int) bool {
		return bytes.Compare(p.delegators[i].Bytes(), addr.Bytes()) >= 0
	})
	if i == len(p.delegators) {
		return -1
	}
	v := p.delegators[i]
	if bytes.Compare(v.Bytes(), addr.Bytes()) == 0 {
		return i
	}
	return -1
}

func (p *pool) setApproved(addr common.Address, approved bool) {
	if approved {
		p.approved.Add(addr)
	} else {
		p.approved.Remove(addr)
	}
}
