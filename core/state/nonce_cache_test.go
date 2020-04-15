package state

import (
	"github.com/idena-network/idena-go/common"
	"github.com/stretchr/testify/require"
	"testing"
)
import dbm "github.com/tendermint/tm-db"

func TestNonceCache_Set_And_Get_Nonce(t *testing.T) {
	require := require.New(t)

	db := dbm.NewMemDB()
	stateDb := NewLazy(db)
	stateDb.IncEpoch()
	epoch := uint16(1)

	var addr common.Address
	addr.SetBytes([]byte{0x1})

	stateDb.SetNonce(addr, 5)
	stateDb.SetEpoch(addr, epoch)
	stateDb.Commit(false)

	ns, _ := NewNonceCache(stateDb)
	ns.SetNonce(addr, epoch, 3)
	require.Equal(uint32(5), ns.GetNonce(addr, epoch))
	require.Equal(uint32(0), ns.GetNonce(addr, epoch+1))

	ns.SetNonce(addr, epoch, 6)
	ns.SetNonce(addr, epoch+1, 1)

	require.Equal(uint32(6), ns.GetNonce(addr, epoch))
	require.Equal(uint32(1), ns.GetNonce(addr, epoch+1))

	ns.SetNonce(addr, epoch, 8)
	ns.SetNonce(addr, epoch, 7)
	require.Equal(uint32(8), ns.GetNonce(addr, epoch))

	require.Equal(uint32(0), ns.GetNonce(addr, epoch+2))

}
