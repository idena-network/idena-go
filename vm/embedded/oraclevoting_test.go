package embedded

import (
	"crypto/ecdsa"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/idena-network/idena-go/tests"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

type configurableOracleVotingDeploy struct {
	contractTester *contractTester
	deployStake    *big.Int

	fact                 []byte
	startTime            uint64
	votingDuration       uint64
	publicVotingDuration uint64
	winnerThreshold      byte
	quorum               byte
	committeeSize        uint64
	votingMinPayment     *big.Int
	ownerFee             byte
	oracleRewardFund     *big.Int
	refundRecipient      *common.Address
}

func (c *configurableOracleVotingDeploy) Parameters() (contract EmbeddedContractType, deployStake *big.Int, params [][]byte) {
	var data [][]byte
	data = append(data, c.fact)
	data = append(data, common.ToBytes(c.startTime))
	data = append(data, common.ToBytes(c.votingDuration))
	data = append(data, common.ToBytes(c.publicVotingDuration))
	data = append(data, common.ToBytes(c.winnerThreshold))
	data = append(data, common.ToBytes(c.quorum))
	data = append(data, common.ToBytes(c.committeeSize))
	if c.votingMinPayment != nil {
		data = append(data, c.votingMinPayment.Bytes())
	} else {
		data = append(data, nil)
	}
	data = append(data, common.ToBytes(c.ownerFee))
	if c.oracleRewardFund != nil {
		data = append(data, c.oracleRewardFund.Bytes())
	} else {
		data = append(data, nil)
	}
	if c.refundRecipient != nil {
		data = append(data, c.refundRecipient.Bytes())
	} else {
		data = append(data, nil)
	}
	return OracleVotingContract, c.deployStake, data
}

func (c *configurableOracleVotingDeploy) SetStartTime(startTime uint64) *configurableOracleVotingDeploy {
	c.startTime = startTime
	return c
}

func (c *configurableOracleVotingDeploy) SetVotingDuration(votingDuration uint64) *configurableOracleVotingDeploy {
	c.votingDuration = votingDuration
	return c
}

func (c *configurableOracleVotingDeploy) SetPublicVotingDuration(publicVotingDuration uint64) *configurableOracleVotingDeploy {
	c.publicVotingDuration = publicVotingDuration
	return c
}

func (c *configurableOracleVotingDeploy) SetWinnerThreshold(winnerThreshold byte) *configurableOracleVotingDeploy {
	c.winnerThreshold = winnerThreshold
	return c
}

func (c *configurableOracleVotingDeploy) SetQuorum(quorum byte) *configurableOracleVotingDeploy {
	c.quorum = quorum
	return c
}

func (c *configurableOracleVotingDeploy) SetCommitteeSize(committeeSize uint64) *configurableOracleVotingDeploy {
	c.committeeSize = committeeSize
	return c
}

func (c *configurableOracleVotingDeploy) SetVotingMinPayment(votingMinPayment *big.Int) *configurableOracleVotingDeploy {
	c.votingMinPayment = votingMinPayment
	return c
}

func (c *configurableOracleVotingDeploy) SetOwnerFee(ownerFee byte) *configurableOracleVotingDeploy {
	c.ownerFee = ownerFee
	return c
}

func (c *configurableOracleVotingDeploy) SetOracleRewardFund(oracleRewardFund *big.Int) *configurableOracleVotingDeploy {
	c.oracleRewardFund = oracleRewardFund
	return c
}

func (c *configurableOracleVotingDeploy) SetRefundRecipient(refundRecipient common.Address) *configurableOracleVotingDeploy {
	c.refundRecipient = &refundRecipient
	return c
}

func (c *configurableOracleVotingDeploy) Deploy() (*oracleVotingCaller, error) {
	var err error
	if err = c.contractTester.Deploy(c); err != nil {
		return nil, err
	}
	return &oracleVotingCaller{c.contractTester}, nil
}

type oracleVotingCaller struct {
	contractTester *contractTester
}

func (c *oracleVotingCaller) StartVoting() error {
	return c.contractTester.OwnerCall(OracleVotingContract, "startVoting")
}

func (c *oracleVotingCaller) ReadProof(addr common.Address) ([]byte, error) {
	return c.contractTester.Read(OracleVotingContract, "proof", addr.Bytes())
}

func (c *oracleVotingCaller) VoteHash(vote, salt []byte) ([]byte, error) {
	return c.contractTester.Read(OracleVotingContract, "voteHash", vote, salt)
}

func (c *oracleVotingCaller) sendVoteProof(key *ecdsa.PrivateKey, vote byte, payment *big.Int) error {

	pubBytes := crypto.FromECDSAPub(&key.PublicKey)

	addr := crypto.PubkeyToAddress(key.PublicKey)

	hash := crypto.Hash(append(pubBytes, c.contractTester.ReadData("vrfSeed")...))

	v := new(big.Float).SetInt(new(big.Int).SetBytes(hash[:]))

	q := new(big.Float).Quo(v, maxHash)

	threshold, _ := helpers.ExtractUInt64(0, c.contractTester.ReadData("committeeSize"))
	shouldBeError := false
	if q.Cmp(big.NewFloat(1-float64(threshold)/float64(c.contractTester.appState.ValidatorsCache.NetworkSize()))) < 0 {
		shouldBeError = true
	}
	_, err := c.ReadProof(addr)
	if shouldBeError != (err != nil) {
		panic("assert failed")
	}

	voteHash, err := c.VoteHash(common.ToBytes(vote), addr.Bytes())
	if err != nil {
		panic(err)
	}
	return c.contractTester.Call(key, OracleVotingContract, payment, "sendVoteProof", voteHash)
}

func (c *oracleVotingCaller) sendVote(key *ecdsa.PrivateKey, vote byte, salt []byte) error {
	return c.contractTester.Call(key, OracleVotingContract, nil, "sendVote", common.ToBytes(vote), salt)
}

func (c *oracleVotingCaller) finishVoting() error {
	return c.contractTester.OwnerCall(OracleVotingContract, "finishVoting")
}

func (c *oracleVotingCaller) prolong() error {
	return c.contractTester.OwnerCall(OracleVotingContract, "prolongVoting")
}

func ConvertToInt(amount decimal.Decimal) *big.Int {
	if amount == (decimal.Decimal{}) {
		return nil
	}
	initial := decimal.NewFromBigInt(common.DnaBase, 0)
	result := amount.Mul(initial)

	return math.ToInt(result)
}

func TestOracleVoting_successScenario(t *testing.T) {
	deployContractStake := common.DnaBase

	ownerBalance := common.DnaBase

	builder := createTestContractBuilder(&networkConfig{
		identityGroups: []identityGroupConfig{
			{count: 2000, state: state.Verified},
		},
	}, ownerBalance)
	tester := builder.Build()
	ownerFee := byte(5)
	refundRecipient := tests.GetRandAddr()
	deploy := tester.ConfigureDeploy(deployContractStake).OracleVoting().SetOwnerFee(ownerFee).
		SetPublicVotingDuration(4320).SetVotingDuration(4320).
		SetRefundRecipient(refundRecipient).SetOracleRewardFund(ConvertToInt(decimal.RequireFromString("1500")))
	_, _, deployArgs := deploy.Parameters()
	caller, err := deploy.Deploy()
	require.NoError(t, err)

	caller.contractTester.Commit()

	require.Equal(t, ConvertToInt(decimal.RequireFromString("250")).Bytes(), caller.contractTester.ReadData("ownerDeposit"))
	require.Equal(t, ConvertToInt(decimal.RequireFromString("1500")).Bytes(), caller.contractTester.ReadData("oracleRewardFund"))
	require.Equal(t, refundRecipient.Bytes(), caller.contractTester.ReadData("refundRecipient"))

	caller.contractTester.setHeight(3)
	caller.contractTester.setTimestamp(30)

	contractBalance := decimal.NewFromFloat(5000.0 / 2000.0 * 99)
	caller.contractTester.SetBalance(ConvertToInt(contractBalance))

	require.Error(t, caller.StartVoting())

	contractBalance = decimal.NewFromFloat(2000)
	caller.contractTester.SetBalance(ConvertToInt(contractBalance))

	require.NoError(t, caller.StartVoting())
	caller.contractTester.Commit()

	require.Equal(t, common.ToBytes(byte(1)), caller.contractTester.ReadData("state"))
	require.Equal(t, big.NewInt(0).Quo(ConvertToInt(contractBalance), big.NewInt(20)).Bytes(), caller.contractTester.ReadData("votingMinPayment"))

	seed := types.Seed{}
	seed.SetBytes(common.ToBytes(uint64(3)))
	require.Equal(t, seed.Bytes(), caller.contractTester.ReadData("vrfSeed"))
	require.Equal(t, common.ToBytes(uint64(100)), caller.contractTester.ReadData("committeeSize"))

	// send proofs
	winnerVote := byte(1)
	votedIdentities := map[common.Address]struct{}{}
	minPayment := big.NewInt(0).SetBytes(caller.contractTester.ReadData("votingMinPayment"))

	voted := 0

	sendVoteProof := func(key *ecdsa.PrivateKey) {

		caller.contractTester.setHeight(4)

		err = caller.sendVoteProof(key, winnerVote, minPayment)
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if err == nil {
			caller.contractTester.AddBalance(minPayment)
			caller.contractTester.Commit()
			require.NoError(t, err)
			if _, ok := votedIdentities[addr]; !ok {
				voted++
			}
			votedIdentities[addr] = struct{}{}
		}
	}
	for _, key := range caller.contractTester.identities {
		sendVoteProof(key)
	}

	//send votes
	for _, key := range caller.contractTester.identities {
		addr := crypto.PubkeyToAddress(key.PublicKey)

		if _, ok := votedIdentities[addr]; !ok {
			continue
		}
		caller.contractTester.setHeight(4320 * 2)

		err := caller.sendVote(key, winnerVote, addr.Bytes())
		if _, ok := votedIdentities[addr]; !ok {
			require.Error(t, err)
		} else {
			caller.contractTester.Commit()
			require.NoError(t, err)
		}
	}

	require.NoError(t, caller.finishVoting())
	caller.contractTester.Commit()

	for addr := range votedIdentities {
		b := caller.contractTester.appState.State.GetBalance(addr)
		require.Equal(t, "113885869565217391304", b.String())
	}

	stakeAfterFinish := caller.contractTester.ContractStake()
	require.Equal(t, deployContractStake.Bytes(), stakeAfterFinish.Bytes())

	require.Equal(t, []byte{winnerVote}, caller.contractTester.ReadData("result"))

	for addr, _ := range votedIdentities {
		require.True(t, caller.contractTester.appState.State.GetBalance(addr).Sign() == 1)
	}
	require.True(t, caller.contractTester.ContractBalance().Sign() == 0)

	caller.contractTester.setHeight(4320*2 + 4 + 30240)

	owner := caller.contractTester.ReadData("owner")

	//terminate
	dest, err := caller.contractTester.Terminate(caller.contractTester.mainKey, OracleVotingContract)
	require.NoError(t, err)
	require.Equal(t, owner, dest.Bytes())

	caller.contractTester.Commit()

	stakeToBalance := big.NewInt(0).Quo(stakeAfterFinish, big.NewInt(2))
	require.Equal(t, "722500000000000000000", caller.contractTester.appState.State.GetBalance(refundRecipient).String())
	require.Equal(t, big.NewInt(0).Add(ownerBalance, stakeToBalance).String(), caller.contractTester.appState.State.GetBalance(dest).String())
	require.Nil(t, caller.contractTester.CodeHash())
	require.Nil(t, caller.contractTester.ContractStake())
	require.Nil(t, caller.contractTester.ReadData("vrfSeed"))
	require.Equal(t, []byte{0x1}, caller.contractTester.ReadData("fact"))
	require.Equal(t, []byte{winnerVote}, caller.contractTester.ReadData("result"))

	expectedHash := func() []byte {
		protoData := &models.ProtoOracleVotingHashData{
			Args: deployArgs,
		}
		data, err := proto.Marshal(protoData)
		require.NoError(t, err)
		hash := crypto.Hash128(data)
		return hash[:]
	}
	require.Equal(t, expectedHash(), caller.contractTester.ReadData("hash"))
	require.Nil(t, caller.contractTester.ReadData("state"))
}

func TestOracleVoting2_Terminate_NotStartedVoting(t *testing.T) {
	deployContractStake := common.DnaBase

	ownerBalance := common.DnaBase

	builder := createTestContractBuilder(&networkConfig{
		identityGroups: []identityGroupConfig{
			{count: 2000, state: state.Human},
		},
	}, ownerBalance)
	tester := builder.Build()
	ownerFee := byte(5)
	startTime := uint64(10)
	caller, err := tester.ConfigureDeploy(deployContractStake).OracleVoting().SetOwnerFee(ownerFee).
		SetPublicVotingDuration(4320).SetVotingDuration(4320).Deploy()
	require.NoError(t, err)

	caller.contractTester.Commit()

	timestamp := int64((time.Hour * 24 * 30).Seconds()) - 1 + int64(startTime)

	caller.contractTester.setTimestamp(timestamp)

	_, err = caller.contractTester.Terminate(caller.contractTester.mainKey, OracleVotingContract)
	require.Error(t, err)

	timestamp += 2
	caller.contractTester.setTimestamp(timestamp)

	_, err = caller.contractTester.Terminate(caller.contractTester.mainKey, OracleVotingContract)
	require.NoError(t, err)

	caller.contractTester.Commit()

	require.Nil(t, caller.contractTester.ReadData("state"))
	require.Nil(t, caller.contractTester.ReadData("fact"))
	require.Nil(t, caller.contractTester.ReadData("result"))
}

func TestOracleVoting2_TerminateRefund(t *testing.T) {
	deployContractStake := common.DnaBase

	ownerBalance := common.DnaBase

	builder := createTestContractBuilder(&networkConfig{
		identityGroups: []identityGroupConfig{
			{count: 2000, state: state.Human},
		},
	}, ownerBalance)
	tester := builder.Build()
	caller, err := tester.ConfigureDeploy(deployContractStake).OracleVoting().SetOwnerFee(10).
		SetPublicVotingDuration(4320).SetVotingDuration(4320).Deploy()
	require.NoError(t, err)

	caller.contractTester.Commit()

	contractBalance := big.NewInt(0).Mul(common.DnaBase, big.NewInt(2000))
	caller.contractTester.SetBalance(contractBalance)
	caller.contractTester.setTimestamp(21)

	require.NoError(t, caller.StartVoting())
	caller.contractTester.Commit()

	caller.contractTester.setHeight(4320*2 + 20)

	require.NoError(t, caller.prolong())
	caller.contractTester.Commit()

	require.Equal(t, common.ToBytes(byte(1)), caller.contractTester.ReadData("no-growth"))

	payment := big.NewInt(0).Mul(common.DnaBase, big.NewInt(100))

	var canVoteKeys []*ecdsa.PrivateKey

	for _, key := range caller.contractTester.identities {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if _, err := caller.ReadProof(addr); err == nil {
			canVoteKeys = append(canVoteKeys, key)
		}
	}

	sendVoteProof := func(key *ecdsa.PrivateKey, vote byte) {
		caller.contractTester.setHeight(4320*2 + 22)

		err = caller.sendVoteProof(key, vote, payment)
		if err == nil {
			caller.contractTester.AddBalance(payment)
			caller.contractTester.Commit()
			require.NoError(t, err)
		}
	}

	for i := 0; i < 5; i++ {
		sendVoteProof(canVoteKeys[i], 1)
	}
	for i := 5; i < 10; i++ {
		sendVoteProof(canVoteKeys[i], 2)
	}

	//send votes
	for i := 0; i < 5; i++ {
		addr := crypto.PubkeyToAddress(canVoteKeys[i].PublicKey)

		caller.contractTester.setHeight(4320*3 + 22)

		err := caller.sendVote(canVoteKeys[i], 1, addr.Bytes())
		require.Equal(t, "quorum is not reachable", err.Error())
	}

	caller.contractTester.setHeight(4320*2 + 20)
	require.Error(t, caller.prolong())

	caller.contractTester.setHeight(4320*4 + 20)
	require.NoError(t, caller.prolong())
	caller.contractTester.Commit()
	require.Equal(t, common.ToBytes(byte(0)), caller.contractTester.ReadData("no-growth"))

	caller.contractTester.setHeight(4320*6 + 20)
	require.NoError(t, caller.prolong())
	caller.contractTester.Commit()
	require.Equal(t, common.ToBytes(byte(1)), caller.contractTester.ReadData("no-growth"))

	caller.contractTester.setHeight(4320*8 + 20)
	require.NoError(t, caller.prolong())
	caller.contractTester.Commit()
	require.Equal(t, common.ToBytes(byte(2)), caller.contractTester.ReadData("no-growth"))

	caller.contractTester.setHeight(4320*10 + 20)
	require.NoError(t, caller.prolong())
	caller.contractTester.Commit()
	require.Equal(t, common.ToBytes(byte(3)), caller.contractTester.ReadData("no-growth"))

	caller.contractTester.setHeight(4320*12 + 20)
	require.Error(t, caller.prolong())

	caller.contractTester.setHeight(4320*12 + 4320*7 + 20)

	stakeBeforeTermination := caller.contractTester.ContractStake()
	dest, err := caller.contractTester.Terminate(caller.contractTester.mainKey, OracleVotingContract)
	caller.contractTester.Commit()
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		addr := crypto.PubkeyToAddress(canVoteKeys[i].PublicKey)
		require.Equal(t, "247500000000000000000", caller.contractTester.appState.State.GetBalance(addr).String())
	}
	expectedOwnerReward := ConvertToInt(decimal.RequireFromString("525"))
	stakeToBalance := big.NewInt(0).Quo(stakeBeforeTermination, big.NewInt(2))
	expectedOwnerBalance := new(big.Int).Add(expectedOwnerReward, new(big.Int).Add(ownerBalance, stakeToBalance))
	require.Equal(t, expectedOwnerBalance.String(), caller.contractTester.appState.State.GetBalance(dest).String())

	require.Nil(t, caller.contractTester.CodeHash())
	require.Nil(t, caller.contractTester.ContractStake())
	require.Nil(t, caller.contractTester.ReadData("fact"))
	require.Nil(t, caller.contractTester.ReadData("result"))
	require.Equal(t, 0, caller.contractTester.ContractBalance().Sign())
}

