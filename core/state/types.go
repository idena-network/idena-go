package state

import (
	"github.com/idena-network/idena-go/common"
	"math/big"
)

type StateAccount struct {
	Address common.Address
	Nonce   uint32
	Epoch   uint16
	Balance *big.Int
}

type StateIdentity struct {
	Address common.Address
	Nickname       *[64]byte `rlp:"nil"`
	Stake          *big.Int
	Invites        uint8
	Age            uint16
	State          IdentityState
	QualifiedFlips uint32
	ShortFlipPoints uint32
	PubKey          []byte `rlp:"nil"`
	RequiredFlips   uint8
	Flips           [][]byte `rlp:"nil"`
	Generation      uint32
	Code            []byte   `rlp:"nil"`
}

type StateApprovedIdentity struct {
	Address common.Address
	Approved bool
	Online   bool
}

type PredefinedState struct {
	Accounts           []*StateAccount
	Identities         []*StateIdentity
	ApprovedIdentities []*StateApprovedIdentity
}
