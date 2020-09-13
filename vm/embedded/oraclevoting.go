package embedded

import (
	"bytes"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/crypto"
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

type OracleVoting struct {
	*BaseContract
	voteHashes  *env.Map
	votes       *env.Map
	voteOptions *env.Map
}

func NewOracleVotingContract(ctx env.CallContext, e env.Env) *OracleVoting {
	return &OracleVoting{
		&BaseContract{
			ctx: ctx,
			env: e,
		},
		env.NewMap([]byte("voteHashes"), e, ctx),
		env.NewMap([]byte("votes"), e, ctx),
		env.NewMap([]byte("voteOptions"), e, ctx),
	}
}

func (f *OracleVoting) Call(method string, args ...[]byte) error {
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
	default:
		return errors.New("unknown method")
	}
}

func (f *OracleVoting) Read(method string, args ...[]byte) ([]byte, error) {

	switch method {
	case "proof":

		addr, err := helpers.ExtractAddr(0, args...)
		if err != nil {
			return nil, err
		}

		if f.GetUint64("state") != 1 {
			return nil, errors.New("contract is not in running state")
		}

		duration := f.GetUint64("votingDuration")

		if f.env.BlockNumber()-f.GetUint64("startBlock") >= duration {
			return nil, errors.New("wrong block number")
		}

		seed := f.GetArray("vrfSeed")

		pubKey := f.env.PubKey(addr)

		h := crypto.Hash(append(pubKey, seed...))

		v := new(big.Float).SetInt(new(big.Int).SetBytes(h[:]))

		q := new(big.Float).Quo(v, maxHash)

		committeeSize := f.GetUint64("committeeSize")
		networkSize := float64(f.env.NetworkSize())
		if networkSize == 0 {
			networkSize = 1
		}
		if q.Cmp(big.NewFloat(1-float64(committeeSize)/networkSize)) < 0 {
			return nil, errors.New("invalid proof")
		}
		return []byte{1}, nil
	case "voteHash":
		vote, err := helpers.ExtractByte(0, args...)
		if err != nil {
			return nil, err
		}
		salt, err := helpers.ExtractArray(1, args...)
		if err != nil {
			return nil, err
		}
		hash := crypto.Hash(append(common.ToBytes(vote), salt...))
		return hash[:], nil
	case "voteBlock":
		block := f.GetUint64("startBlock") + f.GetUint64("votingDuration")
		return common.ToBytes(block), nil
	default:
		return nil, errors.New("unknown method")
	}
}

func (f *OracleVoting) Deploy(args ...[]byte) error {
	fact, err := helpers.ExtractArray(0, args...)
	if err != nil {
		return err
	}
	startTime, err := helpers.ExtractUInt64(1, args...)
	if err != nil {
		return err
	}

	f.BaseContract.Deploy(OracleVotingContract)

	votingDuration := uint64(4320)
	publicVotingDuration := uint64(4320)
	winnerThreshold := uint64(50)
	quorum := uint64(20)
	networkSize := uint64(f.env.NetworkSize())
	committeeSize := math.Min(100, networkSize)
	maxOptions := uint64(2)

	if value, err := helpers.ExtractUInt64(2, args...); err == nil {
		votingDuration = value
	}
	if value, err := helpers.ExtractUInt64(3, args...); err == nil {
		publicVotingDuration = value
	}
	if value, err := helpers.ExtractUInt64(4, args...); err == nil {
		winnerThreshold = math.Min(math.Max(50, value), 100)
	}
	if value, err := helpers.ExtractUInt64(5, args...); err == nil {
		quorum = math.Min(math.Max(20, value), 100)
	}

	if value, err := helpers.ExtractUInt64(6, args...); err == nil {
		committeeSize = math.Min(value, networkSize)
	}

	if value, err := helpers.ExtractUInt64(7, args...); err == nil {
		maxOptions = value
	}

	if value, err := helpers.ExtractBigInt(8, args...); err == nil {
		f.SetBigInt("votingMinPayment", value)
	}

	f.SetOwner(f.ctx.Sender())
	f.SetUint64("startTime", startTime)
	f.SetArray("fact", fact)
	f.SetUint64("state", 0)
	f.SetUint64("votingDuration", votingDuration)
	f.SetUint64("publicVotingDuration", publicVotingDuration)
	f.SetUint64("winnerThreshold", winnerThreshold)
	f.SetUint64("quorum", quorum)
	f.SetUint64("committeeSize", committeeSize)
	f.SetUint64("maxOptions", maxOptions)
	return nil
}

