package tests

import (
	"github.com/google/tink/go/subtle/random"
	"github.com/idena-network/idena-go/common"
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
