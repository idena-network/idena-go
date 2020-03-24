package consensus

import (
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/protocol"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestForkResolver_ResolveFork(t *testing.T) {
	key, _ := crypto.GenerateKey()
	chain, _ := blockchain.NewCustomTestBlockchain(100, 0, key)
	chain2, _ := chain.Copy()
	chain2.ResetTo(80)
	require.Equal(t, chain.GetBlockHeaderByHeight(80).Hash(), chain2.Head.Hash())

	chain2.GenerateEmptyBlocks(1).GenerateBlocks(20)

	require.Equal(t, chain.Head.Height(), chain2.Head.Height())

	forkHashes := chain2.GetTopBlockHashes(100)
	initialHashes := chain.GetTopBlockHashes(100)

	require.NotEqual(t, forkHashes, initialHashes)

	//small fork
	resolver := NewForkResolver([]ForkDetector{}, nil, chain.Blockchain, collector.NewStatsCollector())
	forkBlocks := chain2.ReadBlockForForkedPeer(initialHashes)
	require.Len(t, forkBlocks, 21)
	blocks := make(chan types.BlockBundle, 21)

	for _, b := range forkBlocks {
		blocks <- b
	}
	close(blocks)
	err := resolver.processBlocks(blocks, "test-peer")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "fork has less proposed blocks"))

	//applicable fork
	chain2.ResetTo(80)
	chain2.GenerateBlocks(26)
	forkBlocks = chain2.ReadBlockForForkedPeer(initialHashes)
	blocks = make(chan types.BlockBundle, 22)

	for _, b := range forkBlocks {
		blocks <- b
	}
	close(blocks)
	err = resolver.processBlocks(blocks, "test-peer")
	require.NoError(t, err)
	require.True(t, resolver.HasLoadedFork())

	require.NoError(t, resolver.ApplyFork())
	require.False(t, resolver.HasLoadedFork())
	require.Nil(t, resolver.applicableFork)
	require.True(t, resolver.triedPeers.Cardinality() == 0)

	require.Equal(t, chain.Head.Hash(), chain2.GetBlockHeaderByHeight(chain.Head.Height()).Hash())
}

func TestForkResolver_ResolveFork2(t *testing.T) {
	key, _ := crypto.GenerateKey()
	chain, _ := blockchain.NewCustomTestBlockchain(100, 0, key)
	chain2, _ := chain.Copy()
	chain2.ResetTo(80)
	chain2.GenerateBlocks(50)

	//missed certificate
	resolver := NewForkResolver([]ForkDetector{}, &protocol.Downloader{}, chain.Blockchain, collector.NewStatsCollector())
	initialHashes := chain.GetTopBlockHashes(100)
	forkBlocks := chain2.ReadBlockForForkedPeer(initialHashes)
	blocks := make(chan types.BlockBundle, 22)
	forkBlocks[len(forkBlocks)-1].Cert = nil
	for _, b := range forkBlocks {
		blocks <- b
	}
	close(blocks)
	err := resolver.processBlocks(blocks, "test-peer")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "last block of the fork"))

	//corrupted chain
	forkBlocks = chain2.ReadBlockForForkedPeer(initialHashes)
	blocks = make(chan types.BlockBundle, 25)
	forkBlocks[len(forkBlocks)-1].Cert = nil
	for i, b := range forkBlocks {
		blocks <- b
		if i == 3 {
			blocks <- b
			blocks <- b
		}
	}
	close(blocks)
	err = resolver.processBlocks(blocks, "test-peer")
	require.Error(t, err)

	//invalid voter
	forkBlocks = chain2.ReadBlockForForkedPeer(initialHashes)
	blocks = make(chan types.BlockBundle, 25)
	forkBlocks[len(forkBlocks)-1].Cert = nil
	for i, b := range forkBlocks {
		blocks <- b
		if i == 3 {
			b.Cert.Signatures[0].Signature[0] = 0
			b.Cert.Signatures[0].Signature[1] = 0
		}
	}
	close(blocks)
	err = resolver.processBlocks(blocks, "test-peer")
	require.Error(t, err)

	//no common block
	forkBlocks = chain2.ReadBlockForForkedPeer(initialHashes)
	blocks = make(chan types.BlockBundle, 25)
	forkBlocks[len(forkBlocks)-1].Cert = nil
	for _, b := range forkBlocks[1:] {
		blocks <- b
	}
	close(blocks)
	err = resolver.processBlocks(blocks, "test-peer")
	require.Error(t, err)
}
