package state

import (
	"crypto/rand"
	"github.com/idena-network/idena-go/common"
)

func getRandAddr() common.Address {
	bytes := make([]byte, 20)
	rand.Read(bytes)
	return common.BytesToAddress(bytes)
}

