package api

import (
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-cid"
	"github.com/shopspring/decimal"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/hexutil"
	"idena-go/core/mempool"
	"idena-go/ipfs"
	"idena-go/protocol"
	"math/big"
)

var (
	txTypeMap = map[types.TxType]string{
		types.SendTx:               "send",
		types.ActivationTx:         "activation",
		types.InviteTx:             "invite",
		types.KillTx:               "kill",
		types.SubmitFlipTx:         "submitFlip",
		types.SubmitAnswersHashTx:  "submitAnswersHash",
		types.SubmitShortAnswersTx: "submitShortAnswers",
		types.SubmitLongAnswersTx:  "submitLongAnswers",
		types.EvidenceTx:           "evidence",
		types.OnlineStatusTx:       "online",
	}
)

type BlockchainApi struct {
	bc      *blockchain.Blockchain
	baseApi *BaseApi
	ipfs    ipfs.Proxy
	pool    *mempool.TxPool
	d       *protocol.Downloader
	pm      *protocol.ProtocolManager
}

func NewBlockchainApi(baseApi *BaseApi, bc *blockchain.Blockchain, ipfs ipfs.Proxy, pool *mempool.TxPool, d *protocol.Downloader, pm *protocol.ProtocolManager) *BlockchainApi {
	return &BlockchainApi{bc, baseApi, ipfs, pool, d, pm}
}

type Block struct {
	Hash         common.Hash   `json:"hash"`
	ParentHash   common.Hash   `json:"parentHash"`
	Height       uint64        `json:"height"`
	Time         *big.Int      `json:"timestamp"`
	Root         common.Hash   `json:"root"`         // root of state tree
	IdentityRoot common.Hash   `json:"identityRoot"` // root of approved identities tree
	IpfsHash     string        `json:"ipfsCid"`      // ipfs hash of block body
	Transactions []common.Hash `json:"transactions"`
	Flags        []string      `json:"flags"`
	IsEmpty      bool          `json:"isEmpty"`
}

type Transaction struct {
	Type      string          `json:"type"`
	From      common.Address  `json:"from"`
	To        *common.Address `json:"to"`
	Amount    decimal.Decimal `json:"amount"`
	Nonce     uint32          `json:"nonce"`
	Epoch     uint16          `json:"epoch"`
	Payload   hexutil.Bytes   `json:"payload"`
	BlockHash common.Hash     `json:"blockHash"`
}

func (api *BlockchainApi) LastBlock() *Block {
	return api.BlockAt(api.bc.Head.Height())
}

func (api *BlockchainApi) BlockAt(height uint64) *Block {
	block := api.bc.GetBlockByHeight(height)

	return convertToBlock(block)
}

func (api *BlockchainApi) Block(hash common.Hash) *Block {
	block := api.bc.GetBlock(hash)

	return convertToBlock(block)
}

func (api *BlockchainApi) Transaction(hash common.Hash) *Transaction {
	tx, idx := api.bc.GetTx(hash)
	if tx == nil {
		tx = api.pool.GetTx(hash)
		if tx == nil {
			return nil
		}
	}
	sender, _ := types.Sender(tx)

	var blockHash common.Hash
	if idx != nil {
		blockHash = idx.BlockHash
	}
	return &Transaction{
		Epoch:     tx.Epoch,
		Payload:   hexutil.Bytes(tx.Payload),
		Amount:    blockchain.ConvertToFloat(tx.Amount),
		From:      sender,
		Nonce:     tx.AccountNonce,
		To:        tx.To,
		Type:      txTypeMap[tx.Type],
		BlockHash: blockHash,
	}
}

func (api *BlockchainApi) Mempool() []common.Hash {
	pending := api.pool.GetPendingTransaction()

	var txs []common.Hash
	for _, tx := range pending {
		txs = append(txs, tx.Hash())
	}

	return txs
}

type Syncing struct {
	Syncing      bool   `json:"syncing"`
	CurrentBlock uint64 `json:"currentBlock"`
	HighestBlock uint64 `json:"highestBlock"`
}

func (api *BlockchainApi) Syncing() Syncing {
	isSyncing := api.d.IsSyncing() || !api.pm.HasPeers()
	current, highest := api.d.SyncProgress()
	if !isSyncing {
		highest = current
	}

	return Syncing{
		Syncing:      isSyncing,
		CurrentBlock: current,
		HighestBlock: highest,
	}
}

func convertToBlock(block *types.Block) *Block {
	var txs []common.Hash

	if block == nil {
		return nil
	}

	if block.Body != nil {
		for _, tx := range block.Body.Transactions {
			txs = append(txs, tx.Hash())
		}
	}

	var ipfsHashStr string
	ipfsHash := block.Header.IpfsHash()
	if ipfsHash != nil {
		c, _ := cid.Parse(ipfsHash)
		ipfsHashStr = c.String()
	}

	var flags []string
	if block.Header.Flags().HasFlag(types.IdentityUpdate) {
		flags = append(flags, "IdentityUpdate")
	}
	if block.Header.Flags().HasFlag(types.FlipLotteryStarted) {
		flags = append(flags, "FlipLotteryStarted")
	}
	if block.Header.Flags().HasFlag(types.ShortSessionStarted) {
		flags = append(flags, "ShortSessionStarted")
	}
	if block.Header.Flags().HasFlag(types.LongSessionStarted) {
		flags = append(flags, "LongSessionStarted")
	}
	if block.Header.Flags().HasFlag(types.AfterLongSessionStarted) {
		flags = append(flags, "AfterLongSessionStarted")
	}
	if block.Header.Flags().HasFlag(types.ValidationFinished) {
		flags = append(flags, "ValidationFinished")
	}

	return &Block{
		IsEmpty:      block.IsEmpty(),
		Hash:         block.Hash(),
		IdentityRoot: block.IdentityRoot(),
		Root:         block.Root(),
		Height:       block.Height(),
		ParentHash:   block.Header.ParentHash(),
		Time:         block.Header.Time(),
		IpfsHash:     ipfsHashStr,
		Transactions: txs,
		Flags:        flags,
	}
}
