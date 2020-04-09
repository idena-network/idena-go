package api

import (
	"context"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/protocol"
	"github.com/idena-network/idena-go/rlp"
	"github.com/ipfs/go-cid"
	"github.com/shopspring/decimal"
	"math/big"
	"sort"
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
		types.ChangeGodAddressTx:   "changeGodAddress",
		types.BurnTx:               "burn",
		types.ChangeProfileTx:      "changeProfile",
		types.DeleteFlipTx:         "deleteFlip",
	}
)

type BlockchainApi struct {
	bc      *blockchain.Blockchain
	baseApi *BaseApi
	ipfs    ipfs.Proxy
	pool    *mempool.TxPool
	d       *protocol.Downloader
	pm      *protocol.IdenaGossipHandler
}

func NewBlockchainApi(baseApi *BaseApi, bc *blockchain.Blockchain, ipfs ipfs.Proxy, pool *mempool.TxPool, d *protocol.Downloader, pm *protocol.IdenaGossipHandler) *BlockchainApi {
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
	Hash      common.Hash     `json:"hash"`
	Type      string          `json:"type"`
	From      common.Address  `json:"from"`
	To        *common.Address `json:"to"`
	Amount    decimal.Decimal `json:"amount"`
	Tips      decimal.Decimal `json:"tips"`
	MaxFee    decimal.Decimal `json:"maxFee"`
	Nonce     uint32          `json:"nonce"`
	Epoch     uint16          `json:"epoch"`
	Payload   hexutil.Bytes   `json:"payload"`
	BlockHash common.Hash     `json:"blockHash"`
	UsedFee   decimal.Decimal `json:"usedFee"`
	Timestamp uint64          `json:"timestamp"`
}

type BurntCoins struct {
	Address common.Address  `json:"address"`
	Amount  decimal.Decimal `json:"amount"`
	Key     string          `json:"key"`
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

	var blockHash common.Hash
	var feePerByte *big.Int
	var timestamp uint64
	if idx != nil {
		blockHash = idx.BlockHash
		block := api.bc.GetBlock(blockHash)
		if block != nil {
			feePerByte = block.Header.FeePerByte()
			timestamp = block.Header.Time().Uint64()
		}
	}
	return convertToTransaction(tx, blockHash, feePerByte, timestamp)
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
	isSyncing := api.d.IsSyncing() || !api.pm.HasPeers() || !api.baseApi.engine.Synced()
	if api.bc.Config().Consensus.Automine {
		isSyncing = false
	}
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

type TransactionsArgs struct {
	Address common.Address `json:"address"`
	Count   int            `json:"count"`
	Token   hexutil.Bytes  `json:"token"`
}

type Transactions struct {
	Transactions []*Transaction `json:"transactions"`
	Token        *hexutil.Bytes `json:"token"`
}

// sorted by epoch \ nonce desc (the newest transactions are first)
func (api *BlockchainApi) PendingTransactions(args TransactionsArgs) Transactions {
	txs := api.pool.GetPendingByAddress(args.Address)

	sort.SliceStable(txs, func(i, j int) bool {
		if txs[i].Epoch > txs[j].Epoch {
			return true
		}
		if txs[i].Epoch < txs[j].Epoch {
			return false
		}
		return txs[i].AccountNonce > txs[j].AccountNonce
	})

	var list []*Transaction
	for _, item := range txs {
		list = append(list, convertToTransaction(item, common.Hash{}, nil, 0))
	}

	return Transactions{
		Transactions: list,
		Token:        nil,
	}
}

func (api *BlockchainApi) FeePerByte() *big.Int {
	return api.baseApi.getAppState().State.FeePerByte()
}

func (api *BlockchainApi) SendRawTx(ctx context.Context, bytesTx hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := rlp.DecodeBytes(bytesTx, &tx); err != nil {
		return common.Hash{}, err
	}

	return api.baseApi.sendInternalTx(ctx, &tx)
}

func (api *BlockchainApi) GetRawTx(args SendTxArgs) (hexutil.Bytes, error) {
	var payload []byte
	if args.Payload != nil {
		payload = *args.Payload
	}

	tx := api.baseApi.getTx(args.From, args.To, args.Type, args.Amount, args.MaxFee, args.Tips, args.Nonce, args.Epoch, payload)

	data, err := rlp.EncodeToBytes(tx)

	if err != nil {
		return nil, err
	}

	return data, nil
}

func (api *BlockchainApi) Transactions(args TransactionsArgs) Transactions {

	txs, nextToken := api.bc.ReadTxs(args.Address, args.Count, args.Token)

	var list []*Transaction
	for _, item := range txs {
		list = append(list, convertToTransaction(item.Tx, item.BlockHash, item.FeePerByte, item.Timestamp))
	}

	var token *hexutil.Bytes
	if nextToken != nil {
		b := hexutil.Bytes(nextToken)
		token = &b
	}

	return Transactions{
		Transactions: list,
		Token:        token,
	}
}

func (api *BlockchainApi) BurntCoins() []BurntCoins {
	var res []BurntCoins
	for _, bc := range api.bc.ReadTotalBurntCoins() {
		res = append(res, BurntCoins{
			Address: bc.Address,
			Amount:  blockchain.ConvertToFloat(bc.Amount),
			Key:     bc.Key,
		})
	}
	return res
}

func convertToTransaction(tx *types.Transaction, blockHash common.Hash, feePerByte *big.Int, timestamp uint64) *Transaction {
	sender, _ := types.Sender(tx)
	return &Transaction{
		Hash:      tx.Hash(),
		Epoch:     tx.Epoch,
		Payload:   tx.Payload,
		Amount:    blockchain.ConvertToFloat(tx.Amount),
		MaxFee:    blockchain.ConvertToFloat(tx.MaxFee),
		Tips:      blockchain.ConvertToFloat(tx.Tips),
		From:      sender,
		Nonce:     tx.AccountNonce,
		To:        tx.To,
		Type:      txTypeMap[tx.Type],
		BlockHash: blockHash,
		Timestamp: timestamp,
		UsedFee:   blockchain.ConvertToFloat(fee.CalculateFee(1, feePerByte, tx)),
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
		coinbase = block.Header.Coinbase()
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