func TestOracleVoting2_Refund_No_Winner(t *testing.T) {
	deployContractStake := common.DnaBase

	ownerBalance := common.DnaBase

	builder := createTestContractBuilder(&networkConfig{
		identityGroups: []identityGroupConfig{
			{count: 2000, state: state.Human},
		},
	}, ownerBalance)
	tester := builder.Build()
	caller, err := tester.ConfigureDeploy(deployContractStake).OracleVoting().SetOwnerFee(0).
		SetPublicVotingDuration(4320).SetVotingDuration(4320).SetWinnerThreshold(66).Deploy()
	require.NoError(t, err)

	caller.contractTester.Commit()

	contractBalance := big.NewInt(0).Mul(common.DnaBase, big.NewInt(2000))
	caller.contractTester.SetBalance(contractBalance)
	caller.contractTester.setTimestamp(21)

	require.NoError(t, caller.StartVoting())
	caller.contractTester.Commit()

	caller.contractTester.setHeight(4320*2 + 20)

	require.NoError(t, caller.prolong())
	caller.contractTester.Commit()

	require.Equal(t, common.ToBytes(byte(1)), caller.contractTester.ReadData("no-growth"))

	payment := big.NewInt(0).Mul(common.DnaBase, big.NewInt(100))

	var canVoteKeys []*ecdsa.PrivateKey

	for _, key := range caller.contractTester.identities {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if _, err := caller.ReadProof(addr); err == nil {
			canVoteKeys = append(canVoteKeys, key)
		}
	}

	sendVoteProof := func(key *ecdsa.PrivateKey, vote byte) error {
		err = caller.sendVoteProof(key, vote, payment)
		if err == nil {
			caller.contractTester.AddBalance(payment)
			caller.contractTester.Commit()
			require.NoError(t, err)
		}
		return err
	}
	caller.contractTester.setHeight(4320*2 + 22)
	for i := 0; i < 10; i++ {
		vote := byte(1)
		if i%2 == 0 {
			vote = 2
		}
		require.NoError(t, sendVoteProof(canVoteKeys[i], vote))
	}

	caller.contractTester.setHeight(4320*4 + 20)
	require.NoError(t, caller.prolong())
	caller.contractTester.Commit()
	require.Equal(t, common.ToBytes(byte(0)), caller.contractTester.ReadData("no-growth"))

	caller.contractTester.setHeight(4320*4 + 21)

	var newCanVoteKeys []*ecdsa.PrivateKey

	for _, key := range caller.contractTester.identities {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if _, err := caller.ReadProof(addr); err == nil {
			newCanVoteKeys = append(newCanVoteKeys, key)
		}
	}

	for i := 10; i < 22; i++ {
		vote := byte(1)
		if i%2 == 0 {
			vote = 2
		}
		require.NoError(t, sendVoteProof(newCanVoteKeys[i], vote))
	}

	//send votes
	for i := 0; i < 10; i++ {
		addr := crypto.PubkeyToAddress(canVoteKeys[i].PublicKey)
		caller.contractTester.setHeight(4320*5 + 22)
		vote := byte(1)
		if i%2 == 0 {
			vote = 2
		}
		err = caller.sendVote(canVoteKeys[i], vote, addr.Bytes())
		caller.contractTester.Commit()
		require.NoError(t, err)
	}

	for i := 10; i < 19; i++ {
		addr := crypto.PubkeyToAddress(newCanVoteKeys[i].PublicKey)
		caller.contractTester.setHeight(4320*5 + 22)
		vote := byte(1)
		if i%2 == 0 {
			vote = 2
		}
		require.NoError(t, caller.sendVote(newCanVoteKeys[i], vote, addr.Bytes()))
		caller.contractTester.Commit()
	}
	caller.contractTester.setHeight(4320*6 + 22)

	require.NoError(t, caller.finishVoting())
	caller.contractTester.Commit()

	require.Equal(t, common.ToBytes(oracleVotingStateFinished), caller.contractTester.ReadData("state"))
	require.Nil(t, caller.contractTester.ReadData("result"))

	balance, _ := big.NewInt(0).SetString("207894736842105263157", 10) // ((2000 + (10+12) * payment - ownerDeposit) / 19)*10^18

	for i := 0; i < 10; i++ {
		addr := crypto.PubkeyToAddress(canVoteKeys[i].PublicKey)
		require.Equal(t, balance.String(), caller.contractTester.appState.State.GetBalance(addr).String())
	}

	for i := 10; i < 19; i++ {
		addr := crypto.PubkeyToAddress(newCanVoteKeys[i].PublicKey)
		require.Equal(t, balance.String(), caller.contractTester.appState.State.GetBalance(addr).String())
	}

	for i := 19; i < 22; i++ {
		addr := crypto.PubkeyToAddress(newCanVoteKeys[i].PublicKey)
		require.Equal(t, "0", caller.contractTester.appState.State.GetBalance(addr).String())
	}
}

