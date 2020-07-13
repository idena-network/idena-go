package ceremony

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_addReport(t *testing.T) {
	r := newReportersToReward()
	addr := common.Address{1}
	addr2 := common.Address{2}
	addr3 := common.Address{3}

	r.addReport(5, addr)
	r.addReport(5, addr2)
	r.addReport(5, addr3)
	r.addReport(6, addr)

	require.Equal(t, 3, len(r.reportersByAddr))
	require.Equal(t, 2, len(r.reportersByFlip))
	require.Equal(t, 3, len(r.reportedFlipsByReporter))
	require.Equal(t, addr2, r.reportersByFlip[5][addr2].Address)
}

func Test_getReportedFlipsCountByReporter(t *testing.T) {
	r := newReportersToReward()
	addr := common.Address{1}
	addr2 := common.Address{2}
	r.addReport(2, addr)
	r.addReport(2, addr2)
	r.addReport(3, addr)

	require.Equal(t, 2, r.getReportedFlipsCountByReporter(addr))
	require.Equal(t, 1, r.getReportedFlipsCountByReporter(addr2))
}

func Test_getFlipReportsCount(t *testing.T) {
	r := newReportersToReward()
	addr := common.Address{1}
	addr2 := common.Address{2}
	r.addReport(2, addr)
	r.addReport(2, addr2)
	r.addReport(3, addr)

	require.Equal(t, 0, r.getFlipReportsCount(1))
	require.Equal(t, 2, r.getFlipReportsCount(2))
	require.Equal(t, 1, r.getFlipReportsCount(3))
}

func Test_deleteFlip(t *testing.T) {
	r := newReportersToReward()
	addr := common.Address{1}
	addr2 := common.Address{2}
	r.addReport(2, addr)
	r.addReport(2, addr2)
	r.addReport(3, addr)

	r.deleteFlip(3)
	require.Equal(t, 1, len(r.reportersByFlip))
	require.Equal(t, 2, len(r.reportersByAddr))
	require.Equal(t, 2, len(r.reportedFlipsByReporter))
	require.Equal(t, 1, len(r.reportedFlipsByReporter[addr]))
	require.Equal(t, 1, len(r.reportedFlipsByReporter[addr2]))

	r.deleteFlip(2)

	require.Empty(t, r.reportersByFlip)
	require.Empty(t, r.reportersByAddr)
	require.Empty(t, r.reportedFlipsByReporter)
}

func Test_deleteReporter(t *testing.T) {
	r := newReportersToReward()
	addr := common.Address{1}
	addr2 := common.Address{2}
	r.addReport(2, addr)
	r.addReport(2, addr2)
	r.addReport(3, addr)

	r.deleteReporter(addr2)

	require.Equal(t, 2, len(r.reportersByFlip))
	require.Equal(t, 1, len(r.reportersByAddr))
	require.Equal(t, 1, len(r.reportedFlipsByReporter))
	require.Equal(t, 2, len(r.reportedFlipsByReporter[addr]))

	r.deleteReporter(addr)

	require.Empty(t, r.reportersByFlip)
	require.Empty(t, r.reportersByAddr)
	require.Empty(t, r.reportedFlipsByReporter)
}

func Test_setValidationResult(t *testing.T) {
	r := newReportersToReward()
	addr1 := common.Address{1}
	addr2 := common.Address{2}
	addr3 := common.Address{3}
	addr4 := common.Address{4}

	r.addReport(2, addr1)
	r.addReport(3, addr1)
	r.addReport(2, addr2)

	flipsByAuthor := map[common.Address][]int{
		addr3: {1, 2},
		addr4: {3, 4},
	}

	r.setValidationResult(addr1, state.Newbie, false, flipsByAuthor)
	require.Equal(t, uint8(state.Newbie), r.reportersByAddr[addr1].NewIdentityState)

	r.setValidationResult(addr2, state.Human, false, flipsByAuthor)
	require.Equal(t, uint8(state.Human), r.reportersByAddr[addr2].NewIdentityState)

	r.setValidationResult(addr1, state.Killed, false, flipsByAuthor)
	require.Equal(t, 1, len(r.reportedFlipsByReporter))
	require.Equal(t, 1, len(r.reportersByAddr))
	require.NotContains(t, r.reportersByAddr, addr1)
	require.NotContains(t, r.reportedFlipsByReporter, addr1)

	r.setValidationResult(addr3, state.Suspended, true, flipsByAuthor)
	require.Equal(t, 0, len(r.reportedFlipsByReporter))
	require.Equal(t, 0, len(r.reportersByAddr))
	require.Equal(t, 0, len(r.reportersByFlip))
}
