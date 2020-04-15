package api

import (
	"context"
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/fee"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/consensus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/keystore"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/secstore"
	"github.com/shopspring/decimal"
	"math/big"
)

type BaseApi struct {
	engine   *consensus.Engine
	txpool   *mempool.TxPool
	ks       *keystore.KeyStore
	secStore *secstore.SecStore
}

type BaseTxArgs struct {
	Nonce uint32 `json:"nonce"`
	Epoch uint16 `json:"epoch"`
}

func NewBaseApi(engine *consensus.Engine, txpool *mempool.TxPool, ks *keystore.KeyStore, secStore *secstore.SecStore) *BaseApi {
	return &BaseApi{engine, txpool, ks, secStore}
}

func (api *BaseApi) getAppState() *appstate.AppState {
	state, err := api.engine.ReadonlyAppState()
	if err != nil {
		panic(err)
	}
	return state
}

func (api *BaseApi) getCurrentCoinbase() common.Address {
	return api.secStore.GetAddress()
}

func (api *BaseApi) getTx(from common.Address, to *common.Address, txType types.TxType, amount decimal.Decimal,
	maxFee decimal.Decimal, tips decimal.Decimal, nonce uint32, epoch uint16, payload []byte) *types.Transaction {

	// if maxFee is not set, we set it as 2x from fee
	if maxFee == (decimal.Decimal{}) || maxFee == decimal.Zero {
		tx := blockchain.BuildTx(api.getAppState(), from, to, txType, amount, maxFee, tips, nonce, epoch, payload)
		txFee := fee.CalculateFee(api.getAppState().ValidatorsCache.NetworkSize(), api.getAppState().State.FeePerByte(), tx)
		maxFee = blockchain.ConvertToFloat(new(big.Int).Mul(txFee, big.NewInt(2)))
	}

	tx := blockchain.BuildTx(api.getAppState(), from, to, txType, amount, maxFee, tips, nonce, epoch, payload)

	return tx
}

func (api *BaseApi) getSignedTx(from common.Address, to *common.Address, txType types.TxType, amount decimal.Decimal,
	maxFee decimal.Decimal, tips decimal.Decimal, nonce uint32, epoch uint16, payload []byte,
	key *ecdsa.PrivateKey) (*types.Transaction, error) {

	tx := api.getTx(from, to, txType, amount, maxFee, tips, nonce, epoch, payload)

	return api.signTransaction(from, tx, key)
}

func (api *BaseApi) sendTx(ctx context.Context, from common.Address, to *common.Address, txType types.TxType, amount decimal.Decimal,
	maxFee decimal.Decimal, tips decimal.Decimal, nonce uint32, epoch uint16, payload []byte,
	key *ecdsa.PrivateKey) (common.Hash, error) {

	signedTx, err := api.getSignedTx(from, to, txType, amount, maxFee, tips, nonce, epoch, payload, key)

	if err != nil {
		return common.Hash{}, err
	}

	return api.sendInternalTx(ctx, signedTx)
}

func (api *BaseApi) sendInternalTx(ctx context.Context, tx *types.Transaction) (common.Hash, error) {
	log.Info("Sending new tx", "ip", ctx.Value("remote"), "type", tx.Type, "hash", tx.Hash().Hex(), "nonce", tx.AccountNonce, "epoch", tx.Epoch)

	if err := api.txpool.Add(tx); err != nil {
		return common.Hash{}, err
	}

	return tx.Hash(), nil
}

func (api *BaseApi) signTransaction(from common.Address, tx *types.Transaction, key *ecdsa.PrivateKey) (*types.Transaction, error) {
	if key != nil {
		return types.SignTx(tx, key)
	}
	if from == api.getCurrentCoinbase() {
		return api.secStore.SignTx(tx)
	}
	account, err := api.ks.Find(keystore.Account{Address: from})
	if err != nil {
		return nil, err
	}
	return api.ks.SignTx(account, tx)
}