func TestOracleVoting_RewardPools(t *testing.T) {
	deployContractStake := common.DnaBase

	ownerBalance := common.DnaBase

	builder := createTestContractBuilder(&networkConfig{
		identityGroups: []identityGroupConfig{
			{count: 2000, state: state.Human},
		},
	}, ownerBalance)
	tester := builder.Build()
	caller, err := tester.ConfigureDeploy(deployContractStake).OracleVoting().SetOwnerFee(0).
		SetPublicVotingDuration(4320).SetVotingDuration(4320).SetWinnerThreshold(66).Deploy()
	require.NoError(t, err)

	caller.contractTester.Commit()
	payment := big.NewInt(0).Mul(common.DnaBase, big.NewInt(200))

	contractBalance := big.NewInt(0).Mul(common.DnaBase, big.NewInt(3000))
	caller.contractTester.SetBalance(contractBalance)
	caller.contractTester.setTimestamp(21)

	require.NoError(t, caller.StartVoting())
	caller.contractTester.Commit()

	var canVoteKeys []*ecdsa.PrivateKey

	for _, key := range caller.contractTester.identities {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if _, err := caller.ReadProof(addr); err == nil {
			canVoteKeys = append(canVoteKeys, key)
		}
	}

	sendVoteProof := func(key *ecdsa.PrivateKey, vote byte) error {
		err = caller.sendVoteProof(key, vote, payment)
		if err == nil {
			caller.contractTester.AddBalance(payment)
			caller.contractTester.Commit()
			require.NoError(t, err)
		}
		return err
	}

	caller.contractTester.setHeight(22)
	for i := 0; i < 30; i++ {
		vote := byte(1)
		if i < 14 {
			vote = 2
		}
		require.NoError(t, sendVoteProof(canVoteKeys[i], vote))
	}

	pool1 := common.Address{0x1}
	pool2 := common.Address{0x2}

	for i := 0; i < 15; i++ {
		addr := crypto.PubkeyToAddress(canVoteKeys[i].PublicKey)
		caller.contractTester.appState.State.SetDelegatee(addr, pool1)
		caller.contractTester.appState.IdentityState.SetDelegatee(addr, pool1)
	}

	for i := 15; i < 20; i++ {
		addr := crypto.PubkeyToAddress(canVoteKeys[i].PublicKey)
		caller.contractTester.appState.State.SetDelegatee(addr, pool2)
		caller.contractTester.appState.IdentityState.SetDelegatee(addr, pool2)
	}
	caller.contractTester.appState.Commit(nil, true)

	caller.contractTester.setHeight(4320 + 22)

	//send votes
	for i := 0; i < 30; i++ {
		addr := crypto.PubkeyToAddress(canVoteKeys[i].PublicKey)
		vote := byte(1)
		if i < 14 {
			vote = 2
		}
		err = caller.sendVote(canVoteKeys[i], vote, addr.Bytes())
		caller.contractTester.Commit()
		require.NoError(t, err)
	}
	require.Equal(t, common.ToBytes(uint64(30)), caller.contractTester.ReadData("votedCount"))

	caller.contractTester.setHeight(4320*2 + 22)

	require.NoError(t, caller.finishVoting())

	events := caller.contractTester.env.Commit()
	caller.contractTester.appState.Reset()

	require.NoError(t, caller.finishVoting())
	events2 := caller.contractTester.env.Commit()

	require.Equal(t, events, events2)
	caller.contractTester.appState.Commit(nil, true)

	require.Equal(t, common.ToBytes(oracleVotingStateFinished), caller.contractTester.ReadData("state"))
	require.Equal(t, common.ToBytes(byte(1)), caller.contractTester.ReadData("result"))

	for i := 0; i < 20; i++ {
		addr := crypto.PubkeyToAddress(canVoteKeys[i].PublicKey)
		require.Equal(t, "0", caller.contractTester.appState.State.GetBalance(addr).String())
	}

	balance, _ := big.NewInt(0).SetString("546875000000000000000", 10) // ((3000 + 200*30 - ownerDeposit) / 16)*10^18

	require.Equal(t, balance.String(), caller.contractTester.appState.State.GetBalance(pool1).String())

	for i := 20; i < 30; i++ {
		addr := crypto.PubkeyToAddress(canVoteKeys[i].PublicKey)
		require.Equal(t, balance.String(), caller.contractTester.appState.State.GetBalance(addr).String())
	}

	require.Equal(t, big.NewInt(0).Mul(balance, big.NewInt(5)).String(), caller.contractTester.appState.State.GetBalance(pool2).String())
}