func (f *OracleVoting) startVoting() error {
	if f.GetUint64("state") != 0 {
		return errors.New("contract is not in pending state")
	}
	if uint64(f.env.BlockTimeStamp()) < f.GetUint64("startTime") {
		return errors.New("starting is locked")
	}

	balance := f.env.Balance(f.ctx.ContractAddr())
	minBalance := big.NewInt(0).Mul(f.env.MinFeePerByte(), big.NewInt(int64(100*100*f.GetUint64("committeeSize"))))
	if balance.Cmp(minBalance) < 0 {
		return errors.New("insufficient funds")
	}
	f.SetUint64("state", 1)
	f.SetUint64("startBlock", f.env.BlockNumber())

	if f.GetBigInt("votingMinPayment") == nil {
		payment := decimal.NewFromBigInt(balance, 0)
		payment = payment.Div(decimal.New(20, 0))
		f.SetBigInt("votingMinPayment", math.ToInt(payment))
	}
	f.SetArray("vrfSeed", f.env.BlockSeed())
	f.SetUint16("epoch", f.env.Epoch())
	return nil
}

func (f *OracleVoting) sendVoteProof(args ...[]byte) error {
	voteHash, err := helpers.ExtractArray(0, args...)
	if err != nil {
		return err
	}
	if !f.env.State(f.ctx.Sender()).NewbieOrBetter() {
		return errors.New("sender is now identity")
	}
	if f.env.Epoch() != f.GetUint16("epoch") {
		return errors.New("voting should be prolonged")
	}
	if f.GetUint64("state") != 1 {
		return errors.New("contract is not in running state")
	}
	if f.votes.Get(f.ctx.Sender().Bytes()) != nil {
		return errors.New("sender has voted already")
	}

	duration := f.GetUint64("votingDuration")

	if f.env.BlockNumber()-f.GetUint64("startBlock") >= duration {
		return errors.New("wrong block number")
	}
	payment := f.GetBigInt("votingMinPayment")
	if payment.Cmp(f.ctx.PayAmount()) < 0 {
		return errors.New("tx amount is less than votingMinPayment")
	}

	pubKeyData := f.env.PubKey(f.ctx.Sender())

	selectionHash := crypto.Hash(append(pubKeyData, f.GetArray("vrfSeed")...))

	v := new(big.Float).SetInt(new(big.Int).SetBytes(selectionHash[:]))

	q := new(big.Float).Quo(v, maxHash)

	committeeSize := f.GetUint64("committeeSize")
	networkSize := float64(f.env.NetworkSize())
	if networkSize == 0 {
		networkSize = 1
	}
	if q.Cmp(big.NewFloat(1-float64(committeeSize)/networkSize)) < 0 {
		return errors.New("invalid proof")
	}
	f.voteHashes.Set(f.ctx.Sender().Bytes(), voteHash)
	return nil
}

func (f *OracleVoting) sendVote(args ...[]byte) error {

	//vote = [0..255]
	vote, err := helpers.ExtractByte(0, args...)
	if err != nil {
		return err
	}

	salt, err := helpers.ExtractArray(1, args...)
	if err != nil {
		return err
	}
	if f.GetUint64("state") != 1 {
		return errors.New("contract is not in running state")
	}

	votingDuration := f.GetUint64("votingDuration")
	publicVotingDuration := f.GetUint64("publicVotingDuration")

	duration := f.env.BlockNumber() - f.GetUint64("startBlock")
	if duration < votingDuration || duration > votingDuration+publicVotingDuration {
		return errors.New("wrong block number")
	}
	storedHash := f.voteHashes.Get(f.ctx.Sender().Bytes())

	computedHash := crypto.Hash(append(common.ToBytes(vote), salt...))

	if bytes.Compare(storedHash, computedHash[:]) != 0 {
		return errors.New("wrong vote hash")
	}
	f.votes.Set(f.ctx.Sender().Bytes(), args[0][:1])
	f.voteHashes.Remove(f.ctx.Sender().Bytes())

	cnt, _ := helpers.ExtractUInt64(0, f.voteOptions.Get(args[0][:1]))
	f.voteOptions.Set(args[0][:1], common.ToBytes(cnt+1))

	c := f.GetUint64("votedCount") + 1
	f.SetUint64("votedCount", c)

	return nil
}

