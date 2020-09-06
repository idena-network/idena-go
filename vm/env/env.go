package env

import (
	"bytes"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
	"github.com/idena-network/idena-go/stats/collector"
	"github.com/pkg/errors"
	"math/big"
	"regexp"
	"sort"
)

const (
	maxEnvKeyLength = 32
)

var (
	eventRegexp *regexp.Regexp
)

func init() {
	// any ASCII character
	eventRegexp, _ = regexp.Compile("^[\x00-\x7F]{1,32}$")
}

type Env interface {
	BlockNumber() uint64
	BlockTimeStamp() int64
	SetValue(ctx CallContext, key []byte, value []byte)
	GetValue(ctx CallContext, key []byte) []byte
	RemoveValue(ctx CallContext, key []byte)
	Deploy(ctx CallContext)
	Send(ctx CallContext, dest common.Address, amount *big.Int) error
	MinFeePerGas() *big.Int
	Balance(address common.Address) *big.Int
	BlockSeed() []byte
	NetworkSize() int
	NetworkSizeFree() int
	State(sender common.Address) state.IdentityState
	PubKey(addr common.Address) []byte
	Iterate(ctx CallContext, minKey []byte, maxKey []byte, f func(key []byte, value []byte) bool)
	BurnAll(ctx CallContext)
	ReadContractData(contractAddr common.Address, key []byte) []byte
	Vrf(msg []byte) ([32]byte, []byte)
	Terminate(ctx CallContext, dest common.Address)
	Event(name string, args ...[]byte)
	Epoch() uint16
	ContractStake(common.Address) *big.Int
	MoveToStake(ctx CallContext, reward *big.Int) error
}

type contractValue struct {
	value   []byte
	removed bool
}

type EnvImp struct {
	state          *appstate.AppState
	block          *types.Header
	gasCounter     *GasCounter
	secStore       *secstore.SecStore
	statsCollector collector.StatsCollector

	contractStoreCache    map[common.Address]map[string]*contractValue
	balancesCache         map[common.Address]*big.Int
	deployedContractCache map[common.Address]*state.ContractData
	droppedContracts      map[common.Address]struct{}
	events                []*types.TxEvent
	contractStakeCache    map[common.Address]*big.Int
}

func NewEnvImp(s *appstate.AppState, block *types.Header, gasCounter *GasCounter, secStore *secstore.SecStore, statsCollector collector.StatsCollector) *EnvImp {
	return &EnvImp{state: s, block: block, gasCounter: gasCounter, secStore: secStore,
		contractStoreCache:    map[common.Address]map[string]*contractValue{},
		balancesCache:         map[common.Address]*big.Int{},
		deployedContractCache: map[common.Address]*state.ContractData{},
		droppedContracts:      map[common.Address]struct{}{},
		events:                []*types.TxEvent{},
		contractStakeCache:    map[common.Address]*big.Int{},
		statsCollector:        statsCollector,
	}
}

func (e *EnvImp) Epoch() uint16 {
	e.gasCounter.AddGas(10)
	return e.state.State.Epoch()
}

func (e *EnvImp) getBalance(address common.Address) *big.Int {
	if b, ok := e.balancesCache[address]; ok {
		return b
	}
	return e.state.State.GetBalance(address)
}

func (e *EnvImp) addBalance(address common.Address, amount *big.Int) {
	b := e.getBalance(address)
	e.setBalance(address, new(big.Int).Add(b, amount))
}

func (e *EnvImp) subBalance(address common.Address, amount *big.Int) {
	b := e.getBalance(address)
	e.setBalance(address, new(big.Int).Sub(b, amount))
}

func (e *EnvImp) setBalance(address common.Address, amount *big.Int) {
	collector.AddContractBalanceUpdate(e.statsCollector, address, e.getBalance, amount, e.state)
	e.balancesCache[address] = amount
}

