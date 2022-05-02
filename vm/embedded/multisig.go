package embedded

import (
	"bytes"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/pkg/errors"
	"math/big"
)

const (
	multisigUninitialized = byte(1)
	multisigInitialized   = byte(2)
)

type Multisig struct {
	*BaseContract
	voteAddress *env.Map
	voteAmount  *env.Map
}

func NewMultisig(ctx env.CallContext, e env.Env, statsCollector collector.StatsCollector) *Multisig {
	return &Multisig{&BaseContract{
		ctx:            ctx,
		env:            e,
		statsCollector: statsCollector,
	}, env.NewMap([]byte("addr"), e, ctx), env.NewMap([]byte("amount"), e, ctx)}
}

func (m *Multisig) Deploy(args ...[]byte) error {

	var maxVotes byte
	var err error
	maxVotes, err = helpers.ExtractByte(0, args...)
	if err != nil {
		return err
	}
	if maxVotes > 32 || maxVotes < 1 {
		return errors.New("maxvotes should be in range [1;32]")
	}
	m.SetByte("maxVotes", maxVotes)
	minVotes, err := helpers.ExtractByte(1, args...)
	if err != nil {
		return err
	}
	if minVotes > maxVotes || minVotes < 1 {
		return errors.New("minvotes should be in range [1;maxvotes]")
	}
	m.SetByte("minVotes", minVotes)
	state := multisigUninitialized
	m.SetByte("state", state)
	m.SetByte("count", 0)
	m.SetOwner(m.ctx.Sender())
	collector.AddMultisigDeploy(m.statsCollector, m.ctx.ContractAddr(), minVotes, maxVotes, state)
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
	if m.GetByte("state") != multisigUninitialized {
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
		state := multisigInitialized
		m.SetByte("state", state)
		m.RemoveValue("count")
		collector.AddMultisigCallAdd(m.statsCollector, addr, &state)
	} else {
		collector.AddMultisigCallAdd(m.statsCollector, addr, nil)
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
	collector.AddMultisigCallSend(m.statsCollector, dest, amount)
	return nil
}

func (m *Multisig) push(args ...[]byte) error {

	if m.GetByte("state") != multisigInitialized {
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
	votes := 0
	m.voteAddress.Iterate(func(key []byte, value []byte) bool {
		if bytes.Compare(value, dest.Bytes()) == 0 {
			if bytes.Compare(m.voteAmount.Get(key), amount) == 0 {
				votes++
			}
		}
		return false
	})

	minVotes := m.GetByte("minVotes")
	if votes < int(minVotes) {
		return errors.New("votes < minVotes")
	}

	if err = m.env.Send(m.ctx, dest, big.NewInt(0).SetBytes(amount)); err != nil {
		return err
	}

	m.voteAmount.Iterate(func(key []byte, value []byte) bool {
		m.voteAmount.Set(key, common.Big0.Bytes())
		return false
	})
	collector.AddMultisigCallPush(m.statsCollector, dest, amount, votes, votes)
	return nil
}

func (m *Multisig) Terminate(args ...[]byte) (common.Address, [][]byte, error) {
	if !m.IsOwner() {
		return common.Address{}, nil, errors.New("sender is not an owner")
	}
	balance := m.env.Balance(m.ctx.ContractAddr())
	dust := big.NewInt(0).Mul(m.env.MinFeePerGas(), big.NewInt(100))
	if balance.Cmp(dust) > 0 {
		return common.Address{}, nil, errors.New("contract has dna")
	}
	if balance.Sign() > 0 {
		m.env.BurnAll(m.ctx)
	}
	dest, err := helpers.ExtractAddr(0, args...)
	if err != nil {
		return common.Address{}, nil, err
	}
	collector.AddMultisigTermination(m.statsCollector, dest)
	return dest, nil, nil
}
