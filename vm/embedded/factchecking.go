package embedded

import (
	"bytes"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/vrf/p256"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"math/big"
)

var (
	maxHash *big.Float
)

func init() {
	var max [32]byte
	for i := range max {
		max[i] = 0xFF
	}
	i := new(big.Int)
	i.SetBytes(max[:])
	maxHash = new(big.Float).SetInt(i)
}

type FactChecking struct {
	*BaseContract
	voteHashes *env.Map
	votes      *env.Map
}

func NewFactCheckingContract(ctx env.CallContext, e env.Env) *FactChecking {
	return &FactChecking{
		&BaseContract{
			ctx: ctx,
			env: e,
		},
		env.NewMap([]byte("voteHashes"), e, ctx),
		env.NewMap([]byte("votes"), e, ctx),
	}
}

func (f *FactChecking) Deploy(args ...[]byte) error {
	cid, err := helpers.ExtractArray(0, args...)
	if err != nil {
		return err
	}
	startTime, err := helpers.ExtractUInt64(1, args...)
	if err != nil {
		return err
	}
	f.BaseContract.Deploy(TimeLockContract)
	f.SetOwner(f.ctx.Sender())
	f.SetUint64("startTime", startTime)
	f.SetArray("fact", cid)
	f.SetUint64("state", 0)
	return nil
}

func (f *FactChecking) Call(method string, args ...[]byte) error {
	switch method {
	case "startVoting":
		return f.startVoting()
	case "sendVoteProof":
		return f.sendVoteProof(args...)
	case "sendVote":
		return f.sendVote(args...)
	case "finishVoting":
		return f.finishVoting(args...)
	case "prolongVoting":
		return f.prolongVoting(args...)
	case "terminate":
		return f.terminate(args...)
	default:
		return errors.New("unknown method")
	}
}

func (f *FactChecking) startVoting() error {
	if !f.IsOwner() {
		return errors.New("sender is not an owner")
	}
	if f.GetUint64("state") != 0 {
		return errors.New("contract is not in pending state")
	}
	if uint64(f.env.BlockTimeStamp()) < f.GetUint64("startTime") {
		return errors.New("starting is locked")
	}
	balance := f.env.Balance(f.ctx.ContractAddr())
	minBalance := big.NewInt(0).Mul(f.env.MinFeePerByte(), big.NewInt(100000))
	if balance.Cmp(minBalance) < 0 {
		return errors.New("insufficient funds")
	}
	f.SetUint64("state", 1)
	f.SetUint64("startBlock", f.env.BlockNumber())

	payment := decimal.NewFromBigInt(balance, 0)
	payment = payment.Div(decimal.New(20, 0))

	f.SetBigInt("votingMinPayment", math.ToInt(payment))
	f.SetUint64("confirmCount", 0)
	f.SetUint64("rejectCount", 0)
	f.SetArray("vrfSeed", f.env.BlockSeed())
	f.SetUint64("vrfThreshold", math.Min(100, uint64(f.env.NetworkSize())))
	return nil
}

func (f *FactChecking) sendVoteProof(args ...[]byte) error {
	voteHash, err := helpers.ExtractArray(0, args...)
	if err != nil {
		return err
	}
	proof, err := helpers.ExtractArray(1, args...)
	if err != nil {
		return err
	}
	if !f.env.State(f.ctx.Sender()).NewbieOrBetter() {
		return errors.New("sender is now identity")
	}
	if f.GetUint64("state") != 1 {
		return errors.New("contract is not in running state")
	}
	if f.env.BlockNumber()-f.GetUint64("startBlock") >= 4320 {
		return errors.New("wrong block number")
	}
	payment := f.GetBigInt("votingMinPayment")
	if payment.Cmp(f.ctx.PayAmount()) < 0 {
		return errors.New("tx amount is less than votingMinPayment")
	}

	pubKeyData := f.env.PubKey(f.ctx.Sender())

	pubKey, err := crypto.UnmarshalPubkey(pubKeyData)
	if err != nil {
		return err
	}
	verifier, err := p256.NewVRFVerifier(pubKey)
	if err != nil {
		return err
	}

	h, err := verifier.ProofToHash(f.GetArray("vrfSeed"), proof)

	v := new(big.Float).SetInt(new(big.Int).SetBytes(h[:]))

	q := new(big.Float).Quo(v, maxHash).SetPrec(10)

	threshold := f.GetUint64("vrfThreshold")
	networkSize := float64(f.env.NetworkSize())
	if networkSize == 0 {
		networkSize = 1
	}
	if v, _ := q.Float64(); v < 1-float64(threshold)/networkSize {
		return errors.New("invalid proof")
	}
	f.voteHashes.Set(f.ctx.Sender().Bytes(), voteHash)
	return nil
}

