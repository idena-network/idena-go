package api

import (
	"crypto/ecdsa"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/consensus"
	"idena-go/core/appstate"
	"idena-go/core/mempool"
	"idena-go/crypto"
	"idena-go/keystore"
	"math/big"
)

type BaseApi struct {
	engine *consensus.Engine
	txpool *mempool.TxPool
	ks     *keystore.KeyStore
}

func NewBaseApi(engine *consensus.Engine, txpool *mempool.TxPool, ks *keystore.KeyStore) *BaseApi {
	return &BaseApi{engine, txpool, ks}
}

func (api *BaseApi) getAppState() *appstate.AppState {
	return api.engine.GetAppState()
}

func (api *BaseApi) getCurrentCoinbase() common.Address {
	return crypto.PubkeyToAddress(*api.engine.GetKey().Public().(*ecdsa.PublicKey))
}

func (api *BaseApi) getTx(from common.Address, to common.Address, txType types.TxType, amount *big.Float, nonce uint32, epoch uint16, payload []byte, key *ecdsa.PrivateKey) *types.Transaction {
	tx := types.Transaction{
		AccountNonce: nonce,
		Type:         txType,
		To:           &to,
		Amount:       convertToInt(amount),
		Payload:      payload,
		Epoch:        epoch,
	}

	s := api.engine.GetAppState().State

	if tx.Epoch == 0 {
		tx.Epoch = s.Epoch()
	}

	if tx.AccountNonce == 0 && tx.Epoch == s.Epoch() {
		currentNonce := api.engine.GetAppState().NonceCache.GetNonce(from)
		// if epoch was increased, we should reset nonce to 1
		if s.GetEpoch(from) < s.Epoch() {
			//TODO: should not be zero if we switched to new epoch
			currentNonce = 0
		}
		tx.AccountNonce = currentNonce + 1
	}

	return &tx
}

func (api *BaseApi) getSignedTx(from common.Address, to common.Address, txType types.TxType, amount *big.Float, nonce uint32, epoch uint16, payload []byte, key *ecdsa.PrivateKey) (*types.Transaction, error) {
	tx := api.getTx(from, to, txType, amount, nonce, epoch, payload, key)

	return api.signTransaction(from, tx, key)
}

func (api *BaseApi) sendTx(from common.Address, to common.Address, txType types.TxType, amount *big.Float, nonce uint32, epoch uint16, payload []byte, key *ecdsa.PrivateKey) (common.Hash, error) {

	tx := api.getTx(from, to, txType, amount, nonce, epoch, payload, key)

	signedTx, err := api.signTransaction(from, tx, key)

	if err != nil {
		return common.Hash{}, err
	}

	return api.sendInternalTx(signedTx)
}

func (api *BaseApi) sendInternalTx(tx *types.Transaction) (common.Hash, error) {
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
		return types.SignTx(tx, api.engine.GetKey())
	}
	account, err := api.ks.Find(keystore.Account{Address: from})
	if err != nil {
		return nil, err
	}
	return api.ks.SignTx(account, tx)
}
