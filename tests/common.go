package tests

import (
	"github.com/google/tink/go/subtle/random"
	"github.com/idena-network/idena-go/common"
)

func GetRandAddr() common.Address {
	addr := common.Address{}
	addr.SetBytes(random.GetRandomBytes(20))
	return addr
}