func (e *EnvImp) Send(ctx CallContext, dest common.Address, amount *big.Int) error {
	balance := e.getBalance(ctx.ContractAddr())
	if balance.Cmp(amount) < 0 {
		return errors.New("insufficient funds")
	}
	if amount.Sign() < 0 {
		return errors.New("value must be non-negative")
	}
	e.subBalance(ctx.ContractAddr(), amount)
	e.addBalance(dest, amount)

	e.gasCounter.AddGas(30)
	return nil
}

func (e *EnvImp) Deploy(ctx CallContext) {
	contractAddr := ctx.ContractAddr()
	stake := ctx.PayAmount()
	e.deployedContractCache[contractAddr] = &state.ContractData{
		Stake:    stake,
		CodeHash: ctx.CodeHash(),
	}
	collector.AddContractStake(e.statsCollector, stake)
	e.gasCounter.AddGas(200)
}

func (e *EnvImp) BlockTimeStamp() int64 {
	e.gasCounter.AddGas(5)
	return e.block.Time()
}

func (e *EnvImp) BlockNumber() uint64 {
	e.gasCounter.AddGas(5)
	return e.block.Height()
}

func (e *EnvImp) SetValue(ctx CallContext, key []byte, value []byte) {
	if len(key) > maxEnvKeyLength {
		panic("key is too big")
	}
	addr := ctx.ContractAddr()
	var cache map[string]*contractValue
	var ok bool
	if cache, ok = e.contractStoreCache[addr]; !ok {
		cache = make(map[string]*contractValue)
		e.contractStoreCache[addr] = cache
	}
	cache[string(key)] = &contractValue{
		value:   value,
		removed: false,
	}
	e.gasCounter.AddWrittenBytesAsGas(10 * (len(key) + len(value)))
}

func (e *EnvImp) GetValue(ctx CallContext, key []byte) []byte {
	return e.ReadContractData(ctx.ContractAddr(), key)
}

func (e *EnvImp) RemoveValue(ctx CallContext, key []byte) {
	addr := ctx.ContractAddr()
	var cache map[string]*contractValue
	var ok bool
	if cache, ok = e.contractStoreCache[addr]; !ok {
		cache = map[string]*contractValue{}
		e.contractStoreCache[addr] = cache
	}
	cache[string(key)] = &contractValue{removed: true}
	e.gasCounter.AddGas(5)
}

func (e *EnvImp) MinFeePerGas() *big.Int {
	e.gasCounter.AddGas(5)
	return e.state.State.FeePerGas()
}

func (e *EnvImp) Balance(address common.Address) *big.Int {
	e.gasCounter.AddReadBytesAsGas(5)
	return e.getBalance(address)
}

func (e *EnvImp) BlockSeed() []byte {
	e.gasCounter.AddReadBytesAsGas(5)
	return e.block.Seed().Bytes()
}

func (e *EnvImp) NetworkSize() int {
	e.gasCounter.AddReadBytesAsGas(5)
	return e.NetworkSizeFree()
}

func (e *EnvImp) NetworkSizeFree() int {
	return e.state.ValidatorsCache.NetworkSize()
}

func (e *EnvImp) State(sender common.Address) state.IdentityState {
	e.gasCounter.AddReadBytesAsGas(1)
	return e.state.State.GetIdentityState(sender)
}

func (e *EnvImp) PubKey(addr common.Address) []byte {
	e.gasCounter.AddReadBytesAsGas(10)
	return e.state.State.GetIdentity(addr).PubKey
}

func (e *EnvImp) Iterate(ctx CallContext, minKey []byte, maxKey []byte, f func(key []byte, value []byte) (stopped bool)) {
	addr := ctx.ContractAddr()

	iteratedKeys := make(map[string]struct{})

	if cache, ok := e.contractStoreCache[addr]; ok {
		for key, value := range cache {
			keyBytes := []byte(key)
			if (bytes.Compare(keyBytes, minKey) >= 0 || minKey == nil) && (bytes.Compare(keyBytes, maxKey) <= 0 || maxKey == nil) {
				iteratedKeys[key] = struct{}{}
				e.gasCounter.AddReadBytesAsGas(10 * len(value.value))
				if !value.removed && f(keyBytes, value.value) {
					return
				}
			}
		}
	}

	e.state.State.IterateContractStore(addr, minKey, maxKey, func(key []byte, value []byte) bool {
		if _, ok := iteratedKeys[string(key)]; ok {
			return false
		}
		e.gasCounter.AddReadBytesAsGas(10 * len(value))
		return f(key, value)
	})
}

