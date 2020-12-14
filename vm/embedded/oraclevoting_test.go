package embedded

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/idena-network/idena-go/vm/helpers"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	dbm "github.com/tendermint/tm-db"
	"math"
	"math/big"
	"math/rand"
	"testing"
)

func getDbSize(db *dbm.MemDB) int {
	it, _ := db.Iterator(nil, nil)
	size := 0
	for ; it.Valid(); it.Next() {
		size += len(it.Key()) + len(it.Value())
	}
	it.Close()
	return size
}

func printDbSize(db *dbm.MemDB, descr string) {
	fmt.Printf("%v: size of memDB=%v (key+value)\n", descr, getDbSize(db))
}

func createHeader(height uint64, time int64) *types.Header {
	seed := types.Seed{}
	seed.SetBytes(common.ToBytes(height))
	return &types.Header{
		ProposedHeader: &types.ProposedHeader{
			BlockSeed: seed,
			Height:    height,
			Time:      time,
		},
	}
}

type contractTester struct {
	db       dbm.DB
	mainKey  *ecdsa.PrivateKey
	mainAddr common.Address

	appState *appstate.AppState
	secStore *secstore.SecStore

	initialOwnerContractBalance *big.Int
}

type contractTesterBuilder struct {
	networkSize                 int
	initialOwnerContractBalance *big.Int
}

func createTestContractBuilder(networkSize int) *contractTesterBuilder {
	return &contractTesterBuilder{networkSize: networkSize, initialOwnerContractBalance: common.DnaBase}
}

func (b *contractTesterBuilder) SetInitialOwnerContractBalance(balance *big.Int) *contractTesterBuilder {
	b.initialOwnerContractBalance = balance
	return b
}

func (b *contractTesterBuilder) Build() *contractTester {

	db := dbm.NewMemDB()
	appState := appstate.NewAppState(db, eventbus.New())

	appState.State.SetFeePerGas(big.NewInt(1))
	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)
	addr := crypto.PubkeyToAddress(key.PublicKey)
	appState.State.SetState(addr, state.Newbie)
	appState.State.SetBalance(addr, b.initialOwnerContractBalance)
	appState.State.SetPubKey(addr, crypto.FromECDSAPub(&key.PublicKey))
	appState.IdentityState.Add(addr)

	secStore := secstore.NewSecStore()

	var identities []*ecdsa.PrivateKey

	for i := 0; i < b.networkSize-1; i++ {
		key, _ := crypto.GenerateKeyFromSeed(rnd)
		identities = append(identities, key)
		addr := crypto.PubkeyToAddress(key.PublicKey)
		appState.State.SetState(addr, state.Newbie)
		appState.State.SetPubKey(addr, crypto.FromECDSAPub(&key.PublicKey))
		appState.IdentityState.Add(addr)
	}
	appState.Commit(nil)

	appState.Initialize(1)
	return &contractTester{
		db:                          db,
		appState:                    appState,
		mainKey:                     key,
		mainAddr:                    addr,
		initialOwnerContractBalance: b.initialOwnerContractBalance,
		secStore:                    secStore,
	}
}

type deployContractSwitch struct {
	contractTester *contractTester
	deployStake    *big.Int
}

func (s *deployContractSwitch) OracleVoting() *configurableOracleVotingDeploy {
	return &configurableOracleVotingDeploy{
		contractTester: s.contractTester,
		deployStake:    s.deployStake,
		fact:           []byte{0x1}, startTime: 10, committeeSize: 100, ownerFee: 0,
		publicVotingDuration: 30,
		votingDuration:       30,
		quorum:               20,
		winnerThreshold:      51,
	}
}

type configurableDeploy interface {
	Parameters() (contract EmbeddedContractType, deployStake *big.Int, params [][]byte)
}

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

func (c *configurableOracleVotingDeploy) Deploy() (*oracleVotingCaller, error) {
	if err := c.contractTester.Deploy(c); err != nil {
		return nil, err
	}
	return &oracleVotingCaller{
	}, nil
}

func (c *contractTester) ConfigureDeploy(deployStake *big.Int) *deployContractSwitch {
	return &deployContractSwitch{contractTester: c, deployStake: deployStake}
}

