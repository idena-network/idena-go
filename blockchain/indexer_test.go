package blockchain

import (
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/tests"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestBlockchain_saveOwnTxs(t *testing.T) {
	require := require.New(t)

	chain, _, _, key := NewTestBlockchain(true, nil)

	key2, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	txs := []txWithTimestamp{
		{tx: tests.GetFullTx(1, 1, key, types.SendTx, nil, &addr, nil), timestamp: 10},
		{tx: tests.GetFullTx(2, 1, key, types.SendTx, nil, &addr, nil), timestamp: 20},
		{tx: tests.GetFullTx(4, 1, key, types.SendTx, nil, &addr, nil), timestamp: 30},
		{tx: tests.GetFullTx(5, 1, key, types.SendTx, nil, &addr, nil), timestamp: 35},
		{tx: tests.GetFullTx(6, 1, key, types.SendTx, nil, &addr, nil), timestamp: 50},
		{tx: tests.GetFullTx(7, 1, key, types.SendTx, nil, &addr, nil), timestamp: 80},
		{tx: tests.GetFullTx(9, 1, key, types.SendTx, nil, &addr, nil), timestamp: 80},
		{tx: tests.GetFullTx(9, 1, key, types.SendTx, nil, &addr, nil), timestamp: 456},
		{tx: tests.GetFullTx(10, 1, key, types.SendTx, nil, &addr, nil), timestamp: 456},
		{tx: tests.GetFullTx(1, 2, key, types.SendTx, nil, &addr, nil), timestamp: 500},
		{tx: tests.GetFullTx(2, 2, key, types.SendTx, nil, &addr, nil), timestamp: 500},

		{tx: tests.GetFullTx(1, 1, key2, types.SendTx, nil, &addr, nil), timestamp: 20},
		{tx: tests.GetFullTx(8, 1, key2, types.SendTx, nil, &addr, nil), timestamp: 80},
		{tx: tests.GetFullTx(10, 1, key2, types.SendTx, nil, &addr, nil), timestamp: 80},
		{tx: tests.GetFullTx(4, 1, key2, types.SendTx, nil, &addr, nil), timestamp: 456},
	}

	for _, item := range txs {
		header := &types.Header{
			ProposedHeader: &types.ProposedHeader{
				Time:       new(big.Int).SetUint64(item.timestamp),
				FeePerByte: big.NewInt(1),
			},
		}
		chain.indexer.HandleBlockTransactions(header, []*types.Transaction{item.tx})
	}

	data, token := chain.ReadTxs(addr, 5, nil)

	require.Equal(5, len(data))
	require.Equal(uint32(2), data[0].Tx.AccountNonce)
	require.Equal(uint32(1), data[1].Tx.AccountNonce)
	require.Equal(uint32(10), data[2].Tx.AccountNonce)
	require.Equal(uint32(9), data[3].Tx.AccountNonce)
	require.Equal(uint32(4), data[4].Tx.AccountNonce)
	require.Equal(uint64(456), data[4].Timestamp)
	require.NotNil(token)

	data, token = chain.ReadTxs(addr, 4, token)

	require.Equal(4, len(data))
	require.Equal(uint32(10), data[0].Tx.AccountNonce)
	require.Equal(uint32(9), data[1].Tx.AccountNonce)
	require.Equal(uint32(8), data[2].Tx.AccountNonce)
	require.Equal(uint32(7), data[3].Tx.AccountNonce)
	require.NotNil(token)

	data, token = chain.ReadTxs(addr, 10, token)

	require.Equal(6, len(data))
	require.Equal(uint32(6), data[0].Tx.AccountNonce)
	require.Equal(uint32(5), data[1].Tx.AccountNonce)
	require.Equal(uint32(4), data[2].Tx.AccountNonce)
	require.Equal(uint32(2), data[3].Tx.AccountNonce)
	require.Equal(uint32(1), data[4].Tx.AccountNonce)
	require.Equal(uint32(1), data[5].Tx.AccountNonce)
	require.Equal(uint64(20), data[4].Timestamp)
	require.Equal(uint64(10), data[5].Timestamp)
	require.Nil(token)
}