func TestOracleVoting_AllVotesDiscriminated(t *testing.T) {
	deployContractStake := common.DnaBase

	ownerBalance := common.DnaBase

	builder := createTestContractBuilder(&networkConfig{
		identityGroups: []identityGroupConfig{
			{count: 500, state: state.Newbie},
			{count: 500, state: state.Human, pendingUndelegation: true, delegatee: &common.Address{0x1}},
			{count: 1000, state: state.Verified},
		},
	}, ownerBalance)

	tester := builder.Build()
	caller, err := tester.ConfigureDeploy(deployContractStake).OracleVoting().SetPublicVotingDuration(4320).SetVotingDuration(4320).Deploy()
	require.NoError(t, err)

	caller.contractTester.appState.State.SetGlobalEpoch(3)

	caller.contractTester.Commit()
	caller.contractTester.setHeight(3)
	caller.contractTester.setTimestamp(30)

	contractBalance := decimal.NewFromFloat(2000)
	caller.contractTester.SetBalance(ConvertToInt(contractBalance))

	require.NoError(t, caller.StartVoting())
	caller.contractTester.Commit()

	var canVoteKeyIdxs []int
	for i, key := range caller.contractTester.identities {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if _, err := caller.ReadProof(addr); err == nil {
			canVoteKeyIdxs = append(canVoteKeyIdxs, i)
		}
	}

	minPayment := big.NewInt(0).SetBytes(caller.contractTester.ReadData("votingMinPayment"))
	var discriminatedVoteProofs int
	proofSenders := make(map[*ecdsa.PrivateKey]struct{})
	sendVoteProof := func(key *ecdsa.PrivateKey, vote byte) {
		err = caller.sendVoteProof(key, vote, minPayment)
		require.NoError(t, err)
		caller.contractTester.AddBalance(minPayment)
		caller.contractTester.Commit()
		require.NoError(t, err)
		proofSenders[key] = struct{}{}
	}

	caller.contractTester.setHeight(3 + 4320 - 1)
	for _, canVoteKeyIdx := range canVoteKeyIdxs {
		if canVoteKeyIdx >= 999 {
			continue
		}
		key := caller.contractTester.identities[canVoteKeyIdx]
		sendVoteProof(key, 1)
		discriminatedVoteProofs++
	}

	require.Positive(t, discriminatedVoteProofs)

	caller.contractTester.setHeight(3 + 4320)
	for _, canVoteKeyIdx := range canVoteKeyIdxs {
		key := caller.contractTester.identities[canVoteKeyIdx]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		err := caller.sendVote(key, 1, addr.Bytes())
		require.Error(t, err)
		require.Equal(t, "all vote proofs are discriminated", err.Error())
		caller.contractTester.Commit()
	}

	require.Error(t, caller.finishVoting())
	require.NoError(t, caller.prolong())
	caller.contractTester.Commit()

	canVoteKeyIdxs = make([]int, 0)
	for i, key := range caller.contractTester.identities {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if _, err := caller.ReadProof(addr); err == nil {
			canVoteKeyIdxs = append(canVoteKeyIdxs, i)
		}
	}
	require.Positive(t, len(canVoteKeyIdxs))

	caller.contractTester.setHeight(3 + 4320 + 4320 - 1)
	for _, canVoteKeyIdx := range canVoteKeyIdxs {
		key := caller.contractTester.identities[canVoteKeyIdx]
		sendVoteProof(key, 1)
	}

	caller.contractTester.setHeight(3 + 4320 + 4320 + 4320 - 1)
	var discriminatedVotes int
	for _, canVoteKeyIdx := range canVoteKeyIdxs {
		if canVoteKeyIdx >= 999 {
			continue
		}
		key := caller.contractTester.identities[canVoteKeyIdx]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		err := caller.sendVote(key, 1, addr.Bytes())
		require.NoError(t, err)
		caller.contractTester.Commit()
		discriminatedVotes++
		delete(proofSenders, key)
	}
	require.Positive(t, discriminatedVotes)

	caller.contractTester.setHeight(3 + 4320 + 4320 + 4320)
	err = caller.finishVoting()
	require.Error(t, err)
	require.Equal(t, "all votes are discriminated", err.Error())
	require.NoError(t, caller.prolong())
	caller.contractTester.Commit()

	caller.contractTester.setHeight(3 + 4320 + 4320 + 4320 + 4320)
	for key := range proofSenders {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		err := caller.sendVote(key, 1, addr.Bytes())
		require.NoError(t, err)
		caller.contractTester.Commit()
	}

	err = caller.prolong()
	require.Error(t, err)
	require.NoError(t, caller.finishVoting())
	caller.contractTester.Commit()
}

