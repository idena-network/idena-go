package api

import (
	"crypto/ecdsa"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/consensus"
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

func (api *BaseApi) getCurrentCoinbase() common.Address {
	return crypto.PubkeyToAddress(*api.engine.GetKey().Public().(*ecdsa.PublicKey))
}

func (api *BaseApi) sendTx(from common.Address, to common.Address, txType types.TxType, amount *big.Float, nonce uint32, epoch uint16, payload []byte, key *ecdsa.PrivateKey) (common.Hash, error) {
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
		tx.AccountNonce = api.engine.GetAppState().NonceCache.GetNonce(from) + 1
	}

	signedTx, err := api.signTransaction(from, &tx, key)

	if err != nil {
		return common.Hash{}, err
	}

	if err := api.txpool.Add(signedTx); err != nil {
		return common.Hash{}, err
	}

	return signedTx.Hash(), nil
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
