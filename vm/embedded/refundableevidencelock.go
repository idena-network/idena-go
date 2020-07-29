package embedded

import (
	"github.com/idena-network/idena-go/common"
	math2 "github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"math/big"
)

type RefundableEvidenceLock struct {
	*BaseContract
	deposits *env.Map
}

func NewRefundableEvidenceLock(ctx env.CallContext, e env.Env) *RefundableEvidenceLock {
	return &RefundableEvidenceLock{&BaseContract{
		ctx: ctx,
		env: e,
	}, env.NewMap([]byte("deposits"), e, ctx)}
}

func (e *RefundableEvidenceLock) Deploy(args ...[]byte) error {
	if factEvidenceAddr, err := helpers.ExtractAddr(0, args...); err != nil {
		return err
	} else {
		e.SetArray("factEvidenceAddr", factEvidenceAddr.Bytes())
	}
	if value, err := helpers.ExtractByte(1, args...); err != nil {
		return err
	} else {
		e.SetByte("value", value)
	}

	if successAddr, err := helpers.ExtractAddr(2, args...); err == nil {
		e.SetArray("successAddr", successAddr.Bytes())
	}

	if failAddr, err := helpers.ExtractAddr(3, args...); err == nil {
		e.SetArray("failAddr", failAddr.Bytes())
	}

	refundDelay := uint64(4320)
	if delay, err := helpers.ExtractUInt64(4, args...); err == nil {
		refundDelay = delay
	}
	e.SetUint64("refundDelay", refundDelay)

	factEvidenceFee := byte(0)
	if fee, err := helpers.ExtractByte(5, args...); err != nil {
		return err
	} else {
		factEvidenceFee = byte(math2.MaxInt(1, math2.MinInt(100, int(fee))))
	}
	e.SetByte("factEvidenceFee", factEvidenceFee)

	e.BaseContract.Deploy(RefundableEvidenceLockContract)
	e.SetOwner(e.ctx.Sender())

	e.SetByte("state", 1) // - locked
	e.SetBigInt("sum", big.NewInt(0))
	return nil
}

func (e *RefundableEvidenceLock) Call(method string, args ...[]byte) error {
	switch method {
	case "deposit":
		return e.deposit(args...)
	case "push":
		return e.push(args...)
	case "refund":
		return e.refund(args...)
	default:
		return errors.New("unknown method")
	}
}

func (e *RefundableEvidenceLock) Read(method string, args ...[]byte) ([]byte, error) {
	panic("implement me")
}

func (e *RefundableEvidenceLock) deposit(args ...[]byte) error {
	var factEvidenceAddr common.Address
	factEvidenceAddr.SetBytes(e.GetArray("factEvidenceAddr"))

	state, _ := helpers.ExtractUInt64(0, e.env.ReadContractData(factEvidenceAddr, []byte("state")))
	if state != 0 {
		return errors.New("voting is not in pending state")
	}

	minDeposit := big.NewInt(0).Mul(e.env.MinFeePerByte(), big.NewInt(10000))
	if e.ctx.PayAmount().Cmp(minDeposit) < 0 {
		return errors.New("amount is low")
	}
	balanceBytes := e.deposits.Get(e.ctx.Sender().Bytes())
	balance := big.NewInt(0)
	balance.SetBytes(balanceBytes)

	balance.Add(balance, e.ctx.PayAmount())

	e.deposits.Set(e.ctx.Sender().Bytes(), balance.Bytes())

	sum := e.GetBigInt("sum")
	sum.Add(sum, e.ctx.PayAmount())

	e.SetBigInt("sum", sum)

	feeRate := e.GetByte("factEvidenceFee")

	fee := decimal.NewFromBigInt(e.ctx.PayAmount(), 0)
	fee.Mul(decimal.NewFromInt(int64(feeRate))).Div(decimal.NewFromInt(100))

	err := e.env.Send(e.ctx, factEvidenceAddr, math2.ToInt(fee))
	if err != nil {
		return err
	}
	return nil
}

func (e *RefundableEvidenceLock) push(args ...[]byte) error {
	var factEvidenceAddr common.Address
	factEvidenceAddr.SetBytes(e.GetArray("factEvidenceAddr"))

	state, _ := helpers.ExtractUInt64(0, e.env.ReadContractData(factEvidenceAddr, []byte("state")))
	if state != 2 {
		return errors.New("voting is not completed")
	}
	expected := e.GetByte("value")

	var failAddr common.Address
	failAddr.SetBytes(e.GetArray("failAddr"))

	var successAddr common.Address
	successAddr.SetBytes(e.GetArray("successAddr"))

	votedValue, _ := helpers.ExtractByte(0, e.env.ReadContractData(factEvidenceAddr, []byte("result")))

	if expected == votedValue && (successAddr != common.Address{}) {
		e.SetByte("state", 2) // - unlocked success
		e.env.Send(e.ctx, successAddr, e.env.Balance(e.ctx.ContractAddr()))
	}

	if expected != votedValue && (failAddr != common.Address{}) {
		e.SetByte("state", 3) // - unlocked fail
		e.env.Send(e.ctx, failAddr, e.env.Balance(e.ctx.ContractAddr()))
	}

	if expected == votedValue && (successAddr == common.Address{}) || expected != votedValue && (failAddr == common.Address{}) {
		e.SetByte("state", 4) // - unlocked refund
		delay := e.GetUint64("refundDelay")
		e.SetUint64("refundBlock", e.env.BlockNumber()+delay)
	}

	return nil
}

func (e *RefundableEvidenceLock) refund(args ...[]byte) error {
	if e.GetByte("state") != 4 {
		return errors.New("state is not unlocked_refund")
	}
	if e.env.BlockNumber() < e.GetUint64("refundBlock") {
		return errors.New("block height is less than refundBlock")
	}
	has := false
	e.deposits.Iterate(func(key []byte, value []byte) bool {
		has = true
		return true
	})
	if !has {
		return errors.New("no deposits to refund")
	}
	balance := e.env.Balance(e.ctx.ContractAddr())
	k := decimal.NewFromBigInt(balance, 0).Div(decimal.NewFromBigInt(e.GetBigInt("sum"), 0))

	var err error
	e.deposits.Iterate(func(key []byte, value []byte) bool {
		var dest common.Address
		dest.SetBytes(key)

		deposit := big.NewInt(0)
		deposit.SetBytes(value)

		err = e.env.Send(e.ctx, dest, math2.ToInt(decimal.NewFromBigInt(deposit, 0).Mul(k)))
		return err != nil
	})
	return err
}

func (e *RefundableEvidenceLock) Terminate(args ...[]byte) error {
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