func TestOracleVoting_successScenarioWithPoolsAndDiscriminations(t *testing.T) {
	ownerBalance := common.DnaBase
	builder := createTestContractBuilder(&networkConfig{
		[]identityGroupConfig{
			{
				count: 100,
				state: state.Human,
			},
			{
				count: 100,
				state: state.Newbie,
			},
			{
				count:               100,
				state:               state.Human,
				delegatee:           &common.Address{0x1},
				pendingUndelegation: true,
			},
			{
				count:     100,
				state:     state.Verified,
				delegatee: &common.Address{0x2},
			},
			{
				count:     100,
				state:     state.Newbie,
				delegatee: &common.Address{0x2},
			},
		},
	}, ownerBalance)

	deployContractStake := common.DnaBase
	tester := builder.Build()
	caller, err := tester.ConfigureDeploy(deployContractStake).OracleVoting().SetCommitteeSize(50).Deploy()
	require.NoError(t, err)

	caller.contractTester.appState.State.SetGlobalEpoch(3)

	caller.contractTester.Commit()

	expectedOwnerDeposit := ConvertToInt(decimal.RequireFromString("500"))
	require.Equal(t, expectedOwnerDeposit.Bytes(), caller.contractTester.ReadData("ownerDeposit"))

	caller.contractTester.setHeight(3)
	caller.contractTester.setTimestamp(30)
	contractBalance := decimal.NewFromFloat(2000)
	caller.contractTester.SetBalance(ConvertToInt(contractBalance))

	require.NoError(t, caller.StartVoting())
	caller.contractTester.Commit()

	var humansToVote, newbiesToVote, undelegatedHumansToVote, delegatedVerifiedToVote, delegatedNewbiesToVote []int
	for i, key := range caller.contractTester.identities {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if _, err := caller.ReadProof(addr); err == nil {
			switch {
			case i < 99:
				humansToVote = append(humansToVote, i)
				break
			case i < 199:
				newbiesToVote = append(newbiesToVote, i)
				break
			case i < 299:
				undelegatedHumansToVote = append(undelegatedHumansToVote, i)
				break
			case i < 399:
				delegatedVerifiedToVote = append(delegatedVerifiedToVote, i)
				break
			case i < 499:
				delegatedNewbiesToVote = append(delegatedNewbiesToVote, i)
				break
			}
		}
	}

	minPayment := big.NewInt(0).SetBytes(caller.contractTester.ReadData("votingMinPayment"))
	sendVoteProof := func(key *ecdsa.PrivateKey, vote byte) {
		err = caller.sendVoteProof(key, vote, minPayment)
		require.NoError(t, err)
		caller.contractTester.AddBalance(minPayment)
		caller.contractTester.Commit()
		require.NoError(t, err)
	}
	caller.contractTester.setHeight(3 + 30 - 1)
	for i, keyIndex := range humansToVote {
		var vote byte
		if i < len(humansToVote)/3 {
			vote = 1
		} else {
			vote = 2
		}
		key := caller.contractTester.identities[keyIndex]
		sendVoteProof(key, vote)
	}
	for i, keyIndex := range newbiesToVote {
		var vote byte
		if i < len(newbiesToVote)/3 {
			vote = 2
		} else {
			vote = 1
		}
		key := caller.contractTester.identities[keyIndex]
		sendVoteProof(key, vote)
	}
	for i, keyIndex := range undelegatedHumansToVote {
		var vote byte
		if i < len(undelegatedHumansToVote)/3 {
			vote = 2
		} else {
			vote = 1
		}
		key := caller.contractTester.identities[keyIndex]
		sendVoteProof(key, vote)
	}
	for i, keyIndex := range delegatedVerifiedToVote {
		var vote byte
		if i < len(delegatedVerifiedToVote)/3 {
			vote = 1
		} else {
			vote = 2
		}
		key := caller.contractTester.identities[keyIndex]
		sendVoteProof(key, vote)
	}
	for i, keyIndex := range delegatedNewbiesToVote {
		var vote byte
		if i < len(delegatedNewbiesToVote)/3 {
			vote = 2
		} else {
			vote = 1
		}
		key := caller.contractTester.identities[keyIndex]
		sendVoteProof(key, vote)
	}

	caller.contractTester.setHeight(3 + 30)
	winnerVotesCnt := 0
	poolWinnerVotesCnt := 0
	for i, keyIndex := range humansToVote {
		var vote byte
		if i < len(humansToVote)/3 {
			vote = 1
		} else {
			vote = 2
			winnerVotesCnt++
		}
		key := caller.contractTester.identities[keyIndex]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		err := caller.sendVote(key, vote, addr.Bytes())
		require.NoError(t, err)
		caller.contractTester.Commit()
	}
	for i, keyIndex := range newbiesToVote {
		var vote byte
		if i < len(newbiesToVote)/3 {
			vote = 2
			winnerVotesCnt++
		} else {
			vote = 1
		}
		key := caller.contractTester.identities[keyIndex]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		err := caller.sendVote(key, vote, addr.Bytes())
		require.NoError(t, err)
		caller.contractTester.Commit()
	}
	for i, keyIndex := range undelegatedHumansToVote {
		var vote byte
		if i < len(undelegatedHumansToVote)/3 {
			vote = 2
			winnerVotesCnt++
		} else {
			vote = 1
		}
		key := caller.contractTester.identities[keyIndex]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		err := caller.sendVote(key, vote, addr.Bytes())
		require.NoError(t, err)
		caller.contractTester.Commit()
	}
	for i, keyIndex := range delegatedVerifiedToVote {
		var vote byte
		if i < len(delegatedVerifiedToVote)/3 {
			vote = 1
		} else {
			vote = 2
			winnerVotesCnt++
			poolWinnerVotesCnt++
		}
		key := caller.contractTester.identities[keyIndex]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		err := caller.sendVote(key, vote, addr.Bytes())
		require.NoError(t, err)
		caller.contractTester.Commit()
	}
	for i, keyIndex := range delegatedNewbiesToVote {
		var vote byte
		if i < len(delegatedNewbiesToVote)/3 {
			vote = 2
			winnerVotesCnt++
			poolWinnerVotesCnt++
		} else {
			vote = 1
		}
		key := caller.contractTester.identities[keyIndex]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		err := caller.sendVote(key, vote, addr.Bytes())
		require.NoError(t, err)
		caller.contractTester.Commit()
	}

	expectedPrize := new(big.Int).Div(new(big.Int).Sub(caller.contractTester.appState.State.GetBalance(caller.contractTester.contractAddr), expectedOwnerDeposit), big.NewInt(int64(winnerVotesCnt)))

	caller.contractTester.setHeight(3 + 30 + 100)
	require.NoError(t, caller.finishVoting())
	caller.contractTester.Commit()

	require.Equal(t, common.ToBytes(byte(2)), caller.contractTester.ReadData("result"))

	for i, keyIndex := range humansToVote {
		key := caller.contractTester.identities[keyIndex]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if i < len(humansToVote)/3 {
			require.Zero(t, caller.contractTester.appState.State.GetBalance(addr).Sign())
		} else {
			require.Equal(t, expectedPrize, caller.contractTester.appState.State.GetBalance(addr))
		}
	}
	for i, keyIndex := range newbiesToVote {
		key := caller.contractTester.identities[keyIndex]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if i < len(newbiesToVote)/3 {
			require.Equal(t, expectedPrize, caller.contractTester.appState.State.GetBalance(addr))
		} else {
			require.Zero(t, caller.contractTester.appState.State.GetBalance(addr).Sign())
		}
	}
	for i, keyIndex := range undelegatedHumansToVote {
		key := caller.contractTester.identities[keyIndex]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if i < len(undelegatedHumansToVote)/3 {
			require.Equal(t, expectedPrize, caller.contractTester.appState.State.GetBalance(addr))
		} else {
			require.Zero(t, caller.contractTester.appState.State.GetBalance(addr).Sign())
		}
	}
	for _, keyIndex := range delegatedVerifiedToVote {
		key := caller.contractTester.identities[keyIndex]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		require.Zero(t, caller.contractTester.appState.State.GetBalance(addr).Sign())
	}
	for _, keyIndex := range delegatedNewbiesToVote {
		key := caller.contractTester.identities[keyIndex]
		addr := crypto.PubkeyToAddress(key.PublicKey)
		require.Zero(t, caller.contractTester.appState.State.GetBalance(addr).Sign())
	}
	require.Equal(t, new(big.Int).Mul(expectedPrize, big.NewInt(int64(poolWinnerVotesCnt))), caller.contractTester.appState.State.GetBalance(common.Address{0x2}))

	caller.contractTester.setHeight(3 + 30 + 100 + 4 + 30240)
	_, err = caller.contractTester.Terminate(caller.contractTester.mainKey, OracleVotingContract)
	require.NoError(t, err)

	caller.contractTester.Commit()

	require.Nil(t, caller.contractTester.ReadData("state"))
	require.Equal(t, []byte{0x1}, caller.contractTester.ReadData("fact"))
	require.Equal(t, []byte{0x2}, caller.contractTester.ReadData("result"))
}

