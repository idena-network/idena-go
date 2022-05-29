package embedded

import (
	"crypto/ecdsa"
	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/vm/env"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
	"math/big"
	"math/rand"
)

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
	contractAddr                common.Address
	identities                  []*ecdsa.PrivateKey
	env                         *env.EnvImp
	contractInstance            Contract
	height                      uint64
	timestamp                   int64
}

type contractTesterBuilder struct {
	network                     networkConfig
	initialOwnerContractBalance *big.Int
}

type networkConfig struct {
	identityGroups []identityGroupConfig
}

type identityGroupConfig struct {
	count               int
	state               state.IdentityState
	delegatee           *common.Address
	pendingUndelegation bool
}

func createTestContractBuilder(network *networkConfig, ownerBalance *big.Int) *contractTesterBuilder {
	var convertedNetwork networkConfig
	if network != nil {
		for _, identityGroup := range network.identityGroups {
			for i := 0; i < identityGroup.count; i++ {
				convertedNetwork.identityGroups = append(convertedNetwork.identityGroups, identityGroupConfig{
					state:               identityGroup.state,
					pendingUndelegation: identityGroup.pendingUndelegation,
					delegatee:           identityGroup.delegatee,
				})
			}
		}
	}
	return &contractTesterBuilder{network: convertedNetwork, initialOwnerContractBalance: ownerBalance}
}

func (b *contractTesterBuilder) SetInitialOwnerContractBalance(balance *big.Int) *contractTesterBuilder {
	b.initialOwnerContractBalance = balance
	return b
}

func (b *contractTesterBuilder) getIdentityConfig(i int) (identityGroupConfig, bool) {
	if len(b.network.identityGroups) <= i {
		return identityGroupConfig{}, false
	}
	return b.network.identityGroups[i], true
}

