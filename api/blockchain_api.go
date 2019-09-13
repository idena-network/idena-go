package api

import (
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/protocol"
	"github.com/ipfs/go-cid"
	"github.com/shopspring/decimal"
	"math/big"
)

var (
	txTypeMap = map[types.TxType]string{
		types.SendTx:               "send",
		types.ActivationTx:         "activation",
		types.InviteTx:             "invite",
		types.KillTx:               "kill",
		types.KillInviteeTx:        "killInvitee",
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
	Coinbase     common.Address  `json:"coinbase"`
	Hash         common.Hash     `json:"hash"`
	ParentHash   common.Hash     `json:"parentHash"`
	Height       uint64          `json:"height"`
	Time         *big.Int        `json:"timestamp"`
	Root         common.Hash     `json:"root"`         // root of state tree
	IdentityRoot common.Hash     `json:"identityRoot"` // root of approved identities tree
	IpfsHash     *string         `json:"ipfsCid"`      // ipfs hash of block body
	Transactions []common.Hash   `json:"transactions"`
	Flags        []string        `json:"flags"`
	IsEmpty      bool            `json:"isEmpty"`
	OfflineAddr  *common.Address `json:"offlineAddress"`
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
	tx := api.pool.GetTx(hash)
	var idx *types.TransactionIndex

	if tx == nil {
		tx, idx = api.bc.GetTx(hash)
	}

	if tx == nil {
		return nil
	}

	if idx == nil {
		idx = api.bc.GetTxIndex(hash)
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
	WrongTime    bool   `json:"wrongTime"`
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
		WrongTime:    api.pm.WrongTime(),
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

	var ipfsHashStr *string
	ipfsHash := block.Header.IpfsHash()
	if len(ipfsHash) > 0 {
		c, _ := cid.Parse(ipfsHash)
		stringCid := c.String()
		ipfsHashStr = &stringCid
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
	if block.Header.Flags().HasFlag(types.OfflinePropose) {
		flags = append(flags, "OfflinePropose")
	}
	if block.Header.Flags().HasFlag(types.OfflineCommit) {
		flags = append(flags, "OfflineCommit")
	}
	if block.Header.Flags().HasFlag(types.Snapshot) {
		flags = append(flags, "Snapshot")
	}

	var coinbase common.Address
	if !block.IsEmpty() {
		coinbase = block.Header.ProposedHeader.Coinbase
	}

	return &Block{
		Coinbase:     coinbase,
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
		OfflineAddr:  block.Header.OfflineAddr(),
	}
}
