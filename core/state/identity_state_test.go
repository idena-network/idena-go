package state

import (
	"crypto/rand"
	"github.com/idena-network/idena-go/common"
	"github.com/stretchr/testify/require"
	db2 "github.com/tendermint/tendermint/libs/db"
	"testing"
	"time"
)

func getRandAddr() common.Address {
	bytes := make([]byte, 20)
	rand.Read(bytes)
	return common.BytesToAddress(bytes)
}

func TestIdentityStateDB_Diff(t *testing.T) {
	db := db2.NewMemDB()
	state := NewLazyIdentityState(db)

	var firstGroup []common.Address

	for i := 0; i < 10; i++ {
		addr := getRandAddr()
		state.Add(addr)
		firstGroup = append(firstGroup, addr)
	}

	state.Commit(true)

	var secondGroup []common.Address

	for i := 0; i < 10; i++ {
		addr := getRandAddr()
		state.Add(addr)
		secondGroup = append(secondGroup, addr)
	}

	state.Remove(firstGroup[0])

	state.Commit(true)

	statePrev := NewLazyIdentityState(db)
	statePrev.Load(1)

	diff := state.Diff(statePrev)

	require := require.New(t)

	require.Equal(len(secondGroup), len(diff.Added))

	for _, addr := range secondGroup {
		_, ok := diff.Added[addr]
		require.True(ok)
	}

	require.Equal(0, len(diff.Updated))

	require.Equal(1, len(diff.Deleted))
	require.Equal(firstGroup[0], diff.Deleted[0])


	time.Now().Nanosecond()
}