func (f *OracleVoting) finishVoting(args ...[]byte) error {
	if f.GetUint64("state") != 1 {
		return errors.New("contract is not in running state")
	}
	duration := f.env.BlockNumber() - f.GetUint64("startBlock")

	winnerVotesCnt := uint64(0)
	winner := byte(0)

	f.voteOptions.Iterate(func(key []byte, value []byte) bool {
		cnt, _ := helpers.ExtractUInt64(0, value)
		if cnt > winnerVotesCnt {
			winnerVotesCnt = cnt
			winner = key[0]
		}
		return false
	})

	committeeSize := f.GetUint64("committeeSize")
	winnerThreshold := f.GetUint64("winnerThreshold")

	votingDuration := f.GetUint64("votingDuration")
	publicVotingDuration := f.GetUint64("publicVotingDuration")

	votedCount := f.GetUint64("votedCount")
	quorum := f.GetUint64("quorum")

	if winnerVotesCnt >= committeeSize*winnerThreshold/100 ||
		duration >= votingDuration+publicVotingDuration && votedCount >= committeeSize*quorum/100 {

		f.SetUint64("state", 2)
		var result *byte
		var reward *big.Int
		fund := decimal.NewFromBigInt(f.env.Balance(f.ctx.ContractAddr()), 0)
		if winnerVotesCnt >= votedCount*winnerThreshold/100 {
			result = &winner
			reward = math.ToInt(fund.Div(decimal.NewFromInt(int64(winnerVotesCnt))))
		}
		if winnerVotesCnt < votedCount*winnerThreshold/100 {
			result = nil
			reward = math.ToInt(fund.Div(decimal.NewFromInt(int64(votedCount))))
		}
		f.votes.Iterate(func(key []byte, value []byte) bool {
			vote, _ := helpers.ExtractByte(0, value)
			if result == nil || vote == *result {
				dest := common.Address{}
				dest.SetBytes(key)
				f.env.Send(f.ctx, dest, reward)
				f.env.Event("reward", dest.Bytes(), reward.Bytes())
			}
			return false
		})

		f.env.BurnAll(f.ctx)
		if result != nil {
			f.SetByte("result", *result)
		}
		return nil
	}
	return errors.New("no quorum")
}

func (f *OracleVoting) prolongVoting(args ...[]byte) error {
	if f.GetUint64("state") != 1 {
		return errors.New("contract is not in running state")
	}

	winnerVotesCnt := uint64(0)

	f.voteOptions.Iterate(func(key []byte, value []byte) bool {
		cnt, _ := helpers.ExtractUInt64(0, value)
		if cnt > winnerVotesCnt {
			winnerVotesCnt = cnt
		}
		return false
	})

	duration := f.env.BlockNumber() - f.GetUint64("startBlock")

	committeeSize := f.GetUint64("committeeSize")
	winnerThreshold := f.GetUint64("winnerThreshold")

	votingDuration := f.GetUint64("votingDuration")
	publicVotingDuration := f.GetUint64("publicVotingDuration")

	votedCount := f.GetUint64("votedCount")
	quorum := f.GetUint64("quorum")

	if f.env.Epoch() != f.GetUint16("epoch") || winnerVotesCnt < committeeSize*winnerThreshold/100 &&
		votedCount < committeeSize*quorum/100 &&
		duration >= votingDuration+publicVotingDuration {
		f.SetArray("vrfSeed", f.env.BlockSeed())
		if duration >= votingDuration+publicVotingDuration {
			f.SetUint64("startBlock", f.env.BlockNumber())
		}
		f.SetUint16("epoch", f.env.Epoch())
		return nil
	}
	return errors.New("voting can not be prolonged")
}

func (f *OracleVoting) Terminate(args ...[]byte) error {

	if f.GetUint64("state") < 1 {
		return errors.New("contract is not in running state")
	}
	duration := f.env.BlockNumber() - f.GetUint64("startBlock")

	votingDuration := f.GetUint64("votingDuration")
	publicVotingDuration := f.GetUint64("publicVotingDuration")
	if duration >= votingDuration+publicVotingDuration*2 {
		balance := f.env.Balance(f.ctx.ContractAddr())
		if balance.Sign() > 0 {
			if err := f.env.Send(f.ctx, f.ctx.Sender(), balance); err != nil {
				return err
			}
		}
		f.env.Terminate(f.ctx, f.Owner())
		return nil
	}
	return errors.New("voting can not be terminated")
}
