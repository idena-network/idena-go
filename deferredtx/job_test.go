package deferredtx

import (
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/idena-network/idena-go/vm"
	"github.com/idena-network/idena-go/vm/embedded"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

var fakeVmError error

type fakeVm struct {
}

func (f fakeVm) Read(contractAddr common.Address, method string, args ...[]byte) ([]byte, error) {
	panic("implement me")
}
func (f fakeVm) Run(tx *types.Transaction, gasLimit int64) *types.TxReceipt {
	return &types.TxReceipt{
		Error:   fakeVmError,
		Success: fakeVmError == nil,
	}
}

type fakeTxPool struct {
	counter int
}

func (f *fakeTxPool) Add(tx *types.Transaction) error {
	f.counter++
	return nil
}

func (f fakeTxPool) GetPendingTransaction() []*types.Transaction {
	panic("implement me")
}

func (f fakeTxPool) IsSyncing() bool {
	return false
}

func TestJob_tryLater(t *testing.T) {

	fakeVmError =  embedded.NewContractError("", true)
	chain, appState, _, _ := blockchain.NewTestBlockchain(false, nil)
	os.RemoveAll("test")

	txPool := &fakeTxPool{}
	job, _ := NewJob(chain.Bus(), "test", appState, chain.Blockchain, txPool, nil, chain.SecStore(), func(appState *appstate.AppState, block *types.Header, store *secstore.SecStore, statsCollector collector.StatsCollector, cfg *config.Config) vm.VM {
		return &fakeVm{}
	})
	coinbase := chain.SecStore().GetAddress()

	require.NoError(t, job.AddDeferredTx(coinbase, &common.Address{0x1}, common.DnaBase, nil, nil, 10))
	require.NoError(t, job.AddDeferredTx(coinbase, &common.Address{0x1}, common.DnaBase, nil, nil, 50))
	require.Len(t, job.txs.Txs, 2)

	chain.GenerateEmptyBlocks(8)
	require.Len(t, job.txs.Txs, 2)

	chain.GenerateEmptyBlocks(1)
	require.Equal(t, 1, job.txs.Txs[0].sendTry)
	require.Equal(t, uint64(11), job.txs.Txs[0].BroadcastBlock)

	chain.GenerateEmptyBlocks(1)
	require.Equal(t, 2, job.txs.Txs[0].sendTry)
	require.Equal(t, uint64(13), job.txs.Txs[0].BroadcastBlock)

	chain.GenerateEmptyBlocks(4)
	require.Equal(t, 3, job.txs.Txs[0].sendTry)
	require.Equal(t, uint64(17), job.txs.Txs[0].BroadcastBlock)

	chain.GenerateEmptyBlocks(8)
	require.Equal(t, 4, job.txs.Txs[0].sendTry)
	require.Equal(t, uint64(25), job.txs.Txs[0].BroadcastBlock)

	chain.GenerateEmptyBlocks(2)
	require.Equal(t, 5, job.txs.Txs[0].sendTry)
	require.Equal(t, uint64(33), job.txs.Txs[0].BroadcastBlock)

	fakeVmError = nil
	chain.GenerateEmptyBlocks(10)

	require.Len(t, job.txs.Txs, 1)
	require.Equal(t, 1, txPool.counter)

	fakeVmError = errors.New("custom error")
	chain.GenerateEmptyBlocks(50)

	require.Len(t, job.txs.Txs, 0)
	require.Equal(t, 1, txPool.counter)
}
