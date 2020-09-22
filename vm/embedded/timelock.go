package embedded

import (
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/pkg/errors"
)

type TimeLock struct {
	*BaseContract
}

func NewTimeLock(ctx env.CallContext, env env.Env) *TimeLock {
	return &TimeLock{
		&BaseContract{
			ctx: ctx,
			env: env,
		},
	}
}

func (t *TimeLock) Deploy(args ...[]byte) error {
	if time, err := helpers.ExtractUInt64(0, args...); err != nil {
		return err
	} else {
		t.SetUint64("timestamp", time)
	}

	t.BaseContract.Deploy(TimeLockContract)
	t.SetOwner(t.ctx.Sender())
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
	return nil
}

func (t *TimeLock) Terminate(args ...[]byte) error {
	if !t.IsOwner() {
		return errors.New("sender is not an owner")
	}
	if uint64(t.env.BlockTimeStamp()) < t.GetUint64("timestamp") {
		return errors.New("terminate is locked")
	}
	balance := t.env.Balance(t.ctx.ContractAddr())
	if balance.Sign() > 0 {
		return errors.New("contract has dna")
	}
	dest, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return err
	}
	t.env.Terminate(t.ctx, dest)
	return nil
}
