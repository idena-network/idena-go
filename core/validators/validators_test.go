package validators

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tm-db"
	"math/rand"
	"testing"
)

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
		obj.SetState(true)

		isOnline := rand.Int31()%2 == 0
		m[addr] = isOnline

		if isOnline {
			obj.SetOnline(true)
			countOnline++
		}
	}
	identityStateDB.Commit(false)

	vCache := NewValidatorsCache(identityStateDB, common.Address{})
	vCache.Load()

	for addr, online := range m {
		require.Equal(online, vCache.IsOnlineIdentity(addr))
		require.True(vCache.Contains(addr))
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

	for j := 0; j < countAll; j++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)

		obj := identityStateDB.GetOrNewIdentityObject(addr)
		obj.SetState(true)
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
	identityStateDB.Add(pool2)
	identityStateDB.SetOnline(pool3, false)

	identityStateDB.Commit(false)

	vCache := NewValidatorsCache(identityStateDB, common.Address{0x11})
	vCache.Load()

	require.Equal(4, vCache.PoolSize(pool1))
	require.Equal(3, vCache.PoolSize(pool2))
	require.Equal(1, vCache.PoolSize(pool3))
	require.True(vCache.IsPool(pool1))
	require.True(vCache.IsPool(pool2))
	require.True(vCache.IsPool(pool3))
	require.False(vCache.IsPool(common.Address{0x4}))
	require.Len(vCache.delegations, 7)

	require.Len(vCache.sortedValidators, 12)

	require.NotContains(vCache.sortedValidators, pool1)
	require.Contains(vCache.sortedValidators, pool2)
	require.NotContains(vCache.sortedValidators, pool3)

	stepValidators := vCache.GetOnlineValidators([32]byte{0x1}, 1, 1, 12)
	require.Equal(12, stepValidators.Original.Cardinality())

	require.True(stepValidators.Addresses.Contains(pool1))
	require.True(stepValidators.Addresses.Contains(pool2))

	require.False(stepValidators.Original.Contains(pool1))
	require.True(stepValidators.Original.Contains(pool2))

	require.False(stepValidators.Addresses.Contains(pool3))
	require.Equal(7, stepValidators.Size)
	require.Equal(5, stepValidators.VotesCountSubtrahend(1))
}

func TestValidatorsCache_Clone(t *testing.T) {
	require := require.New(t)
	database := db.NewMemDB()
	identityStateDB, _ := state.NewLazyIdentityState(database)

	countAll := 100

	for j := 0; j < countAll; j++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)

		obj := identityStateDB.GetOrNewIdentityObject(addr)
		obj.SetState(true)

		if rand.Int31()%2 == 0 {
			obj.SetOnline(true)
		}
		if rand.Int31()%3 == 0 {
			obj.SetDelegatee(common.Address{0x1})
		}
	}
	identityStateDB.Commit(false)

	vCache := NewValidatorsCache(identityStateDB, common.Address{0x1})
	vCache.Load()

	clone := vCache.Clone()

	require.Equal(vCache.height, clone.height)
	require.Equal(vCache.god, clone.god)
	require.Equal(vCache.sortedValidators, clone.sortedValidators)
	require.Equal(vCache.onlineNodesSet, clone.onlineNodesSet)
	require.Equal(vCache.nodesSet, clone.nodesSet)
	require.False(vCache.onlineNodesSet == clone.onlineNodesSet)
	require.False(vCache.nodesSet == clone.nodesSet)
	require.Equal(vCache.pools, clone.pools)
	require.Equal(vCache.delegations, clone.delegations)
}