func TestOracleVoting_finishVotingWithZeroSecretVotesDuringPublicVoting(t *testing.T) {

	deployContractStake := common.DnaBase

	ownerBalance := common.DnaBase

	builder := createTestContractBuilder(&networkConfig{
		identityGroups: []identityGroupConfig{
			{count: 1000, state: state.Verified},
		},
	}, ownerBalance)
	tester := builder.Build()
	ownerFee := byte(5)
	caller, err := tester.ConfigureDeploy(deployContractStake).OracleVoting().
		SetQuorum(5).
		SetOwnerFee(ownerFee).
		SetPublicVotingDuration(4320).SetVotingDuration(4320).Deploy()
	require.NoError(t, err)

	caller.contractTester.Commit()
	caller.contractTester.setHeight(3)
	caller.contractTester.setTimestamp(30)

	contractBalance := decimal.NewFromFloat(2000)
	caller.contractTester.SetBalance(ConvertToInt(contractBalance))

	require.NoError(t, caller.StartVoting())
	caller.contractTester.Commit()

	// send proofs
	winnerVote := byte(1)
	var votedIdentities []*ecdsa.PrivateKey
	minPayment := big.NewInt(0).SetBytes(caller.contractTester.ReadData("votingMinPayment"))

	sendVoteProof := func(key *ecdsa.PrivateKey) {

		caller.contractTester.setHeight(4)

		err = caller.sendVoteProof(key, winnerVote, minPayment)
		if err == nil {
			caller.contractTester.AddBalance(minPayment)
			caller.contractTester.Commit()
			require.NoError(t, err)
			votedIdentities = append(votedIdentities, key)
		}
	}
	for _, key := range caller.contractTester.identities {
		sendVoteProof(key)
		if len(votedIdentities) == 5 {
			break
		}
	}

	//send votes
	for i := 0; i < len(votedIdentities)-1; i++ {
		key := votedIdentities[i]
		caller.contractTester.setHeight(4320 + 1000)
		addr := crypto.PubkeyToAddress(key.PublicKey)
		require.NoError(t, caller.sendVote(key, winnerVote, addr.Bytes()))
		caller.contractTester.Commit()
	}
	prolongRes := caller.prolong()
	require.Error(t, prolongRes)
	require.Equal(t, "voting can not be prolonged", prolongRes.Error())
	require.Error(t, caller.finishVoting())

	key := votedIdentities[len(votedIdentities)-1]
	caller.contractTester.setHeight(4320 + 1000)
	addr := crypto.PubkeyToAddress(key.PublicKey)
	require.NoError(t, caller.sendVote(key, winnerVote, addr.Bytes()))
	caller.contractTester.Commit()

	prolongRes = caller.prolong()
	require.Error(t, prolongRes)
	require.Equal(t, "voting can not be prolonged", prolongRes.Error())
	require.NoError(t, caller.finishVoting())
}