func (c *contractTester) createContract(ctx env.CallContext, e env.Env) Contract {
	switch ctx.CodeHash() {
	case TimeLockContract:
		return NewTimeLock(ctx, e, nil)
	case OracleVotingContract:
		return NewOracleVotingContract(ctx, e, nil)
	case EvidenceLockContract:
		return NewOracleLock(ctx, e, nil)
	case RefundableEvidenceLockContract:
		return NewRefundableEvidenceLock(ctx, e)
	case MultisigContract:
		return NewMultisig(ctx, e)
	default:
		return nil
	}
}

func (c *contractTester) Deploy(config configurableDeploy) error {

	contractType, deployStake, deployParams := config.Parameters()

	attachment := attachments.CreateDeployContractAttachment(contractType, deployParams...)
	payload, err := attachment.ToBytes()
	if err != nil {
		return err
	}

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: 1,
		Type:         types.DeployContract,
		Amount:       deployStake,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, c.mainKey)
	ctx := env.NewDeployContextImpl(tx, attachment.CodeHash)

	gas := new(env.GasCounter)
	gas.Reset(-1)

	// deploy
	e := env.NewEnvImp(c.appState, createHeader(2, 1), gas, c.secStore, nil)

	return c.createContract(ctx, e).Deploy(attachment.Args...)
}

func (c *contractTester) Call(contract EmbeddedContractType) {

}

type oracleVotingCaller struct {
	contractTester *contractTester
}

func (c *oracleVotingCaller) StartVoting() error {
	c.contractTester.Call(OracleVotingContract)
	return nil
}

func TestOracleVoting_scenario_0(t *testing.T) {
	deployContractStake := common.DnaBase

	builder := createTestContractBuilder(2000)
	tester := builder.Build()
	caller, err := tester.ConfigureDeploy(deployContractStake).OracleVoting().SetOwnerFee(5).Deploy()

	require.NoError(t, err)

	caller.StartVoting()
}


