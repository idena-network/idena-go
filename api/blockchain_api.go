package api

import (
	"context"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/core/mempool"
	state2 "github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/keywords"
	"github.com/idena-network/idena-go/protocol"
	"github.com/idena-network/idena-go/rlp"
	"github.com/idena-network/idena-go/state"
	"github.com/idena-network/idena-go/vm"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"math/big"
	"sort"
	"sync"
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
		types.DeployContractTx:     "deployContract",
		types.CallContractTx:       "callContract",
		types.TerminateContractTx:  "terminateContract",
		types.DelegateTx:           "delegate",
		types.UndelegateTx:         "undelegate",
		types.KillDelegatorTx:      "killDelegator",
		types.StoreToIpfsTx:        "storeToIpfs",
		types.ReplenishStakeTx:     "replenishStake",
	}
)

type BlockchainApi struct {
	bc              *blockchain.Blockchain
	baseApi         *BaseApi
	ipfs            ipfs.Proxy
	pool            *mempool.TxPool
	d               *protocol.Downloader
	pm              *protocol.IdenaGossipHandler
	nodeState       *state.NodeState
	burntCoins      *burntCoinsCache
	burntCoinsMutex sync.Mutex
}

type burntCoinsCache struct {
	version int64
	value   []BurntCoins
}

func newBurntCoinsCache() *burntCoinsCache {
	return &burntCoinsCache{}
}

func (c *burntCoinsCache) tryGet(version int64) ([]BurntCoins, bool) {
	if c.version != version {
		return nil, false
	}
	return c.value, true
}

func (c *burntCoinsCache) put(version int64, value []BurntCoins) {
	c.version = version
	c.value = value
}

func NewBlockchainApi(baseApi *BaseApi, bc *blockchain.Blockchain, ipfs ipfs.Proxy, pool *mempool.TxPool, d *protocol.Downloader, pm *protocol.IdenaGossipHandler, nodeState *state.NodeState) *BlockchainApi {
	return &BlockchainApi{bc, baseApi, ipfs, pool, d, pm, nodeState, newBurntCoinsCache(), sync.Mutex{}}
}

type Block struct {
	Coinbase     common.Address  `json:"coinbase"`
	Hash         common.Hash     `json:"hash"`
	ParentHash   common.Hash     `json:"parentHash"`
	Height       uint64          `json:"height"`
	Time         int64           `json:"timestamp"`
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
	Timestamp int64           `json:"timestamp"`
}

type BurntCoins struct {
	Address common.Address  `json:"address"`
	Amount  decimal.Decimal `json:"amount"`
	Key     string          `json:"key"`
}

type EstimateRawTxResponse struct {
	Receipt *TxReceipt      `json:"receipt"`
	TxHash  common.Hash     `json:"txHash"`
	TxFee   decimal.Decimal `json:"txFee"`
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
	var feePerGas *big.Int
	var timestamp int64
	if idx != nil {
		blockHash = idx.BlockHash
		block := api.bc.GetBlock(blockHash)
		if block != nil {
			feePerGas = block.Header.FeePerGas()
			timestamp = block.Header.Time()
		}
	}
	return convertToTransaction(tx, blockHash, feePerGas, timestamp)
}

func (api *BlockchainApi) TxReceipt(hash common.Hash) *TxReceipt {
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
	var feePerGas *big.Int
	if idx != nil {
		blockHash = idx.BlockHash
		block := api.bc.GetBlock(blockHash)
		if block != nil {
			feePerGas = block.Header.FeePerGas()
		}
	}

	receipt := api.bc.GetReceipt(hash)

	return convertReceipt(tx, receipt, feePerGas)
}

func (api *BlockchainApi) Mempool() []common.Hash {
	pending := api.pool.GetPendingTransaction(true, true, common.MultiShard, false)

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
	GenesisBlock uint64 `json:"genesisBlock"`
	Message      string `json:"message"`
}

