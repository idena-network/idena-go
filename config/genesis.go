package config

import (
	"idena-go/common"
	"idena-go/core/state"
	"math/big"
)

type GenesisAllocation struct {
	Balance *big.Int
	Stake   *big.Int
	State   state.IdentityState
}

type GenesisConf struct {
	Alloc      map[common.Address]GenesisAllocation
	GodAddress common.Address
}
