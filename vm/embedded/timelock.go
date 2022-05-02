package embedded

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/pkg/errors"
	"math/big"
)

type TimeLock struct {
	*BaseContract
}

func NewTimeLock(ctx env.CallContext, env env.Env, statsCollector collector.StatsCollector) *TimeLock {
	return &TimeLock{
		&BaseContract{
			ctx:            ctx,
			env:            env,
			statsCollector: statsCollector,
		},
	}
}

func (t *TimeLock) Deploy(args ...[]byte) error {
	time, err := helpers.ExtractUInt64(0, args...)
	if err != nil {
		return err
	}
	t.SetUint64("timestamp", time)
	t.SetOwner(t.ctx.Sender())
	collector.AddTimeLockDeploy(t.statsCollector, t.ctx.ContractAddr(), time)
	return nil
}

func (t *TimeLock) Call(method string, args ...[]byte) (err error) {
	switch method {
	case "transfer":
		return t.transfer(args...)
	default:
		return errors.New("unknown method")
	}
}

func (t *TimeLock) Read(method string, args ...[]byte) ([]byte, error) {
	panic("implement me")
}

func (t *TimeLock) transfer(args ...[]byte) (err error) {
	if !t.IsOwner() {
		return errors.New("sender is not an owner")
	}
	dest, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return err
	}
	amount, err := helpers.ExtractBigInt(1, args...)
	if err != nil {
		return err
	}

	if uint64(t.env.BlockTimeStamp()) < t.GetUint64("timestamp") {
		return errors.New("transfer is locked")
	}

	if err := t.env.Send(t.ctx, dest, amount); err != nil {
		return err
	}
	collector.AddTimeLockCallTransfer(t.statsCollector, dest, amount)
	return nil
}

func (t *TimeLock) Terminate(args ...[]byte) (common.Address, [][]byte, error) {
	if !t.IsOwner() {
		return common.Address{}, nil, errors.New("sender is not an owner")
	}
	if uint64(t.env.BlockTimeStamp()) < t.GetUint64("timestamp") {
		return common.Address{}, nil, errors.New("terminate is locked")
	}
	balance := t.env.Balance(t.ctx.ContractAddr())
	dust := big.NewInt(0).Mul(t.env.MinFeePerGas(), big.NewInt(100))
	if balance.Cmp(dust) > 0 {
		return common.Address{}, nil, errors.New("contract has dna")
	}
	if balance.Sign() > 0 {
		t.env.BurnAll(t.ctx)
	}
	dest, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return common.Address{}, nil, err
	}
	collector.AddTimeLockTermination(t.statsCollector, dest)
	return dest, nil, nil
}
