package tests

import (
	"crypto/ecdsa"
	"github.com/google/tink/go/subtle/random"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"math/big"
)

func GetRandAddr() common.Address {
	addr := common.Address{}
	addr.SetBytes(random.GetRandomBytes(20))
	return addr
}

func getAmount(amount int64) *big.Int {
	return new(big.Int).Mul(common.DnaBase, big.NewInt(amount))
}

func GetTx(nonce uint32, epoch uint16, key *ecdsa.PrivateKey) *types.Transaction {
	return GetTypedTx(nonce, epoch, key, types.SendTx)
}

func GetTypesTxWithAmount(nonce uint32, epoch uint16, key *ecdsa.PrivateKey, txType types.TxType, amount *big.Int) *types.Transaction {

	addr := crypto.PubkeyToAddress(key.PublicKey)
	to := &addr

	if txType == types.EvidenceTx || txType == types.SubmitShortAnswersTx ||
		txType == types.SubmitLongAnswersTx || txType == types.SubmitAnswersHashTx || txType == types.SubmitFlipTx {
		to = nil
	}

	return GetFullTx(nonce, epoch, key, txType, amount, to, nil)
}

func GetTypedTx(nonce uint32, epoch uint16, key *ecdsa.PrivateKey, txType types.TxType) *types.Transaction {
	return GetTypesTxWithAmount(nonce, epoch, key, txType, new(big.Int))
}

func GetFullTx(nonce uint32, epoch uint16, key *ecdsa.PrivateKey, txType types.TxType, amount *big.Int,
	to *common.Address, payload []byte) *types.Transaction {

	tx := types.Transaction{
		AccountNonce: nonce,
		Type:         txType,
		To:           to,
		Amount:       amount,
		Epoch:        epoch,
		Payload:      payload,
		MaxFee:       new(big.Int).Mul(common.DnaBase, big.NewInt(20)),
	}

	signedTx, _ := types.SignTx(&tx, key)

	return signedTx
}
