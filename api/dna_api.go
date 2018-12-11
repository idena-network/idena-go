package api

import (
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/common/hexutil"
	"idena-go/consensus"
	"idena-go/core/mempool"
	"math/big"
)

type DnaApi struct {
	engine *consensus.Engine
	txpool *mempool.TxPool
}

func NewDnaApi(engine *consensus.Engine, txpool *mempool.TxPool) *DnaApi {
	return &DnaApi{engine, txpool}
}

type State struct {
	Name string `json:"name"`
}

func (api *DnaApi) State() State {
	return State{
		Name: api.engine.GetState(),
	}
}

// SendTxArgs represents the arguments to sumbit a new transaction into the transaction pool.
type SendTxArgs struct {
	Type   *hexutil.Bytes  `json:"type"`
	From   common.Address  `json:"from"`
	To     *common.Address `json:"to"`
	Amount *big.Int        `json:"value"`
	Nonce  uint64          `json:"nonce"`
	Input  *hexutil.Bytes  `json:"input"`
}

func (api *DnaApi) SendTransaction(args SendTxArgs) (common.Hash, error) {

	tx := types.Transaction{
		AccountNonce: args.Nonce,
		Type:         types.TxType((*args.Type)[0]),
		To:           args.To,
		Amount:       args.Amount,
		Payload:      *args.Input,
	}

	signedTx, err := types.SignTx(&tx, api.engine.GetKey())

	if err != nil {
		return common.Hash{}, err
	}

	if err := api.txpool.Add(signedTx); err != nil {
		return common.Hash{}, err
	}

	return signedTx.Hash(), nil
}
