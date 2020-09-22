package embedded

import (
	"bytes"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/pkg/errors"
	"math/big"
)

type Multisig struct {
	*BaseContract
	voteAddress *env.Map
	voteAmount  *env.Map
}

func NewMultisig(ctx env.CallContext, e env.Env) *Multisig {
	return &Multisig{&BaseContract{
		ctx: ctx,
		env: e,
	}, env.NewMap([]byte("addr"), e, ctx), env.NewMap([]byte("amount"), e, ctx)}
}

func (m *Multisig) Deploy(args ...[]byte) error {

	var maxVotes byte
	var err error
	if maxVotes, err = helpers.ExtractByte(0, args...); err != nil {
		return err
	} else {
		if maxVotes > 32 || maxVotes < 1 {
			return errors.New("maxvotes should be in range [1;32]")
		}
		m.SetByte("maxVotes", maxVotes)
	}

	if minVotes, err := helpers.ExtractByte(1, args...); err != nil {
		return err
	} else {
		if minVotes > maxVotes || minVotes < 1 {
			return errors.New("minvotes should be in range [1;maxvotes]")
		}
		m.SetByte("minVotes", minVotes)
	}

	m.SetByte("state", 1)
	m.SetByte("count", 0)
	m.BaseContract.Deploy(MultisigContract)
	m.SetOwner(m.ctx.Sender())
	return nil
}

func (m *Multisig) Call(method string, args ...[]byte) (err error) {
	switch method {
	case "add":
		return m.add(args...)
	case "send":
		return m.send(args...)
	case "push":
		return m.push(args...)
	default:
		return errors.New("unknown method")
	}
}

func (m *Multisig) Read(method string, args ...[]byte) ([]byte, error) {
	panic("implement me")
}

func (m *Multisig) add(args ...[]byte) (err error) {
	if !m.IsOwner() {
		return errors.New("sender is not an owner")
	}
	if m.GetByte("state") != 1 {
		return errors.New("contract is initialized")
	}

	addr, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return err
	}

	if m.voteAddress.Get(addr.Bytes()) != nil {
		return errors.New("address has been added")
	}

	m.voteAddress.Set(addr.Bytes(), addr.Bytes())
	m.voteAmount.Set(addr.Bytes(), common.Big0.Bytes())
	c := m.GetByte("count") + 1
	m.SetByte("count", c)
	if c == m.GetByte("maxVotes") {
		m.SetByte("state", 2)
		m.RemoveValue("count")
	}
	return nil
}

func (m *Multisig) send(args ...[]byte) error {
	dest, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return err
	}
	amount, err := helpers.ExtractArray(1, args...)
	if err != nil {
		return err
	}
	voteAmount := m.voteAmount.Get(m.ctx.Sender().Bytes())
	if voteAmount == nil {
		return errors.New("unknown sender")
	}

	m.voteAmount.Set(m.ctx.Sender().Bytes(), amount)
	m.voteAddress.Set(m.ctx.Sender().Bytes(), dest.Bytes())
	return nil
}

func (m *Multisig) push(args ...[]byte) error {

	if m.GetByte("state") != 2 {
		return errors.New("contract is not initialized")
	}

	dest, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return err
	}

	amount, err := helpers.ExtractArray(1, args...)
	if err != nil {
		return err
	}
	voteAddressCnt := 0
	m.voteAddress.Iterate(func(key []byte, value []byte) bool {
		if bytes.Compare(value, dest.Bytes()) == 0 {
			voteAddressCnt++
		}
		return false
	})

	minVotes := m.GetByte("minVotes")
	if voteAddressCnt < int(minVotes) {
		return errors.New("voteAddressCnt < minVotes")
	}

	voteAmountCnt := 0
	m.voteAmount.Iterate(func(key []byte, value []byte) bool {
		if bytes.Compare(value, amount) == 0 {
			voteAmountCnt++
		}
		return false
	})

	if voteAmountCnt < int(minVotes) {
		return errors.New("voteAmountCnt < minVotes")
	}

	if err = m.env.Send(m.ctx, dest, big.NewInt(0).SetBytes(amount)); err != nil {
		return err
	}

	m.voteAmount.Iterate(func(key []byte, value []byte) bool {
		m.voteAmount.Set(key, common.Big0.Bytes())
		return false
	})
	return nil
}

func (m *Multisig) Terminate(args ...[]byte) error {
	if !m.IsOwner() {
		return errors.New("sender is not an owner")
	}
	balance := m.env.Balance(m.ctx.ContractAddr())
	if balance.Sign() > 0 {
		return errors.New("contract has dna")
	}
	dest, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return err
	}
	m.env.Terminate(m.ctx, dest)
	return nil
}
