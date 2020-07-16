package embedded

import (
	"github.com/idena-network/idena-go/common"
	env2 "github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"math/big"
)

type EmbeddedContractType = common.Hash

const (
	ContractCallGas   = 100
	ContractDeployGas = 20
)

var (
	TimeLockContract     EmbeddedContractType
	FactEvidenceContract EmbeddedContractType
	EvidenceLockContract EmbeddedContractType

	AvailableContracts map[EmbeddedContractType]struct{}
)

func init() {
	TimeLockContract.SetBytes([]byte{0x1})
	FactEvidenceContract.SetBytes([]byte{0x2})
	EvidenceLockContract.SetBytes([]byte{0x3})

	AvailableContracts = map[EmbeddedContractType]struct{}{
		TimeLockContract:     {},
		FactEvidenceContract: {},
		EvidenceLockContract: {},
	}
}

type Contract interface {
	Deploy(args ...[]byte) error
	Call(method string, args ...[]byte) error
	Read(method string, args ...[]byte) ([]byte, error)
	Terminate(args ...[]byte) error
}

// base contract with useful common methods

type BaseContract struct {
	ctx env2.CallContext
	env env2.Env
}

func (b *BaseContract) SetOwner(address common.Address) {
	b.env.SetValue(b.ctx, []byte("owner"), address.Bytes())
}

func (b *BaseContract) Owner() common.Address {
	bytes := b.env.GetValue(b.ctx, []byte("owner"))
	var owner common.Address
	owner.SetBytes(bytes)
	return owner
}

func (b *BaseContract) Deploy(contractType EmbeddedContractType) {
	b.env.Deploy(b.ctx, contractType)
}

func (b *BaseContract) SetUint64(s string, value uint64) {
	b.env.SetValue(b.ctx, []byte(s), common.ToBytes(value))
}

func (b *BaseContract) GetUint64(s string) uint64 {
	data := b.env.GetValue(b.ctx, []byte(s))
	ret, _ := helpers.ExtractUInt64(0, data)
	return ret
}

func (b *BaseContract) SetArray(s string, cid []byte) {
	b.env.SetValue(b.ctx, []byte(s), cid)
}

func (b *BaseContract) GetArray(s string) []byte {
	return b.env.GetValue(b.ctx, []byte(s))
}

func (b *BaseContract) SetBigInt(s string, value *big.Int) {
	b.env.SetValue(b.ctx, []byte(s), value.Bytes())
}

func (b *BaseContract) GetBigInt(s string) *big.Int {
	data := b.env.GetValue(b.ctx, []byte(s))
	if data == nil {
		return nil
	}
	ret := new(big.Int)
	ret.SetBytes(data)
	return ret
}

func (b *BaseContract) SetByte(s string, value byte) {
	b.env.SetValue(b.ctx, []byte(s), []byte{value})
}

func (b *BaseContract) GetByte(s string) byte {
	data := b.env.GetValue(b.ctx, []byte(s))
	if len(data) == 0 {
		return 0
	}
	return data[0]
}

func (b *BaseContract) IsOwner() bool {
	return b.Owner() == b.ctx.Sender()
}
