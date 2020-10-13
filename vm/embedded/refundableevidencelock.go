package embedded

import (
	"github.com/idena-network/idena-go/common"
	math2 "github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/stats/collector"
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
	oracleVoting, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return err
	}
	e.SetArray("oracleVoting", oracleVoting.Bytes())
	value, err := helpers.ExtractByte(1, args...)
	if err != nil {
		return err
	}
	e.SetByte("value", value)

	successAddr, successAddrErr := helpers.ExtractAddr(2, args...)
	if successAddrErr == nil {
		e.SetArray("successAddr", successAddr.Bytes())
	}

	failAddr, failAddrErr := helpers.ExtractAddr(3, args...)
	if failAddrErr == nil {
		e.SetArray("failAddr", failAddr.Bytes())
	}

	refundDelay := uint64(4320)
	if delay, err := helpers.ExtractUInt64(4, args...); err == nil {
		refundDelay = delay
	}
	e.SetUint64("refundDelay", refundDelay)

	depositDeadline, err := helpers.ExtractUInt64(5, args...)
	if err == nil {
		e.SetUint64("depositDeadline", depositDeadline)
	} else {
		return errors.Wrap(err, "depositDeadline (5) is required")
	}

	factEvidenceFee := byte(0)
	if fee, err := helpers.ExtractByte(6, args...); err != nil {
		return err
	} else {
		factEvidenceFee = byte(math2.MaxInt(1, math2.MinInt(100, int(fee))))
	}
	e.SetByte("factEvidenceFee", factEvidenceFee)

	e.BaseContract.Deploy(RefundableEvidenceLockContract)
	e.SetOwner(e.ctx.Sender())

	state := byte(1) // - locked
	e.SetByte("state", state)
	sum := big.NewInt(0)
	e.SetBigInt("sum", sum)

	collector.AddRefundableOracleLockDeploy(e.statsCollector, e.ctx.ContractAddr(), oracleVoting, value, successAddr,
		successAddrErr, failAddr, failAddrErr, refundDelay, depositDeadline, factEvidenceFee, state, sum)
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
	var oracleVotingAddr common.Address
	oracleVotingAddr.SetBytes(e.GetArray("oracleVoting"))

	if uint64(e.env.BlockTimeStamp()) > e.GetUint64("depositDeadline") {
		return errors.New("deposit is late")
	}

	minDeposit := big.NewInt(0).Mul(e.env.MinFeePerGas(), big.NewInt(10000))
	if e.ctx.PayAmount().Cmp(minDeposit) < 0 {
		return errors.New("deposit is low")
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
	fee = fee.Mul(decimal.NewFromInt(int64(feeRate))).Div(decimal.NewFromInt(100))

	feeInt := math2.ToInt(fee)
	err := e.env.Send(e.ctx, oracleVotingAddr, feeInt)
	if err != nil {
		return err
	}
	collector.AddRefundableOracleLockCallDeposit(e.statsCollector, balance, sum, feeInt)
	return nil
}

func (e *RefundableEvidenceLock) push(args ...[]byte) error {
	var oracleVoting common.Address
	oracleVoting.SetBytes(e.GetArray("oracleVoting"))
	oracleVotingExist := e.env.ContractStake(oracleVoting) != nil && e.env.ContractStake(oracleVoting).Sign() > 0

	state, _ := helpers.ExtractUInt64(0, e.env.ReadContractData(oracleVoting, []byte("state")))
	if state != 2 && oracleVotingExist {
		return errors.New("voting is not completed")
	}
	expected := e.GetByte("value")

	var failAddr common.Address
	failAddr.SetBytes(e.GetArray("failAddr"))

	var successAddr common.Address
	successAddr.SetBytes(e.GetArray("successAddr"))

	votedValue, votedValueErr := helpers.ExtractByte(0, e.env.ReadContractData(oracleVoting, []byte("result")))

	var newState byte
	var amount *big.Int
	if expected == votedValue && !successAddr.IsEmpty() {
		newState = 2 // - unlocked success
		e.SetByte("state", newState)
		amount = e.env.Balance(e.ctx.ContractAddr())
		e.env.Send(e.ctx, successAddr, amount)
	}

	if expected != votedValue && !failAddr.IsEmpty() {
		newState = 3 // - unlocked fail
		e.SetByte("state", newState)
		amount = e.env.Balance(e.ctx.ContractAddr())
		e.env.Send(e.ctx, failAddr, amount)
	}

	var refundBlock uint64
	if expected == votedValue && successAddr.IsEmpty() ||
		expected != votedValue && failAddr.IsEmpty() ||
		!oracleVotingExist {
		newState = 4 // - unlocked refund
		e.SetByte("state", newState)
		delay := e.GetUint64("refundDelay")
		refundBlock = e.env.BlockNumber() + delay
		e.SetUint64("refundBlock", refundBlock)
	}

	collector.AddRefundableOracleLockCallPush(e.statsCollector, newState, oracleVotingExist, votedValue, votedValueErr, amount, refundBlock)

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
	if err != nil {
		return err
	}
	collector.AddRefundableOracleLockCallRefund(e.statsCollector, balance, k)
	return nil
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
	collector.AddRefundableOracleLockTermination(e.statsCollector, dest)
	return nil
}
