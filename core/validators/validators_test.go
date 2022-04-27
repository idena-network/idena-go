package validators

import (
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/tests"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-db"
	"math/rand"
	"testing"
)

const enableUpgrade8 = true

func TestValidatorsCache_Contains(t *testing.T) {
	require := require.New(t)
	database := db.NewMemDB()
	identityStateDB, _ := state.NewLazyIdentityState(database)

	m := make(map[common.Address]bool)

	countOnline, countAll := 0, 100

	for j := 0; j < countAll; j++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)

		obj := identityStateDB.GetOrNewIdentityObject(addr)
		obj.SetValidated(true)

		isOnline := rand.Int31()%2 == 0
		m[addr] = isOnline

		if isOnline {
			obj.SetOnline(true)
			countOnline++
		}
	}
	identityStateDB.Commit(false, enableUpgrade8)

	vCache := NewValidatorsCache(identityStateDB, common.Address{})
	vCache.Load()

	for addr, online := range m {
		require.Equal(online, vCache.IsOnlineIdentity(addr))
		require.True(vCache.IsValidated(addr))
	}

	require.Equal(countOnline, vCache.OnlineSize())
	require.Equal(countAll, vCache.NetworkSize())
}

func TestValidatorsCache_Load(t *testing.T) {
	require := require.New(t)
	database := db.NewMemDB()
	identityStateDB, _ := state.NewLazyIdentityState(database)

	countAll := 12

	pool1 := common.Address{1}
	pool2 := common.Address{2}
	pool3 := common.Address{3}

	vCacheForUpdate := NewValidatorsCache(identityStateDB, common.Address{0x11})
	vCacheForUpdate.Load()

	for j := 0; j < countAll; j++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)

		obj := identityStateDB.GetOrNewIdentityObject(addr)
		obj.SetValidated(true)
		obj.SetOnline(true)
		if j%3 == 0 { // identities 0,3,6,9
			obj.SetDelegatee(pool1)
			obj.SetOnline(false)
		} else if j%4 == 0 { // identities 4,8
			obj.SetDelegatee(pool2)
			obj.SetOnline(false)
		} else if j%11 == 0 { // identities 11
			obj.SetDelegatee(pool3)
			obj.SetOnline(false)
		}
	}
	identityStateDB.SetOnline(pool1, true)
	identityStateDB.SetOnline(pool2, true)
	identityStateDB.SetValidated(pool2, true)
	identityStateDB.SetOnline(pool3, false)

	_, _, diff, _ := identityStateDB.Commit(false, enableUpgrade8)

	vCache := NewValidatorsCache(identityStateDB, common.Address{0x11})
	vCache.Load()

	vCacheForUpdate.UpdateFromIdentityStateDiff(diff)

	require.Equal(vCache.sortedValidators.list, vCacheForUpdate.sortedValidators.list)

	require.Equal(4, vCache.PoolSize(pool1))
	require.Equal(3, vCache.PoolSize(pool2))
	require.Equal(1, vCache.PoolSize(pool3))
	require.True(vCache.IsPool(pool1))
	require.True(vCache.IsPool(pool2))
	require.True(vCache.IsPool(pool3))
	require.False(vCache.IsPool(common.Address{0x4}))
	require.Len(vCache.delegations, 7)

	require.Len(vCache.sortedValidators.list, 12)

	require.NotContains(vCache.sortedValidators.list, pool1)
	require.Contains(vCache.sortedValidators.list, pool2)
	require.NotContains(vCache.sortedValidators.list, pool3)

	stepValidators := vCache.GetOnlineValidators([32]byte{0x1}, 1, 1, 12)
	require.Equal(12, stepValidators.Original.Cardinality())

	require.True(stepValidators.Validators.Contains(pool1))
	require.True(stepValidators.Validators.Contains(pool2))

	require.False(stepValidators.Original.Contains(pool1))
	require.True(stepValidators.Original.Contains(pool2))

	require.False(stepValidators.Validators.Contains(pool3))
	require.Equal(7, stepValidators.ApprovedValidators.Cardinality())
	require.Equal(5, stepValidators.VotesCountSubtrahend(1))

	identityStateDB.RemoveDelegatee(vCache.pools[pool2].delegators[0])
	identityStateDB.RemoveDelegatee(vCache.pools[pool2].delegators[1])
	identityStateDB.Commit(false, enableUpgrade8)

	vCache.Load()
	require.False(vCache.IsPool(pool2))
}