func (f *FactChecking) sendVote(args ...[]byte) error {

	//vote = 1 - confirm, vote = 2 - reject
	vote, err := helpers.ExtractUInt64(0, args...)
	if err != nil {
		return err
	}

	salt, err := helpers.ExtractArray(0, args...)

	if f.GetUint64("state") != 1 {
		return errors.New("contract is not in running state")
	}
	duration := f.env.BlockNumber() - f.GetUint64("startBlock")
	if duration < 4320 || duration > 4320*2 {
		return errors.New("wrong block number")
	}
	storedHash := f.voteHashes.Get(f.ctx.Sender().Bytes())

	computedHash := crypto.Hash(append(common.ToBytes(vote), salt...))

	if bytes.Compare(storedHash, computedHash[:]) != 0 {
		return errors.New("wrong vote hash")
	}
	f.votes.Set(f.ctx.Sender().Bytes(), args[0])
	f.voteHashes.Remove(f.ctx.Sender().Bytes())
	if vote == 1 {
		c := f.GetUint64("confirmCount") + 1
		f.SetUint64("confirmCount", c)
	} else if vote == 2 {
		c := f.GetUint64("rejectCount") + 1
		f.SetUint64("rejectCount", c)
	}
	return nil
}

func (f *FactChecking) finishVoting(args ...[]byte) error {
	if f.GetUint64("state") != 1 {
		return errors.New("contract is not in running state")
	}
	confirms := f.GetUint64("confirmCount")
	rejects := f.GetUint64("rejectCount")
	duration := f.env.BlockNumber() - f.GetUint64("startBlock")
	if confirms >= math.Min(75, uint64(f.env.NetworkSize()*3/4)) ||
		rejects >= math.Min(50, uint64(f.env.NetworkSize()/2)) ||
		duration >= 4320*2 && confirms+rejects > 25 {

		f.SetUint64("state", 2)
		var result uint64
		var reward *big.Int
		balance := decimal.NewFromBigInt(f.env.Balance(f.ctx.ContractAddr()), 0)
		if confirms >= (confirms+rejects)*2/3 {
			result = 1
			reward = math.ToInt(balance.Mul(decimal.NewFromFloat(0.7)).Div(decimal.NewFromInt(int64(confirms))))
		}
		if rejects > (confirms+rejects)/3 {
			result = 2
			reward = math.ToInt(balance.Mul(decimal.NewFromFloat(0.7)).Div(decimal.NewFromInt(int64(rejects))))
		}
		f.votes.Iterate(func(key []byte, value []byte) bool {
			vote, _ := helpers.ExtractUInt64(0, value)
			if vote == result {
				dest := common.Address{}
				dest.SetBytes(key)
				f.env.Send(f.ctx, dest, reward)
			}
			return false
		})
		f.env.BurnAll(f.ctx)
		return nil
	}
	return errors.New("no quorum")
}

func (f *FactChecking) prolongVoting(args ...[]byte) error {
	if f.GetUint64("state") != 1 {
		return errors.New("contract is not in running state")
	}
	confirms := f.GetUint64("confirmCount")
	rejects := f.GetUint64("rejectCount")
	duration := f.env.BlockNumber() - f.GetUint64("startBlock")
	if confirms < math.Min(75, uint64(f.env.NetworkSize()*3/4)) &&
		rejects < math.Min(50, uint64(f.env.NetworkSize()/2)) ||
		duration >= 4320*2 && confirms+rejects < 25 {
		f.SetArray("vrfSeed", f.env.BlockSeed())
		f.SetUint64("startBlock", f.env.BlockNumber())
		return nil
	}
	return errors.New("voting can not be prolonged")
}

func (f *FactChecking) terminate(args ...[]byte) error {
	if f.GetUint64("state") != 1 {
		return errors.New("contract is not in running state")
	}
	duration := f.env.BlockNumber() - f.GetUint64("startBlock")
	if duration >= 4320*3 {
		balance := f.env.Balance(f.ctx.ContractAddr())
		if err := f.env.Send(f.ctx, f.ctx.Sender(), balance); err != nil {
			return err
		}
		return nil
	}
	return errors.New("voting can not be terminated")
}