func (api *BlockchainApi) Syncing() Syncing {
	isSyncing := api.d.IsSyncing() || !api.pm.HasPeers() || !api.baseApi.engine.Synced() || api.nodeState.Syncing()
	if api.bc.Config().Consensus.Automine {
		isSyncing = false
	}

	current, highest := api.d.SyncProgress()
	if !isSyncing {
		highest = current
	}
	return Syncing{
		Syncing:      isSyncing,
		GenesisBlock: api.bc.GenesisInfo().Genesis.Height(),
		CurrentBlock: current,
		HighestBlock: highest,
		WrongTime:    api.pm.WrongTime(),
		Message:      api.nodeState.Info(),
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

type EstimateTxResponse struct {
	TxHash common.Hash     `json:"txHash"`
	TxFee  decimal.Decimal `json:"txFee"`
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

func (api *BlockchainApi) FeePerGas() *big.Int {
	return api.baseApi.getReadonlyAppState().State.FeePerGas()
}

func (api *BlockchainApi) SendRawTx(ctx context.Context, bytesTx hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.FromBytes(bytesTx); err != nil {
		//TODO: remove later
		if err := rlp.DecodeBytes(bytesTx, &tx); err != nil {
			return common.Hash{}, err
		} else {
			tx.UseRlp = true
		}
	}

	return api.baseApi.sendInternalTx(ctx, &tx)
}

func (api *BlockchainApi) GetRawTx(args SendTxArgs) (hexutil.Bytes, error) {
	var payload []byte
	if args.Payload != nil {
		payload = *args.Payload
	}

	tx := api.baseApi.getTx(args.From, args.To, args.Type, args.Amount, args.MaxFee, args.Tips, args.Nonce, args.Epoch, payload)

	var data []byte
	var err error
	if args.UseProto {
		data, err = tx.ToBytes()
	} else {
		data, err = rlp.EncodeToBytes(tx)
	}

	if err != nil {
		return nil, err
	}

	return data, nil
}

func (api *BlockchainApi) EstimateRawTx(bytesTx hexutil.Bytes, from *common.Address) (*EstimateRawTxResponse, error) {
	tx := new(types.Transaction)
	if err := tx.FromBytes(bytesTx); err != nil {
		return nil, err
	}
	if from == nil && !tx.Signed() {
		return nil, errors.New("either sender or signature should be specified")
	}
	if from != nil && tx.Signed() {
		return nil, errors.New("sender and signature can't be specified at the same time")
	}
	if tx.Signed() {
		if err := api.baseApi.txpool.Validate(tx); err != nil {
			return nil, err
		}
	}

	response := &EstimateRawTxResponse{
		TxFee: blockchain.ConvertToFloat(fee.CalculateFee(1, api.baseApi.getReadonlyAppState().State.FeePerGas(), tx)),
	}
	if tx.Signed() {
		response.TxHash = tx.Hash()
	}

	if tx.Type == types.CallContractTx || tx.Type == types.DeployContractTx || tx.Type == types.TerminateContractTx {
		appState := api.baseApi.getAppStateForCheck()
		vm := vm.NewVmImpl(appState, api.bc, api.bc.Head, nil, api.bc.Config())
		if tx.Type == types.CallContractTx && !common.ZeroOrNil(tx.Amount) {
			var sender common.Address
			if tx.Signed() {
				sender, _ = types.Sender(tx)
			} else {
				sender = *from
			}
			appState.State.SubBalance(sender, tx.Amount)
			appState.State.AddBalance(*tx.To, tx.Amount)
		}
		r := vm.Run(tx, from, -1, true)
		r.GasCost = api.bc.GetGasCost(appState, r.GasUsed)
		receipt := convertEstimatedReceipt(tx, r, appState.State.FeePerGas())
		response.Receipt = receipt
		response.TxFee = response.TxFee.Add(receipt.GasCost)
	}

	return response, nil
}

func (api *BlockchainApi) EstimateTx(args SendTxArgs) (*EstimateTxResponse, error) {
	var payload []byte
	if args.Payload != nil {
		payload = *args.Payload
	}

	tx, err := api.baseApi.getSignedTx(args.From, args.To, args.Type, args.Amount, args.MaxFee, args.Tips, args.Nonce, args.Epoch, payload, nil)
	if err != nil {
		return nil, err
	}
	err = api.baseApi.txpool.Validate(tx)
	if err != nil {
		return nil, err
	}

	return &EstimateTxResponse{
		TxHash: tx.Hash(),
		TxFee:  blockchain.ConvertToFloat(fee.CalculateFee(1, api.baseApi.getReadonlyAppState().State.FeePerGas(), tx)),
	}, nil
}

func (api *BlockchainApi) Transactions(args TransactionsArgs) Transactions {

	txs, nextToken := api.bc.ReadTxs(args.Address, args.Count, args.Token)

	var list []*Transaction
	for _, item := range txs {
		list = append(list, convertToTransaction(item.Tx, item.BlockHash, item.FeePerGas, item.Timestamp))
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

	appState := api.baseApi.getReadonlyAppState()

	if value, ok := api.burntCoins.tryGet(appState.State.Version()); ok {
		return value
	}

	api.burntCoinsMutex.Lock()
	defer api.burntCoinsMutex.Unlock()

	if value, ok := api.burntCoins.tryGet(appState.State.Version()); ok {
		return value
	}

	type burntCoins struct {
		Address common.Address
		Key     string
		Amount  *big.Int
	}
	var list []*burntCoins
	var coinsByAddressAndKey map[common.Address]map[string]*burntCoins

	// TODO temporary code to support burnt coins from local repo
	for _, value := range api.bc.ReadTotalBurntCoins() {
		address, key, amount := value.Address, value.Key, value.Amount
		if common.ZeroOrNil(amount) {
			continue
		}
		if coinsByAddressAndKey == nil {
			coinsByAddressAndKey = make(map[common.Address]map[string]*burntCoins)
		}
		if addressCoinsByKey, ok := coinsByAddressAndKey[address]; ok {
			if total, ok := addressCoinsByKey[key]; ok {
				total.Amount = new(big.Int).Add(total.Amount, amount)
			} else {
				bc := &burntCoins{address, key, amount}
				addressCoinsByKey[key] = bc
				list = append(list, bc)
			}
		} else {
			bc := &burntCoins{address, key, amount}
			addressCoinsByKey = make(map[string]*burntCoins)
			addressCoinsByKey[key] = bc
			coinsByAddressAndKey[address] = addressCoinsByKey
			list = append(list, bc)
		}
	}

	appState.State.IterateBurntCoins(func(height uint64, value state2.BurntCoins) {
		for _, bc := range value.Items {
			address, key, amount := bc.Address, bc.Key, bc.Amount
			if common.ZeroOrNil(amount) {
				continue
			}
			if coinsByAddressAndKey == nil {
				coinsByAddressAndKey = make(map[common.Address]map[string]*burntCoins)
			}
			if addressCoinsByKey, ok := coinsByAddressAndKey[address]; ok {
				if total, ok := addressCoinsByKey[key]; ok {
					total.Amount = new(big.Int).Add(total.Amount, amount)
				} else {
					bc := &burntCoins{address, key, amount}
					addressCoinsByKey[key] = bc
					list = append(list, bc)
				}
			} else {
				bc := &burntCoins{address, key, amount}
				addressCoinsByKey = make(map[string]*burntCoins)
				addressCoinsByKey[key] = bc
				coinsByAddressAndKey[address] = addressCoinsByKey
				list = append(list, bc)
			}
		}
	})

	res := make([]BurntCoins, 0, len(list))
	for _, bc := range list {
		res = append(res, BurntCoins{
			Address: bc.Address,
			Amount:  blockchain.ConvertToFloat(bc.Amount),
			Key:     bc.Key,
		})
	}
	api.burntCoins.put(appState.State.Version(), res)
	return res
}

func convertToTransaction(tx *types.Transaction, blockHash common.Hash, feePerGas *big.Int, timestamp int64) *Transaction {
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
		UsedFee:   blockchain.ConvertToFloat(fee.CalculateFee(1, feePerGas, tx)),
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

func (api *BlockchainApi) KeyWord(index int) (keywords.Keyword, error) {
	return keywords.Get(index)
}