func test_LoadAndUpdateFromIdentityStateDiff(poolFirst bool, t *testing.T) {
	require := require.New(t)
	var caches []*ValidatorsCache
	var diff *state.IdentityStateDiff
	var singleAddresses []common.Address
	var pools []common.Address
	var delegators [][]common.Address
	database := db.NewMemDB()

	{

		identityStateDB, _ := state.NewLazyIdentityState(database)
		_, _, _, err := identityStateDB.Commit(true, enableUpgrade8)
		require.NoError(err)

		for validated := 0; validated <= 1; validated++ {
			for online := 0; online <= 1; online++ {
				for discriminated := 0; discriminated <= 1; discriminated++ {
					addr := tests.GetRandAddr()
					if validated == 1 {
						identityStateDB.SetValidated(addr, true)
					}
					identityStateDB.SetOnline(addr, online == 1 && validated == 1)
					identityStateDB.SetDiscriminated(addr, discriminated == 1)
					singleAddresses = append(singleAddresses, addr)
				}
			}
		}

		for validated := 0; validated <= 1; validated++ {
			for online := 0; online <= 1; online++ {
				for discriminated := 0; discriminated <= 1; discriminated++ {
					for allDelegatorsDiscriminated := 0; allDelegatorsDiscriminated <= 1; allDelegatorsDiscriminated++ {
						for allDelegatorsNotValidated := 0; allDelegatorsNotValidated <= 1; allDelegatorsNotValidated++ {
							poolAddr := tests.GetRandAddr()
							if poolFirst {
								poolAddr[0] = 0x1
							} else {
								poolAddr[0] = 0x2
							}
							if validated == 1 {
								identityStateDB.SetValidated(poolAddr, true)
							}
							identityStateDB.SetOnline(poolAddr, online == 1 && (validated == 1 || allDelegatorsNotValidated == 0))
							identityStateDB.SetDiscriminated(poolAddr, discriminated == 1)
							delegators = append(delegators, []common.Address{})
							for i := 0; i < 10; i++ {
								delegatorDiscriminated := i < 5 || allDelegatorsDiscriminated == 1
								delegatorNotValidated := i < 5 || allDelegatorsNotValidated == 1
								delegator := tests.GetRandAddr()
								if poolFirst {
									delegator[0] = 0x2
								} else {
									delegator[0] = 0x1
								}
								if !delegatorNotValidated {
									identityStateDB.SetValidated(delegator, true)
								}
								identityStateDB.SetDiscriminated(delegator, delegatorDiscriminated)
								identityStateDB.SetDelegatee(delegator, poolAddr)
								delegators[len(pools)] = append(delegators[len(pools)], delegator)
							}
							pools = append(pools, poolAddr)
						}
					}
				}
			}
		}

		_, _, diff, err = identityStateDB.Commit(true, enableUpgrade8)
		require.NoError(err)

		vCache := NewValidatorsCache(identityStateDB, common.Address{0x1, 0x2, 0x3})
		vCache.Load()
		caches = append(caches, vCache)
	}

	{
		emptyDB, _ := state.NewLazyIdentityState(db.NewMemDB())
		vCache := NewValidatorsCache(emptyDB, common.Address{0x1, 0x2, 0x3})
		vCache.Load()
		vCache.UpdateFromIdentityStateDiff(diff)
		caches = append(caches, vCache)
	}

	for _, vCache := range caches {

		containsInt := func(arr []int, val int) bool {
			for _, v := range arr {
				if v == val {
					return true
				}
			}
			return false
		}

		require.Equal(14, vCache.OnlineSize())
		onlineIdentityIndexes := []int{6, 7}
		for i, addr := range singleAddresses {
			if containsInt(onlineIdentityIndexes, i) {
				require.True(vCache.IsOnlineIdentity(addr), fmt.Sprintf("addr index %v", i))
			} else {
				require.False(vCache.IsOnlineIdentity(addr), fmt.Sprintf("addr index %v", i))
			}
		}
		onlinePoolIndexes := []int{8, 10, 12, 14, 24, 25, 26, 27, 28, 29, 30, 31}
		for i, addr := range pools {
			if containsInt(onlinePoolIndexes, i) {
				require.True(vCache.IsOnlineIdentity(addr), fmt.Sprintf("pool index %v", i))
			} else {
				require.False(vCache.IsOnlineIdentity(addr), fmt.Sprintf("pool index %v", i))
			}
		}

		require.Equal(100, vCache.NetworkSize())

		validatedCommitteeSize, onlineCommitteeSize := vCache.ForkCommitteeSizes()

		require.Equal(16, validatedCommitteeSize)
		require.Equal(8, onlineCommitteeSize)

		require.Equal(52, vCache.discriminatedAddresses.Cardinality())

		discriminatedIdentityIndexes := []int{5, 7}
		for i, addr := range singleAddresses {
			if containsInt(discriminatedIdentityIndexes, i) {
				require.True(vCache.discriminatedAddresses.Contains(addr), fmt.Sprintf("addr index %v", i))
			} else {
				require.False(vCache.discriminatedAddresses.Contains(addr), fmt.Sprintf("addr index %v", i))
			}
		}
		type poolWithDiscriminations struct {
			discriminated, delegatorsDiscriminated bool
		}
		poolsWithDiscriminations := map[int]poolWithDiscriminations{
			2:  {false, true},
			6:  {false, true},
			10: {false, true},
			12: {true, false},
			14: {true, true},
			18: {false, true},
			20: {true, false},
			21: {true, false},
			22: {true, true},
			23: {true, false},
			26: {false, true},
			28: {true, false},
			29: {true, false},
			30: {true, true},
			31: {true, false},
		}
		for i, addr := range pools {
			if p, ok := poolsWithDiscriminations[i]; ok {
				if p.discriminated {
					require.True(vCache.discriminatedAddresses.Contains(addr), fmt.Sprintf("pool index %v", i))
				} else {
					require.False(vCache.discriminatedAddresses.Contains(addr), fmt.Sprintf("pool index %v", i))
				}
				for i2, delegator := range delegators[i] {
					if p.delegatorsDiscriminated && i2 >= 5 {
						require.True(vCache.discriminatedAddresses.Contains(delegator), fmt.Sprintf("pool index %v, delegator index %v", i, i2))
					} else {
						require.False(vCache.discriminatedAddresses.Contains(delegator), fmt.Sprintf("pool index %v, delegator index %v", i, i2))
					}
				}
			} else {
				require.False(vCache.discriminatedAddresses.Contains(addr), fmt.Sprintf("pool index %v", i))
				for i2, delegator := range delegators[i] {
					require.False(vCache.discriminatedAddresses.Contains(delegator), fmt.Sprintf("pool index %v, delegator index %v", i, i2))
				}
			}
		}

		discriminatedIdentityIndexes = []int{5, 7}
		for i, addr := range singleAddresses {
			if containsInt(discriminatedIdentityIndexes, i) {
				require.True(vCache.IsDiscriminated(addr), fmt.Sprintf("addr index %v", i))
			} else {
				require.False(vCache.IsDiscriminated(addr), fmt.Sprintf("addr index %v", i))
			}
		}
		discriminatedPoolIndexes := []int{2, 6, 10, 14, 21, 22, 23, 29, 30, 31}
		for i, addr := range pools {
			if containsInt(discriminatedPoolIndexes, i) {
				require.True(vCache.IsDiscriminated(addr), fmt.Sprintf("pool index %v", i))
			} else {
				require.False(vCache.IsDiscriminated(addr), fmt.Sprintf("pool index %v", i))
			}
		}

		require.Equal(50, vCache.ValidatorsSize())
	}

	{
		emptyDB, _ := state.NewLazyIdentityState(db.NewMemDB())
		vCache := NewValidatorsCache(emptyDB, common.Address{0x1, 0x2, 0x3})
		vCache.Load()
		vCache.UpdateFromIdentityStateDiff(diff)

		identityStateDB, _ := state.NewLazyIdentityState(database)
		require.NoError(identityStateDB.Load(2))
		identityStateDB.Remove(delegators[2][5])
		for i := 5; i < 8; i++ {
			identityStateDB.Remove(delegators[0][i])
			identityStateDB.Remove(delegators[16][i])
		}
		identityStateDB.SetDiscriminated(delegators[0][8], true)
		identityStateDB.SetDiscriminated(delegators[16][8], true)
		identityStateDB.SetDelegatee(singleAddresses[5], singleAddresses[4])
		identityStateDB.SetOnline(singleAddresses[6], false)
		identityStateDB.SetDelegatee(singleAddresses[7], singleAddresses[6])

		_, _, diff2, _ := identityStateDB.Commit(true, enableUpgrade8)

		vCache.UpdateFromIdentityStateDiff(diff2)

		require.True(vCache.IsPool(pools[0]))
		require.False(vCache.IsDiscriminated(pools[0]))
		require.True(vCache.IsPool(pools[16]))
		require.False(vCache.IsDiscriminated(pools[16]))
		require.False(vCache.discriminatedAddresses.Contains(delegators[2][5]))
		require.True(vCache.discriminatedAddresses.Contains(delegators[2][6]))
		require.False(vCache.IsDiscriminated(pools[24]))
		require.False(vCache.IsDiscriminated(pools[26]))
		require.True(vCache.IsPool(singleAddresses[4]))
		require.False(vCache.IsDiscriminated(singleAddresses[4]))
		require.True(vCache.IsPool(singleAddresses[6]))
		require.False(vCache.IsDiscriminated(singleAddresses[6]))
		require.Equal(1, vCache.pools[singleAddresses[6]].approved.Cardinality())
		require.True(vCache.pools[singleAddresses[6]].approved.Contains(singleAddresses[6]))

		identityStateDB.Remove(delegators[0][9])
		identityStateDB.Remove(delegators[16][9])
		identityStateDB.Remove(pools[24])
		identityStateDB.Remove(pools[26])
		_, _, diff3, _ := identityStateDB.Commit(true, enableUpgrade8)
		vCache.UpdateFromIdentityStateDiff(diff3)

		require.True(vCache.IsPool(pools[0]))
		require.True(vCache.IsDiscriminated(pools[0]))
		require.True(vCache.IsPool(pools[16]))
		require.False(vCache.IsDiscriminated(pools[16]))
		require.Equal(1, vCache.pools[pools[16]].approved.Cardinality())
		require.False(vCache.IsDiscriminated(pools[24]))
		require.True(vCache.IsDiscriminated(pools[26]))

		identityStateDB.SetDiscriminated(pools[16], true)
		_, _, diff4, _ := identityStateDB.Commit(true, enableUpgrade8)
		vCache.UpdateFromIdentityStateDiff(diff4)

		require.True(vCache.IsPool(pools[16]))
		require.True(vCache.IsDiscriminated(pools[16]))
	}
}

