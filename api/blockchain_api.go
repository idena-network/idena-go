package api

import (
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-cid"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/hexutil"
	"idena-go/ipfs"
	"math/big"
)

var (
	txTypeMap = map[types.TxType]string{
		types.RegularTx:    "regular",
		types.ActivationTx: "activation",
		types.InviteTx:     "invite",
		types.KillTx:       "kill",
		types.SubmitFlipTx: "flipSubmit",
	}
)

type BlockchainApi struct {
	bc      *blockchain.Blockchain
	baseApi *BaseApi
	ipfs    ipfs.Proxy
}

func NewBlockchainApi(baseApi *BaseApi, bc *blockchain.Blockchain, ipfs ipfs.Proxy) *BlockchainApi {
	return &BlockchainApi{bc, baseApi, ipfs}
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
}

type Transaction struct {
	Type      string          `json:"type"`
	From      common.Address  `json:"from"`
	To        *common.Address `json:"to"`
	Amount    *big.Float      `json:"amount"`
	Nonce     uint32          `json:"nonce"`
	Epoch     uint16          `json:"epoch"`
	Payload   hexutil.Bytes   `json:"payload"`
	BlockHash common.Hash
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
		return nil
	}
	sender, _ := types.Sender(tx)

	return &Transaction{
		Epoch:     tx.Epoch,
		Payload:   hexutil.Bytes(tx.Payload),
		Amount:    convertToFloat(tx.Amount),
		From:      sender,
		Nonce:     tx.AccountNonce,
		To:        tx.To,
		Type:      txTypeMap[tx.Type],
		BlockHash: idx.BlockHash,
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

	return &Block{
		Hash:         block.Hash(),
		IdentityRoot: block.IdentityRoot(),
		Root:         block.Root(),
		Height:       block.Height(),
		ParentHash:   block.Header.ParentHash(),
		Time:         block.Header.Time(),
		IpfsHash:     ipfsHashStr,
		Transactions: txs,
	}
}
