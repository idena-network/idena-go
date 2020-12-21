package embedded

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/pkg/errors"
)

type OracleLock struct {
	*BaseContract
}

func NewOracleLock(ctx env.CallContext, e env.Env, statsCollector collector.StatsCollector) *OracleLock {
	return &OracleLock{&BaseContract{
		ctx:            ctx,
		env:            e,
		statsCollector: statsCollector,
	}}
}

func (e *OracleLock) Deploy(args ...[]byte) error {
	oracleVotingAddr, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return err
	}
	e.SetArray("oracleVotingAddr", oracleVotingAddr.Bytes())

	value, err := helpers.ExtractByte(1, args...)
	if err != nil {
		return err
	}
	e.SetByte("value", value)

	successAddr, err := helpers.ExtractAddr(2, args...)
	if err != nil {
		return err
	}
	e.SetArray("successAddr", successAddr.Bytes())

	failAddr, err := helpers.ExtractAddr(3, args...)
	if err != nil {
		return err
	}
	e.SetArray("failAddr", failAddr.Bytes())

	e.SetOwner(e.ctx.Sender())
	collector.AddOracleLockDeploy(e.statsCollector, e.ctx.ContractAddr(), oracleVotingAddr, value, successAddr, failAddr)
	return nil
}

func (e *OracleLock) Call(method string, args ...[]byte) error {
	switch method {
	case "push":
		return e.push(args...)
	case "checkOracleVoting":
		return e.checkOracleVoting(args...)
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

	expected := e.GetByte("value")
	hasVotedValue := e.GetByte("hasVotedValue") == 1
	votedValue := e.GetByte("voted")

	if hasVotedValue {
		amount := e.env.Balance(e.ctx.ContractAddr())
		if expected != votedValue {
			var dest common.Address
			dest.SetBytes(e.GetArray("failAddr"))
			e.env.Send(e.ctx, dest, amount)
			collector.AddOracleLockCallPush(e.statsCollector, false, votedValue, nil, amount)
		} else {
			var dest common.Address
			dest.SetBytes(e.GetArray("successAddr"))
			e.env.Send(e.ctx, dest, amount)
			collector.AddOracleLockCallPush(e.statsCollector, true, votedValue, nil, amount)
		}
		return nil
	}
	return errors.New("oracle value is nil")
}

func (e *OracleLock) checkOracleVoting(args ...[]byte) error {
	var oracleVotingAddr common.Address
	oracleVotingAddr.SetBytes(e.GetArray("oracleVotingAddr"))
	state, _ := helpers.ExtractByte(0, e.env.ReadContractData(oracleVotingAddr, []byte("state")))
	if state != oracleVotingStateFinished {
		return errors.New("voting is not completed")
	}
	votedValue, err := helpers.ExtractByte(0, e.env.ReadContractData(oracleVotingAddr, []byte("result")))
	if err != nil {
		e.SetByte("voted", votedValue)
		e.SetByte("hasVotedValue", 1)
	}
	return nil
}

func (e *OracleLock) Terminate(args ...[]byte) (common.Address, error) {
	var oracleVoting common.Address
	oracleVoting.SetBytes(e.GetArray("oracleVotingAddr"))
	oracleVotingExist := e.env.ContractStake(oracleVoting) != nil && e.env.ContractStake(oracleVoting).Sign() > 0

	hasVotedValue := e.GetByte("hasVotedValue") == 1
	if hasVotedValue {
		balance := e.env.Balance(e.ctx.ContractAddr())
		if balance.Sign() > 0 {
			return common.Address{}, errors.New("contract has dna")
		}
		if oracleVotingExist {
			return common.Address{}, errors.New("oracle voting exists")
		}
		return e.Owner(), nil
	}
	if !e.IsOwner() {
		return common.Address{}, errors.New("sender is not an owner")
	}
	if oracleVotingExist {
		return common.Address{}, errors.New("oracle voting exists")
	}
	balance := e.env.Balance(e.ctx.ContractAddr())
	if balance.Sign() > 0 {
		e.env.BurnAll(e.ctx)
	}
	collector.AddOracleLockTermination(e.statsCollector, e.Owner())
	return e.Owner(), nil
}