func TestValidatorsCache_LoadAndUpdateFromIdentityStateDiff(t *testing.T) {
	test_LoadAndUpdateFromIdentityStateDiff(true, t)
	test_LoadAndUpdateFromIdentityStateDiff(false, t)
}

func TestValidatorsCache_Clone(t *testing.T) {
	require := require.New(t)
	database := db.NewMemDB()
	identityStateDB, _ := state.NewLazyIdentityState(database)

	countAll := 100
	var delegators []common.Address

	for j := 0; j < countAll; j++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)

		obj := identityStateDB.GetOrNewIdentityObject(addr)
		obj.SetValidated(true)

		if rand.Int31()%2 == 0 {
			obj.SetOnline(true)
		}
		if rand.Int31()%3 == 0 {
			delegators = append(delegators, addr)
			obj.SetDelegatee(common.Address{0x1})
		}
		if rand.Int31()%4 == 0 {
			obj.SetDiscriminated(true)
		}
	}
	identityStateDB.Commit(false, enableUpgrade8)

	vCache := NewValidatorsCache(identityStateDB, common.Address{0x1})
	vCache.Load()

	clone := vCache.Clone()

	require.Equal(vCache.height, clone.height)
	require.Equal(vCache.god, clone.god)
	require.Equal(vCache.sortedValidators, clone.sortedValidators)
	require.Equal(vCache.onlineAddresses, clone.onlineAddresses)
	require.Equal(vCache.validatedAddresses, clone.validatedAddresses)
	require.False(vCache.onlineAddresses == clone.onlineAddresses)
	require.False(vCache.validatedAddresses == clone.validatedAddresses)
	require.Equal(vCache.delegations, clone.delegations)
	require.Equal(vCache.discriminatedAddresses, clone.discriminatedAddresses)
	require.Equal(vCache.pools, clone.pools)

	require.Equal(vCache.pools[common.Address{0x1}].approved, clone.pools[common.Address{0x1}].approved)
	require.Equal(vCache.pools[common.Address{0x1}].delegators, clone.pools[common.Address{0x1}].delegators)
	approvedLen := clone.pools[common.Address{0x1}].approved.Cardinality()
	delegatorsLen := len(clone.pools[common.Address{0x1}].delegators)

	vCache.pools[common.Address{0x1}].remove(delegators[0])

	require.NotEqual(vCache.pools, clone.pools)
	require.Equal(approvedLen, clone.pools[common.Address{0x1}].approved.Cardinality())
	require.Equal(delegatorsLen, len(clone.pools[common.Address{0x1}].delegators))
}