func (e *EnvImp) BurnAll(ctx CallContext) {
	e.gasCounter.AddReadBytesAsGas(10)
	address := ctx.ContractAddr()
	collector.AddContractBurntCoins(e.statsCollector, address, e.getBalance)
	e.setBalance(address, common.Big0)
}

func (e *EnvImp) ReadContractData(contractAddr common.Address, key []byte) []byte {
	if cache, ok := e.contractStoreCache[contractAddr]; ok {
		if value, ok := cache[string(key)]; ok {
			if value.removed {
				return nil
			}
			e.gasCounter.AddReadBytesAsGas(10 * len(value.value))
			return value.value
		}
	}
	value := e.state.State.GetContractValue(contractAddr, key)
	e.gasCounter.AddReadBytesAsGas(10 * len(value))
	return value
}

func (e *EnvImp) Vrf(msg []byte) (hash [32]byte, proof []byte) {
	e.gasCounter.AddGas(30)
	return e.secStore.VrfEvaluate(msg)
}

func (e *EnvImp) Terminate(ctx CallContext, dest common.Address) {
	stake := e.state.State.GetContractStake(ctx.ContractAddr())
	if stake == nil || stake.Sign() == 0 {
		return
	}
	e.addBalance(dest, stake)
	e.droppedContracts[ctx.ContractAddr()] = struct{}{}

	emptySlice := make([]byte, 32)
	minKey := emptySlice[:]
	var maxKey []byte
	for i := 0; i < 32; i++ {
		maxKey = append(maxKey, 0xFF)
	}

	e.Iterate(ctx, minKey, maxKey, func(key []byte, value []byte) (stopped bool) {
		e.RemoveValue(ctx, key)
		return false
	})
}

func (e *EnvImp) Commit() []*types.TxEvent {

	var contracts []common.Address
	for contract := range e.contractStoreCache {
		contracts = append(contracts, contract)
	}
	sort.SliceStable(contracts, func(i, j int) bool {
		return bytes.Compare(contracts[i].Bytes(), contracts[j].Bytes()) == 1
	})

	for _, contract := range contracts {
		cache := e.contractStoreCache[contract]

		var keys []string
		for k := range cache {
			keys = append(keys, k)
		}
		sort.SliceStable(keys, func(i, j int) bool {
			return keys[i] > keys[j]
		})
		for _, k := range keys {
			v := cache[k]
			if v.removed {
				e.state.State.RemoveContractValue(contract, []byte(k))
			} else {
				e.state.State.SetContractValue(contract, []byte(k), v.value)
			}
		}
	}
	for addr, b := range e.balancesCache {
		e.state.State.SetBalance(addr, b)
	}
	for contract, data := range e.deployedContractCache {
		e.state.State.DeployContract(contract, data.CodeHash, data.Stake)
	}
	for contract := range e.droppedContracts {
		e.state.State.DropContract(contract)
	}
	for contract, stake := range e.contractStakeCache {
		e.state.State.SetContractStake(contract, stake)
	}

	return e.events
}

func (e *EnvImp) Event(name string, args ...[]byte) {
	if !eventRegexp.MatchString(name) {
		panic("event name should contain only ASCII characters. Length should be 1-32")
	}
	size := 0
	for _, a := range args {
		size += len(a)
	}
	e.gasCounter.AddGas(100 + 10*size)
	e.events = append(e.events, &types.TxEvent{
		EventName: name, Data: args,
	})
}

func (e *EnvImp) contractStake(contract common.Address) *big.Int {
	if v, ok := e.deployedContractCache[contract]; ok {
		return v.Stake
	}
	if v, ok := e.contractStakeCache[contract]; ok {
		return v
	}
	return e.state.State.GetContractStake(contract)
}

func (e *EnvImp) ContractStake(contract common.Address) *big.Int {
	e.gasCounter.AddGas(10)
	return e.contractStake(contract)
}

