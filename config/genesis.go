package config

import (
	"github.com/idena-network/idena-go/common"
	"math/big"
)

type GenesisAllocation struct {
	Balance *big.Int
	Stake   *big.Int
	State   uint8
}

type GenesisConf struct {
	Alloc             map[common.Address]GenesisAllocation
	GodAddress        common.Address
	FirstCeremonyTime int64
	GodAddressInvites uint16
}