func TestFactChecking_Call(t *testing.T) {
	db := dbm.NewMemDB()
	appState := appstate.NewAppState(db, eventbus.New())

	initialBalance := common.DnaBase

	appState.State.SetFeePerGas(big.NewInt(1))
	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)
	addr := crypto.PubkeyToAddress(key.PublicKey)
	appState.State.SetState(addr, state.Newbie)
	appState.State.SetBalance(addr, initialBalance)
	appState.State.SetPubKey(addr, crypto.FromECDSAPub(&key.PublicKey))
	appState.IdentityState.Add(addr)

	secStore := secstore.NewSecStore()

	var identities []*ecdsa.PrivateKey

	for i := 0; i < 2000; i++ {
		key, _ := crypto.GenerateKeyFromSeed(rnd)
		identities = append(identities, key)
		addr := crypto.PubkeyToAddress(key.PublicKey)
		appState.State.SetState(addr, state.Newbie)
		appState.State.SetPubKey(addr, crypto.FromECDSAPub(&key.PublicKey))
		appState.IdentityState.Add(addr)
	}
	appState.Commit(nil)

	appState.Initialize(1)

	printDbSize(db, "Before contract deploy")

	ownerFee := byte(5)

	attachment := attachments.CreateDeployContractAttachment(OracleVotingContract, []byte{0x1}, common.ToBytes(uint64(10)),
		nil, nil, nil, nil, nil, nil, common.ToBytes(ownerFee))
	payload, err := attachment.ToBytes()
	require.NoError(t, err)

	deployContractStake := common.DnaBase

	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: 1,
		Type:         types.DeployContract,
		Amount:       deployContractStake,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)
	ctx := env.NewDeployContextImpl(tx, attachment.CodeHash)

	gas := new(env.GasCounter)
	gas.Reset(-1)

	// deploy
	e := env.NewEnvImp(appState, createHeader(2, 1), gas, secStore, nil)
	contract := NewOracleVotingContract(ctx, e, nil)

	require.NoError(t, contract.Deploy(attachment.Args...))
	e.Commit()
	contractAddr := ctx.ContractAddr()

	appState.Commit(nil)
	printDbSize(db, "After contract deploy")
	fmt.Printf("Deploy gas: %v\n", gas.UsedGas)
	gas.Reset(-1)
	// start voting

	callAttach := attachments.CreateCallContractAttachment("startVoting")
	payload, _ = callAttach.ToBytes()
	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &contractAddr,
		Type:         types.CallContract,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)

	txNotOwner, _ := types.SignTx(tx, identities[0])

	e = env.NewEnvImp(appState, createHeader(3, 21), gas, secStore, nil)
	contract = NewOracleVotingContract(env.NewCallContextImpl(txNotOwner, OracleVotingContract), e, nil)

	require.Error(t, contract.Call("startVoting"))
	e.Reset()
	gas.Reset(-1)
	e = env.NewEnvImp(appState, createHeader(3, 21), gas, secStore, nil)
	contract = NewOracleVotingContract(env.NewCallContextImpl(tx, OracleVotingContract), e, nil)

	require.Error(t, contract.Call("startVoting"))
	e.Reset()
	gas.Reset(-1)

	contractBalance := big.NewInt(0).Mul(common.DnaBase, big.NewInt(2000))

	appState.State.SetBalance(contractAddr, contractBalance)

	require.NoError(t, contract.Call("startVoting"))
	e.Commit()

	appState.Commit(nil)
	printDbSize(db, "After start voting")
	fmt.Printf("Start voting gas: %v\n", gas.UsedGas)
	gas.Reset(-1)

	require.Equal(t, common.ToBytes(uint64(1)), appState.State.GetContractValue(contractAddr, []byte("state")))
	require.Equal(t, big.NewInt(0).Quo(contractBalance, big.NewInt(20)).Bytes(), appState.State.GetContractValue(contractAddr, []byte("votingMinPayment")))

	seed := types.Seed{}
	seed.SetBytes(common.ToBytes(uint64(3)))
	require.Equal(t, seed.Bytes(), appState.State.GetContractValue(contractAddr, []byte("vrfSeed")))
	require.Equal(t, common.ToBytes(uint64(100)), appState.State.GetContractValue(contractAddr, []byte("committeeSize")))

	// send proofs
	winnerVote := byte(1)
	votedIdentities := map[common.Address]struct{}{}
	minPayment := big.NewInt(0).SetBytes(appState.State.GetContractValue(contractAddr, []byte("votingMinPayment")))

	voted := 0
	for _, key := range identities {

		pubBytes := crypto.FromECDSAPub(&key.PublicKey)

		addr := crypto.PubkeyToAddress(key.PublicKey)

		hash := crypto.Hash(append(pubBytes, appState.State.GetContractValue(contractAddr, []byte("vrfSeed"))...))

		v := new(big.Float).SetInt(new(big.Int).SetBytes(hash[:]))

		q := new(big.Float).Quo(v, maxHash)

		threshold, _ := helpers.ExtractUInt64(0, appState.State.GetContractValue(contractAddr, []byte("committeeSize")))
		shouldBeError := false
		if q.Cmp(big.NewFloat(1-float64(threshold)/float64(appState.ValidatorsCache.NetworkSize()))) < 0 {
			shouldBeError = true
		}
		_, err = contract.Read("proof", addr.Bytes())
		require.Equal(t, shouldBeError, err != nil)

		salt := addr.Bytes()
		voteHash, err := contract.Read("voteHash", common.ToBytes(winnerVote), salt)
		require.Nil(t, err)

		callAttach = attachments.CreateCallContractAttachment("sendVoteProof", voteHash[:])
		payload, _ = callAttach.ToBytes()
		tx = &types.Transaction{
			Epoch:        0,
			AccountNonce: 2,
			To:           &contractAddr,
			Type:         types.CallContract,
			Payload:      payload,
			Amount:       minPayment,
		}
		tx, _ = types.SignTx(tx, key)
		gas.Reset(-1)
		e = env.NewEnvImp(appState, createHeader(4, 21), gas, secStore, nil)
		contract = NewOracleVotingContract(env.NewCallContextImpl(tx, OracleVotingContract), e, nil)
		err = contract.Call(callAttach.Method, callAttach.Args...)
		if shouldBeError {
			e.Reset()
			require.Error(t, err)
		} else {
			voted++
			e.Commit()
			appState.State.AddBalance(contractAddr, minPayment)
			appState.Commit(nil)
			require.NoError(t, err)
			votedIdentities[addr] = struct{}{}
		}
	}
	appState.Commit(nil)

	printDbSize(db, "After send vote proofs")
	gas.Reset(-1)

	//send votes

	for _, key := range identities {
		addr := crypto.PubkeyToAddress(key.PublicKey)

		salt := addr.Bytes()
		callAttach = attachments.CreateCallContractAttachment("sendVote", common.ToBytes(winnerVote), salt)
		payload, _ = callAttach.ToBytes()
		tx = &types.Transaction{
			Epoch:        0,
			AccountNonce: 2,
			To:           &contractAddr,
			Type:         types.CallContract,
			Payload:      payload,
		}
		gas.Reset(-1)
		tx, _ = types.SignTx(tx, key)
		e = env.NewEnvImp(appState, createHeader(4320*2, 21), gas, secStore, nil)
		contract = NewOracleVotingContract(env.NewCallContextImpl(tx, OracleVotingContract), e, nil)

		err := contract.Call(callAttach.Method, callAttach.Args...)
		if _, ok := votedIdentities[addr]; !ok {
			e.Reset()
			require.Error(t, err)
		} else {
			e.Commit()
			appState.Commit(nil)
			require.NoError(t, err)
		}
	}

	appState.Commit(nil)
	printDbSize(db, "After send open votes")
	gas.Reset(-1)

	callAttach = attachments.CreateCallContractAttachment("finishVoting")
	payload, _ = callAttach.ToBytes()
	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &contractAddr,
		Type:         types.CallContract,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)
	gas.Reset(-1)
	e = env.NewEnvImp(appState, createHeader(4320*2, 21), gas, secStore, nil)
	contract = NewOracleVotingContract(env.NewCallContextImpl(tx, OracleVotingContract), e, nil)

	require.NoError(t, contract.Call(callAttach.Method, callAttach.Args...))
	e.Commit()
	appState.Commit(nil)
	printDbSize(db, "After finishVoting")

	for addr := range votedIdentities {
		b := appState.State.GetBalance(addr)
		require.Equal(t, "120652173913043478260", b.String())
	}

	stakeAfterFinish := appState.State.GetContractStake(contractAddr)

	require.Equal(t, 0, big.NewInt(0).Add(deployContractStake, big.NewInt(0).Quo(contractBalance, big.NewInt(int64(100/ownerFee)))).Cmp(stakeAfterFinish))

	fmt.Printf("Finish voting gas: %v\n", gas.UsedGas)

	gas.Reset(-1)
	require.Equal(t, []byte{winnerVote}, appState.State.GetContractValue(contractAddr, []byte("result")))

	for addr, _ := range votedIdentities {
		require.True(t, appState.State.GetBalance(addr).Sign() == 1)
	}
	require.True(t, appState.State.GetBalance(contractAddr).Sign() == 0)

	//terminate
	terminateAttach := attachments.CreateTerminateContractAttachment()
	payload, _ = terminateAttach.ToBytes()
	tx = &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &contractAddr,
		Type:         types.TerminateContract,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)

	e = env.NewEnvImp(appState, createHeader(4320*3+4+30240, 21), gas, secStore, nil)
	contract = NewOracleVotingContract(env.NewCallContextImpl(tx, OracleVotingContract), e, nil)
	require.NoError(t, contract.Terminate(terminateAttach.Args...))
	e.Commit()

	appState.Commit(nil)
	printDbSize(db, "After terminating")
	fmt.Printf("Terminating gas: %v\n", gas.UsedGas)

	stakeToBalance := big.NewInt(0).Quo(stakeAfterFinish, big.NewInt(2))

	require.Equal(t, appState.State.GetBalance(addr).Bytes(), big.NewInt(0).Add(initialBalance, stakeToBalance).Bytes())
	require.Nil(t, appState.State.GetCodeHash(contractAddr))
	require.Equal(t, 0, appState.State.GetStakeBalance(contractAddr).Sign())

	require.Nil(t, appState.State.GetContractValue(contractAddr, []byte("vrfSeed")))
}

func Test_minOracleReward(t *testing.T) {

	cases := []struct {
		percent int
		network int
		reward  float64
	}{
		{percent: 1, network: 1000, reward: 1.0},
		{percent: 1, network: 3400, reward: 1.0},
		{percent: 50, network: 5000, reward: 3.0379},
		{percent: 50, network: 1000, reward: 2.5428},
		{percent: 100, network: 1000, reward: 3.0},
		{percent: 90, network: 100000, reward: 4.8192},
	}

	for _, c := range cases {
		f, _ := decimal.NewFromBigInt(minOracleReward(uint64(c.percent*c.network/100), c.network), -18).Float64()
		require.Equal(t, c.reward, math.Round(f*10000)/10000)
	}

}
