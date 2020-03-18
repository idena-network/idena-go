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
	identityStateDB := state.NewLazyIdentityState(database)

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


func TestValidatorsCache_Clone(t *testing.T) {
	require := require.New(t)
	database := db.NewMemDB()
	identityStateDB := state.NewLazyIdentityState(database)

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

	vCache := NewValidatorsCache(identityStateDB, common.Address{0x1})
	vCache.Load()

	clone := vCache.Clone()


	require.Equal(vCache.height, clone.height)
	require.Equal(vCache.god, clone.god)
	require.Equal(vCache.validOnlineNodes, clone.validOnlineNodes)
	require.Equal(vCache.onlineNodesSet, clone.onlineNodesSet)
	require.Equal(vCache.nodesSet, clone.nodesSet)
	require.False(vCache.onlineNodesSet == clone.onlineNodesSet)
	require.False(vCache.nodesSet == clone.nodesSet)
}