func Test_handleOwnTxsWithAccounts(t *testing.T) {
	require := require.New(t)

	chain, _, _, key := NewTestBlockchain(true, nil)
	key2, _ := crypto.GenerateKey()
	key3, _ := crypto.GenerateKey()
	key4, _ := crypto.GenerateKey()

	addr1 := crypto.PubkeyToAddress(key.PublicKey)
	addr2 := crypto.PubkeyToAddress(key2.PublicKey)
	addr3 := crypto.PubkeyToAddress(key3.PublicKey)
	addr4 := crypto.PubkeyToAddress(key4.PublicKey)

	accountsMap := map[common.Address]struct{}{
		addr1: {},
		addr3: {},
	}

	txs := []txWithTimestamp{
		{tx: tests.GetFullTx(1, 1, key, types.SendTx, nil, &addr2, nil), timestamp: 10}, // indexed
		{tx: tests.GetFullTx(2, 1, key, types.SendTx, nil, &addr3, nil), timestamp: 20}, // indexed

		{tx: tests.GetFullTx(4, 1, key2, types.SendTx, nil, &addr2, nil), timestamp: 30}, // not indexed
		{tx: tests.GetFullTx(5, 1, key2, types.SendTx, nil, &addr3, nil), timestamp: 35}, // indexed
		{tx: tests.GetFullTx(6, 1, key2, types.SendTx, nil, &addr4, nil), timestamp: 50}, // not indexed

		{tx: tests.GetFullTx(7, 1, key3, types.SendTx, nil, &addr1, nil), timestamp: 80}, // indexed
		{tx: tests.GetFullTx(9, 1, key3, types.SendTx, nil, &addr2, nil), timestamp: 80}, // indexed
		{tx: tests.GetFullTx(7, 1, key3, types.SendTx, nil, &addr3, nil), timestamp: 80}, // indexed

		{tx: tests.GetFullTx(9, 1, key4, types.SendTx, nil, &addr1, nil), timestamp: 456},  // not indexed
		{tx: tests.GetFullTx(10, 1, key4, types.SendTx, nil, &addr2, nil), timestamp: 456}, // not indexed
	}

	for _, item := range txs {
		header := &types.Header{
			ProposedHeader: &types.ProposedHeader{
				Time:       new(big.Int).SetUint64(item.timestamp),
				FeePerByte: big.NewInt(1),
			},
		}
		sender, _ := types.Sender(item.tx)
		chain.indexer.handleOwnTx(header, sender, item.tx, accountsMap)
	}

	data, _ := chain.ReadTxs(addr1, 10, nil)
	require.Equal(4, len(data))

	data, _ = chain.ReadTxs(addr2, 10, nil)
	require.Equal(0, len(data))

	data, _ = chain.ReadTxs(addr3, 10, nil)
	require.Equal(5, len(data))

	data, _ = chain.ReadTxs(addr4, 10, nil)
	require.Equal(0, len(data))
}

func Test_Blockchain_saveBurntCoins(t *testing.T) {
	require := require.New(t)

	chain, _, _, key := NewTestBlockchain(true, nil)
	chain.config.Blockchain.BurnTxRange = 3

	key2, _ := crypto.GenerateKey()

	createHeader := func(height uint64) *types.Header {
		return &types.Header{
			ProposedHeader: &types.ProposedHeader{
				Height: height,
				Time:   big.NewInt(0),
			},
		}
	}

	// Block height=1
	chain.indexer.HandleBlockTransactions(createHeader(1), []*types.Transaction{
		tests.GetFullTx(0, 0, key2, types.BurnTx, big.NewInt(3), nil,
			attachments.CreateBurnAttachment("3")),
		tests.GetFullTx(0, 0, key2, types.BurnTx, big.NewInt(4), nil,
			attachments.CreateBurnAttachment("2")),
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(2), nil,
			attachments.CreateBurnAttachment("1")),
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(3), nil,
			attachments.CreateBurnAttachment("1")),
	})

	addr := crypto.PubkeyToAddress(key.PublicKey)
	addr2 := crypto.PubkeyToAddress(key2.PublicKey)

	burntCoins := chain.ReadTotalBurntCoins()
	require.Equal(3, len(burntCoins))
	require.Equal(addr, burntCoins[0].Address)
	require.Equal(big.NewInt(5), burntCoins[0].Amount)

	require.Equal(addr2, burntCoins[1].Address)
	require.Equal("2", burntCoins[1].Key)
	require.Equal(big.NewInt(4), burntCoins[1].Amount)

	require.Equal(addr2, burntCoins[2].Address)
	require.Equal("3", burntCoins[2].Key)
	require.Equal(big.NewInt(3), burntCoins[2].Amount)

	// Block height=2
	chain.indexer.HandleBlockTransactions(createHeader(2), []*types.Transaction{
		tests.GetFullTx(0, 0, key2, types.BurnTx, big.NewInt(5), nil,
			attachments.CreateBurnAttachment("2")),
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(1), nil,
			attachments.CreateBurnAttachment("1")),
		tests.GetFullTx(0, 0, key, types.SendTx, big.NewInt(2), nil, nil),
	})

	burntCoins = chain.ReadTotalBurntCoins()
	require.Equal(3, len(burntCoins))
	require.Equal(addr2, burntCoins[0].Address)
	require.Equal(big.NewInt(9), burntCoins[0].Amount)
	require.Equal("2", burntCoins[0].Key)

	require.Equal(addr, burntCoins[1].Address)
	require.Equal(big.NewInt(6), burntCoins[1].Amount)

	require.Equal(addr2, burntCoins[2].Address)
	require.Equal("3", burntCoins[2].Key)
	require.Equal(big.NewInt(3), burntCoins[2].Amount)

	// Block height=4
	chain.indexer.HandleBlockTransactions(createHeader(4), []*types.Transaction{
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(3), nil,
			attachments.CreateBurnAttachment("1")),
	})

	burntCoins = chain.ReadTotalBurntCoins()
	require.Equal(2, len(burntCoins))
	require.Equal(addr2, burntCoins[0].Address)
	require.Equal(big.NewInt(5), burntCoins[0].Amount)
	require.Equal(addr, burntCoins[1].Address)
	require.Equal(big.NewInt(4), burntCoins[1].Amount)

	// Block height=7
	chain.indexer.HandleBlockTransactions(createHeader(7), []*types.Transaction{
		tests.GetFullTx(0, 0, key, types.BurnTx, big.NewInt(1), nil,
			attachments.CreateBurnAttachment("1")),
	})

	burntCoins = chain.ReadTotalBurntCoins()
	require.Equal(1, len(burntCoins))
	require.Equal(addr, burntCoins[0].Address)
	require.Equal(big.NewInt(1), burntCoins[0].Amount)
}
