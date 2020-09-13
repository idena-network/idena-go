package embedded

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/pkg/errors"
)

type OracleLock struct {
	*BaseContract
}

func NewOracleLock(ctx env.CallContext, e env.Env) *OracleLock {
	return &OracleLock{&BaseContract{
		ctx: ctx,
		env: e,
	}}
}

func (e *OracleLock) Deploy(args ...[]byte) error {
	if oracleVotingAddr, err := helpers.ExtractAddr(0, args...); err != nil {
		return err
	} else {
		e.SetArray("oracleVotingAddr", oracleVotingAddr.Bytes())
	}
	if value, err := helpers.ExtractByte(1, args...); err != nil {
		return err
	} else {
		e.SetByte("value", value)
	}

	if successAddr, err := helpers.ExtractAddr(2, args...); err != nil {
		return err
	} else {
		e.SetArray("successAddr", successAddr.Bytes())
	}

	if failAddr, err := helpers.ExtractAddr(3, args...); err != nil {
		return err
	} else {
		e.SetArray("failAddr", failAddr.Bytes())
	}

	e.BaseContract.Deploy(EvidenceLockContract)
	e.SetOwner(e.ctx.Sender())
	return nil
}

func (e *OracleLock) Call(method string, args ...[]byte) error {
	switch method {
	case "push":
		return e.push(args...)
	default:
		return errors.New("unknown method")
	}
}

func (e *OracleLock) Read(method string, args ...[]byte) ([]byte, error) {
	panic("implement me")
}

func (e *OracleLock) push(args ...[]byte) error {
	var oracleVotingAddr common.Address
	oracleVotingAddr.SetBytes(e.GetArray("oracleVotingAddr"))

	state, _ := helpers.ExtractUInt64(0, e.env.ReadContractData(oracleVotingAddr, []byte("state")))
	if state != 2 {
		return errors.New("voting is not completed")
	}
	expected := e.GetByte("value")

	votedValue, err := helpers.ExtractByte(0, e.env.ReadContractData(oracleVotingAddr, []byte("result")))
	if err != nil || expected != votedValue {
		var dest common.Address
		dest.SetBytes(e.GetArray("failAddr"))
		e.env.Send(e.ctx, dest, e.env.Balance(e.ctx.ContractAddr()))
	} else {
		var dest common.Address
		dest.SetBytes(e.GetArray("successAddr"))
		e.env.Send(e.ctx, dest, e.env.Balance(e.ctx.ContractAddr()))
	}
	return nil
}

func (e *OracleLock) Terminate(args ...[]byte) error {
	if !e.IsOwner() {
		return errors.New("sender is not an owner")
	}
	balance := e.env.Balance(e.ctx.ContractAddr())
	if balance.Sign() > 0 {
		return errors.New("contract has dna")
	}
	dest, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return err
	}
	e.env.Terminate(e.ctx, dest)
	return nil
}
