package env

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
)

func ComputeContractAddr(tx *types.Transaction, from common.Address) common.Address {
	hash := crypto.Hash(append(append(from.Bytes(), common.ToBytes(tx.Epoch)...), common.ToBytes(tx.AccountNonce)...))
	var result common.Address
	result.SetBytes(hash[:])
	return result
}