func Test_pool_index(t *testing.T) {
	pool := newPool(common.Address{0x8}, true)

	pool.add(common.Address{0x5}, true)
	pool.add(common.Address{0x2}, true)
	pool.add(common.Address{0x1}, true)
	pool.add(common.Address{0x4}, true)
	pool.add(common.Address{0x3}, true)

	for i := 0; i < 5; i++ {
		require.Equal(t, common.Address{byte(i + 1)}, pool.delegators[i])
	}

	for i := 0; i < 5; i++ {
		require.Equal(t, i, pool.index(common.Address{byte(i + 1)}))
	}
	require.Equal(t, -1, pool.index(common.Address{0x6}))

	pool.add(common.Address{0x9}, true)

	require.Equal(t, -1, pool.index(common.Address{0x8}))
}

func TestValidatorsCache_pool_discriminated(t *testing.T) {
	p := newPool(common.Address{0x1}, true)
	require.False(t, p.discriminated())
	p.add(common.Address{0x2}, false)
	require.False(t, p.discriminated())
	p.add(common.Address{0x3}, true)
	require.False(t, p.discriminated())
	p.setApproved(common.Address{0x1}, false)
	require.False(t, p.discriminated())
	p.remove(common.Address{0x3})
	require.True(t, p.discriminated())
	p.setApproved(common.Address{0x1}, true)
	require.False(t, p.discriminated())

	p = newPool(common.Address{0x1}, false)
	require.True(t, p.discriminated())
	p.add(common.Address{0x2}, false)
	require.True(t, p.discriminated())
	p.add(common.Address{0x3}, true)
	require.False(t, p.discriminated())
	p.remove(common.Address{0x2})
	require.False(t, p.discriminated())
	p.remove(common.Address{0x3})
	require.True(t, p.discriminated())
	p.setApproved(common.Address{0x1}, true)
	require.False(t, p.discriminated())
}

