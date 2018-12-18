package validators

import (
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/db"
	"idena-go/common"
	"idena-go/core/state"
	"idena-go/crypto"
	"testing"
)

func TestValidatorsCache_Contains(t *testing.T) {
	database := db.NewMemDB()
	stateDb, _ := state.NewLatest(database)

	var arr []common.Address

	for j := 0; j < 10; j++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)
		arr = append(arr, addr)
		obj := stateDb.GetOrNewIdentityObject(addr)
		obj.Approve()
	}
	stateDb.Commit(false)

	vCache := NewValidatorsCache(stateDb)
	vCache.Load()

	for i := 0; i < len(arr); i++ {
		require.True(t, vCache.Contains(arr[i]))
	}
}