func (e *EnvImp) MoveToStake(ctx CallContext, amount *big.Int) error {
	balance := e.getBalance(ctx.ContractAddr())
	if balance.Cmp(amount) < 0 {
		return errors.New("insufficient funds")
	}
	if amount.Sign() < 0 {
		return errors.New("value must be non-negative")
	}
	e.subBalance(ctx.ContractAddr(), amount)

	if v, ok := e.deployedContractCache[ctx.ContractAddr()]; ok {
		v.Stake = big.NewInt(0).Add(v.Stake, amount)
		return nil
	}
	stake := e.contractStake(ctx.ContractAddr())
	stake = big.NewInt(0).Add(stake, amount)
	e.contractStakeCache[ctx.ContractAddr()] = stake
	return nil
}

func (e *EnvImp) Reset() {
	e.contractStoreCache = map[common.Address]map[string]*contractValue{}
	e.balancesCache = map[common.Address]*big.Int{}
	e.deployedContractCache = map[common.Address]*state.ContractData{}
	e.droppedContracts = map[common.Address]struct{}{}
	e.contractStakeCache = map[common.Address]*big.Int{}
	e.events = []*types.TxEvent{}
}

type CallContext interface {
	Sender() common.Address
	ContractAddr() common.Address
	Epoch() uint16
	Nonce() uint32
	PayAmount() *big.Int
	CodeHash() common.Hash
}

type CallContextImpl struct {
	tx       *types.Transaction
	codeHash common.Hash
}

func NewCallContextImpl(tx *types.Transaction, codeHash common.Hash) *CallContextImpl {
	return &CallContextImpl{tx: tx, codeHash: codeHash}
}

func (c *CallContextImpl) PayAmount() *big.Int {
	return c.tx.AmountOrZero()
}

func (c *CallContextImpl) Epoch() uint16 {
	return c.tx.Epoch
}

func (c *CallContextImpl) Nonce() uint32 {
	return c.tx.AccountNonce
}

func (c *CallContextImpl) ContractAddr() common.Address {
	return *c.tx.To
}

func (c *CallContextImpl) Sender() common.Address {
	addr, _ := types.Sender(c.tx)
	return addr
}

func (c *CallContextImpl) CodeHash() common.Hash {
	return c.codeHash
}

type DeployContextImpl struct {
	tx       *types.Transaction
	codeHash common.Hash
}

func (d *DeployContextImpl) PayAmount() *big.Int {
	return d.tx.Amount
}

func NewDeployContextImpl(tx *types.Transaction, codeHash common.Hash) *DeployContextImpl {
	return &DeployContextImpl{tx: tx, codeHash: codeHash}
}

func (d *DeployContextImpl) CodeHash() common.Hash {
	return d.codeHash
}

func (d *DeployContextImpl) Epoch() uint16 {
	return d.tx.Epoch
}

func (d *DeployContextImpl) Nonce() uint32 {
	return d.tx.AccountNonce
}

func (d *DeployContextImpl) Sender() common.Address {
	addr, _ := types.Sender(d.tx)
	return addr
}

func (d *DeployContextImpl) ContractAddr() common.Address {
	hash := crypto.Hash(append(append(d.Sender().Bytes(), common.ToBytes(d.tx.Epoch)...), common.ToBytes(d.tx.AccountNonce)...))
	var result common.Address
	result.SetBytes(hash[:])
	return result
}

type ReadContextImpl struct {
	Contract common.Address
	Hash     common.Hash
}

func (r *ReadContextImpl) CodeHash() common.Hash {
	return r.Hash
}

func (r *ReadContextImpl) Sender() common.Address {
	panic("implement me")
}

func (r *ReadContextImpl) ContractAddr() common.Address {
	return r.Contract
}

func (r *ReadContextImpl) Epoch() uint16 {
	panic("implement me")
}

func (r *ReadContextImpl) Nonce() uint32 {
	panic("implement me")
}

func (r *ReadContextImpl) PayAmount() *big.Int {
	panic("implement me")
}
