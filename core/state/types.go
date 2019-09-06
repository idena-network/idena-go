package state

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"math/big"
)

type StateAccount struct {
	Address common.Address
	Nonce   uint32
	Epoch   uint16
	Balance *big.Int
}

type StateIdentityFlip struct {
	Cid  []byte
	Pair uint8
}

type StateIdentity struct {
	Address         common.Address
	Nickname        *[64]byte `rlp:"nil"`
	Stake           *big.Int
	Invites         uint8
	Birthday        uint16
	State           IdentityState
	QualifiedFlips  uint32
	ShortFlipPoints uint32
	PubKey          []byte `rlp:"nil"`
	RequiredFlips   uint8
	Flips           []StateIdentityFlip `rlp:"nil"`
	Generation      uint32
	Code            []byte   `rlp:"nil"`
	Invitees        []TxAddr `rlp:"nil"`
	Inviter         *TxAddr  `rlp:"nil"`
	Penalty         *big.Int
}

type StateApprovedIdentity struct {
	Address  common.Address
	Approved bool
	Online   bool
}

type StateGlobal struct {
	Epoch              uint16
	NextValidationTime *big.Int
	ValidationPeriod   ValidationPeriod
	GodAddress         common.Address
	WordsSeed          types.Seed `rlp:"nil"`
	LastSnapshot       uint64
}

type PredefinedState struct {
	Block              uint64
	Seed               types.Seed
	Global             StateGlobal
	Accounts           []*StateAccount
	Identities         []*StateIdentity
	ApprovedIdentities []*StateApprovedIdentity
}