func (b *contractTesterBuilder) Build() *contractTester {

	db := dbm.NewMemDB()
	appState, _ := appstate.NewAppState(db, eventbus.New())

	appState.State.SetFeePerGas(big.NewInt(1))
	rnd := rand.New(rand.NewSource(1))
	key, _ := crypto.GenerateKeyFromSeed(rnd)
	addr := crypto.PubkeyToAddress(key.PublicKey)
	appState.State.SetBalance(addr, b.initialOwnerContractBalance)
	appState.State.SetPubKey(addr, crypto.FromECDSAPub(&key.PublicKey))
	if cfg, ok := b.getIdentityConfig(0); ok {
		appState.State.SetState(addr, cfg.state)
		if cfg.state.NewbieOrBetter() {
			appState.IdentityState.SetValidated(addr, true)
		}
		if cfg.pendingUndelegation {
			appState.State.SetPendingUndelegation(addr)
		}
		if cfg.delegatee != nil {
			appState.State.SetDelegatee(addr, *cfg.delegatee)
			if !cfg.pendingUndelegation {
				appState.IdentityState.SetDelegatee(addr, *cfg.delegatee)
			}
		}
	} else {
		appState.State.SetState(addr, state.Newbie)
		appState.IdentityState.SetValidated(addr, true)
	}

	secStore := secstore.NewSecStore()

	var identities []*ecdsa.PrivateKey

	for i := 1; i < len(b.network.identityGroups); i++ {
		key, _ := crypto.GenerateKeyFromSeed(rnd)
		identities = append(identities, key)
		addr := crypto.PubkeyToAddress(key.PublicKey)
		appState.State.SetPubKey(addr, crypto.FromECDSAPub(&key.PublicKey))
		cfg := b.network.identityGroups[i]
		appState.State.SetState(addr, cfg.state)
		if cfg.state.NewbieOrBetter() {
			appState.IdentityState.SetValidated(addr, true)
		}
		if cfg.pendingUndelegation {
			appState.State.SetPendingUndelegation(addr)
		}
		if cfg.delegatee != nil {
			appState.State.SetDelegatee(addr, *cfg.delegatee)
			if !cfg.pendingUndelegation {
				appState.IdentityState.SetDelegatee(addr, *cfg.delegatee)
			}
		}
	}
	appState.Commit(nil, true)

	appState.Initialize(1)
	return &contractTester{
		db:                          db,
		appState:                    appState,
		mainKey:                     key,
		mainAddr:                    addr,
		initialOwnerContractBalance: b.initialOwnerContractBalance,
		secStore:                    secStore,
		identities:                  identities,
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

func (c *contractTester) ConfigureDeploy(deployStake *big.Int) *deployContractSwitch {
	return &deployContractSwitch{contractTester: c, deployStake: deployStake}
}

func (c *contractTester) createContract(ctx env.CallContext, e env.Env) Contract {
	switch ctx.CodeHash() {
	case TimeLockContract:
		return NewTimeLock(ctx, e, nil)
	case OracleVotingContract:
		return NewOracleVotingContract5(ctx, e, nil)
	case OracleLockContract:
		return NewOracleLock2(ctx, e, nil)
	case RefundableOracleLockContract:
		return NewRefundableOracleLock2(ctx, e, nil)
	case MultisigContract:
		return NewMultisig(ctx, e, nil)
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
		Type:         types.DeployContractTx,
		Amount:       deployStake,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, c.mainKey)
	ctx := env.NewDeployContextImpl(tx, nil, attachment.CodeHash)

	gas := new(env.GasCounter)
	gas.Reset(-1)

	// deploy
	c.env = env.NewEnvImp(c.appState, createHeader(2, 1), gas, nil)
	c.contractAddr = ctx.ContractAddr()
	c.contractInstance = c.createContract(ctx, c.env)
	err = c.contractInstance.Deploy(attachment.Args...)
	c.env.Deploy(ctx)
	return err
}

func (c *contractTester) Call(key *ecdsa.PrivateKey, contract EmbeddedContractType, payment *big.Int, method string, args ...[]byte) error {
	callAttach := attachments.CreateCallContractAttachment(method, args...)
	payload, _ := callAttach.ToBytes()
	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &c.contractAddr,
		Type:         types.CallContractTx,
		Payload:      payload,
		Amount:       payment,
	}
	tx, _ = types.SignTx(tx, key)
	gas := new(env.GasCounter)
	gas.Reset(-1)

	ctx := env.NewCallContextImpl(tx, nil, contract)

	c.env = env.NewEnvImp(c.appState, createHeader(c.height, c.timestamp), gas, nil)
	c.contractInstance = c.createContract(ctx, c.env)
	return c.contractInstance.Call(callAttach.Method, callAttach.Args...)
}

func (c *contractTester) Terminate(key *ecdsa.PrivateKey, contract EmbeddedContractType) (common.Address, error) {
	terminateAttach := attachments.CreateTerminateContractAttachment()
	payload, _ := terminateAttach.ToBytes()
	tx := &types.Transaction{
		Epoch:        0,
		AccountNonce: 2,
		To:           &c.contractAddr,
		Type:         types.TerminateContractTx,
		Payload:      payload,
	}
	tx, _ = types.SignTx(tx, key)

	gas := new(env.GasCounter)
	gas.Reset(-1)

	ctx := env.NewCallContextImpl(tx, nil, contract)

	c.env = env.NewEnvImp(c.appState, createHeader(c.height, c.timestamp), gas, nil)
	c.contractInstance = c.createContract(ctx, c.env)
	dest, keysToSave, err := c.contractInstance.Terminate(terminateAttach.Args...)
	if err == nil {
		c.env.Terminate(ctx, keysToSave, dest)
	}
	return dest, err
}

func (c *contractTester) OwnerCall(contract EmbeddedContractType, method string, args ...[]byte) error {
	return c.Call(c.mainKey, contract, nil, method, args...)
}

func (c *contractTester) IdentityCall(identityIndex int, contract EmbeddedContractType, method string, args ...[]byte) error {
	return c.Call(c.identities[identityIndex], contract, nil, method, args...)
}

func (c *contractTester) Read(contract EmbeddedContractType, method string, bytes ...[]byte) ([]byte, error) {
	if c.contractInstance == nil {
		return nil, errors.New("no contract instance")
	}
	return c.contractInstance.Read(method, bytes...)
}

func (c *contractTester) Commit() {
	c.env.Commit()
	c.appState.Commit(nil, true)
}

func (c *contractTester) SetBalance(balance *big.Int) {
	c.appState.State.SetBalance(c.contractAddr, balance)
}

func (c *contractTester) AddBalance(amount *big.Int) {
	c.appState.State.AddBalance(c.contractAddr, amount)
}

func (c *contractTester) ReadData(key string) []byte {
	return c.appState.State.GetContractValue(c.contractAddr, []byte(key))
}

func (c *contractTester) setHeight(h uint64) {
	c.height = h
}

func (c *contractTester) setTimestamp(timestamp int64) {
	c.timestamp = timestamp
}

func (c *contractTester) ContractStake() *big.Int {
	return c.appState.State.GetContractStake(c.contractAddr)
}

func (c *contractTester) ContractBalance() *big.Int {
	return c.appState.State.GetBalance(c.contractAddr)
}

func (c *contractTester) CodeHash() *common.Hash {
	return c.appState.State.GetCodeHash(c.contractAddr)
}