func TestValidatorsCache_pool_index(t *testing.T) {
	p := newPool(common.Address{0x1}, true)
	p.add(common.Address{0x4}, true)
	p.add(common.Address{0x6}, true)
	p.add(common.Address{0x2}, true)
	p.add(common.Address{0x5}, true)
	p.add(common.Address{0x3}, true)

	require.Equal(t, -1, p.index(common.Address{0x1}))
	require.Equal(t, -1, p.index(common.Address{}))
	require.Equal(t, -1, p.index(common.Address{0x7}))
	require.Equal(t, 0, p.index(common.Address{0x2}))
	require.Equal(t, 1, p.index(common.Address{0x3}))
	require.Equal(t, 2, p.index(common.Address{0x4}))
	require.Equal(t, 3, p.index(common.Address{0x5}))
	require.Equal(t, 4, p.index(common.Address{0x6}))
}

func TestValidatorsCache_determineValidators(t *testing.T) {
	require := require.New(t)
	database := db.NewMemDB()
	identityStateDB, _ := state.NewLazyIdentityState(database)
	vCache := NewValidatorsCache(identityStateDB, common.Address{0x1})

	vCache.delegations = map[common.Address]common.Address{
		{0x1}: {0x1, 0x1},
		{0x2}: {0x2, 0x2},
	}

	vCache.pools = map[common.Address]*pool{
		{0x1, 0x1}: {approved: mapset.NewSet(common.Address{0x1, 0x1, 0x1})},
		{0x2, 0x2}: {approved: mapset.NewSet()},
	}

	vCache.discriminatedAddresses = mapset.NewSet(common.Address{0x3})

	validators, approvedValidators := vCache.determineValidators(mapset.NewSet(common.Address{0x1}, common.Address{0x2}, common.Address{0x3}, common.Address{0x4}))

	require.Equal(mapset.NewSet(common.Address{0x1, 0x1}, common.Address{0x2, 0x2}, common.Address{0x3}, common.Address{0x4}), validators)
	require.Equal(mapset.NewSet(common.Address{0x1, 0x1}, common.Address{0x4}), approvedValidators)
}
