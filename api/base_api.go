package api

import (
	"context"
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/consensus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/keystore"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/secstore"
	"github.com/shopspring/decimal"
)

type BaseApi struct {
	engine   *consensus.Engine
	txpool   *mempool.TxPool
	ks       *keystore.KeyStore
	secStore *secstore.SecStore
	ipfs     ipfs.Proxy
}

type BaseTxArgs struct {
	Nonce uint32 `json:"nonce"`
	Epoch uint16 `json:"epoch"`
}

func NewBaseApi(engine *consensus.Engine, txpool *mempool.TxPool, ks *keystore.KeyStore, secStore *secstore.SecStore, ipfs ipfs.Proxy) *BaseApi {
	return &BaseApi{engine, txpool, ks, secStore, ipfs}
}

func (api *BaseApi) getReadonlyAppState() *appstate.AppState {
	state, err := api.engine.ReadonlyAppState()
	if err != nil {
		panic(err)
	}
	return state
}

func (api *BaseApi) getAppStateForCheck() *appstate.AppState {
	state, err := api.engine.AppStateForCheck()
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

	state := api.getReadonlyAppState()
	return blockchain.BuildTxWithFeeEstimating(state, from, to, txType, amount, maxFee, tips, nonce, epoch, payload)
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

	if err := api.txpool.AddInternalTx(tx); err != nil {
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

func (api *BaseApi) getCoinbaseShard() common.ShardId {
	state := api.getReadonlyAppState()
	return state.State.ShardId(api.secStore.GetAddress())
}

func (api *BaseApi) canSign(address common.Address) bool {
	if address == api.getCurrentCoinbase() {
		return true
	}
	_, err := api.ks.Find(keystore.Account{Address: address})
	return err == nil
}
