package wasm

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/appstate"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/vm/costs"
	"github.com/idena-network/idena-wasm-binding/lib"
	"github.com/pkg/errors"
	"math/big"
	"regexp"
)

var (
	eventRegexp *regexp.Regexp
)

func init() {
	// any ASCII character
	eventRegexp, _ = regexp.Compile("^[\x00-\x7F]{1,32}$")
}

type contractValue struct {
	value   []byte
	removed bool
}

type ContractData struct {
	Code []byte
}

type BlockHeaderProvider interface {
	GetBlockHeaderByHeight(height uint64) *types.Header
}

type WasmEnv struct {
	id             int
	appState       *appstate.AppState
	ctx            *ContractContext
	head           *types.Header
	parent         *WasmEnv
	headerProvider BlockHeaderProvider

	contractStoreCache    map[common.Address]map[string]*contractValue
	balancesCache         map[common.Address]*big.Int
	deployedContractCache map[common.Address]ContractData
	events                []*types.TxEvent
	method                string
	isDebug               bool
}

func (w *WasmEnv) Burn(meter *lib.GasMeter, amount *big.Int) error {
	return w.SubBalance(meter, amount)
}

func (w *WasmEnv) Ecrecover(meter *lib.GasMeter, data []byte, signature []byte) []byte {
	meter.ConsumeGas(costs.GasToWasmGas(costs.EcrecoverGas))
	pubkey, _ := crypto.Ecrecover(data, signature)
	return pubkey
}

func (w *WasmEnv) BlockHeader(meter *lib.GasMeter, height uint64) []byte {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadBlockGas))
	if header := w.headerProvider.GetBlockHeaderByHeight(height); header != nil {
		r, _ := header.ToBytes()
		return r
	}
	return []byte{}
}

func (w *WasmEnv) Keccak256(meter *lib.GasMeter, data []byte) []byte {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ComputeHashGas))
	hash := crypto.Hash(data)
	return hash[:]
}

func (w *WasmEnv) IsDebug() bool {
	return w.isDebug
}

func (w *WasmEnv) PayAmount(meter *lib.GasMeter) *big.Int {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadBlockGas))
	return w.ctx.payAmount
}

func (w *WasmEnv) ContractCodeHash(addr lib.Address) *[]byte {
	if data, ok := w.deployedContractCache[addr]; ok {
		hash := common.Hash(crypto.Hash(data.Code)).Bytes()
		return &hash
	}

	if w.parent != nil {
		return w.parent.ContractCodeHash(addr)
	}
	codeHash := w.appState.State.GetCodeHash(addr)
	if codeHash != nil {
		hash := codeHash.Bytes()
		return &hash
	}

	return nil
}

func (w *WasmEnv) Epoch(meter *lib.GasMeter) uint16 {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadGlobalStateGas))
	return w.appState.State.Epoch()
}

func (w *WasmEnv) ReadContractData(meter *lib.GasMeter, address lib.Address, key []byte) []byte {
	value := w.readContractData(address, key)
	meter.ConsumeGas(costs.GasToWasmGas(uint64(costs.ReadStatePerByteGas * len(value))))
	return value
}

func (w *WasmEnv) Event(meter *lib.GasMeter, name string, args ...[]byte) {
	if !eventRegexp.MatchString(name) {
		panic("event name should contain only ASCII characters. Length should be 1-32")
	}
	size := 0
	for _, a := range args {
		size += len(a)
	}
	meter.ConsumeGas(costs.GasToWasmGas(uint64(costs.EmitEventBase + costs.EmitEventPerByteGas*size)))
	w.events = append(w.events, &types.TxEvent{
		Contract:  w.ctx.ContractAddr(),
		EventName: name, Data: args,
	})
}

func (w *WasmEnv) CodeHash(meter *lib.GasMeter) []byte {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ComputeHashGas))
	hash := crypto.Hash(w.OwnCode(meter))
	return hash[:]
}

func (w *WasmEnv) ContractAddrByHash(meter *lib.GasMeter, hash []byte, args []byte, nonce []byte) lib.Address {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ComputeHashGas))
	return ComputeContractAddrByHash(hash, args, nonce)
}

func (w *WasmEnv) OwnCode(meter *lib.GasMeter) []byte {
	addr := w.ContractAddress(meter)
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadStateGas))
	return w.GetCode(addr)
}

func (w *WasmEnv) ContractAddr(meter *lib.GasMeter, code []byte, args []byte, nonce []byte) lib.Address {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ComputeHashGas * 2))
	return ComputeContractAddr(code, args, nonce)
}

func (w *WasmEnv) MinFeePerGas(meter *lib.GasMeter) *big.Int {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadGlobalStateGas))
	return w.appState.State.FeePerGas()
}

func (w *WasmEnv) BlockSeed(meter *lib.GasMeter) []byte {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadBlockGas))
	return w.head.Seed().Bytes()
}

func (w *WasmEnv) SubBalance(meter *lib.GasMeter, amount *big.Int) error {
	meter.ConsumeGas(costs.GasToWasmGas(costs.MoveBalanceGas))
	balance := w.getBalance(w.ctx.ContractAddr())
	if amount.Sign() < 0 {
		return errors.New("value must be non-negative")
	}
	if balance.Cmp(amount) < 0 {
		return errors.New("insufficient funds")
	}
	w.subBalance(w.ctx.ContractAddr(), amount)
	return nil
}

func (w *WasmEnv) AddBalance(meter *lib.GasMeter, address lib.Address, amount *big.Int) {
	meter.ConsumeGas(costs.GasToWasmGas(costs.MoveBalanceGas))
	w.addBalance(address, amount)
}

func (w *WasmEnv) ContractAddress(meter *lib.GasMeter) lib.Address {
	if meter != nil {
		meter.ConsumeGas(costs.GasToWasmGas(costs.ReadBlockGas))
	}
	return w.ctx.ContractAddr()
}

func NewWasmEnv(appState *appstate.AppState, blockHeaderProvider BlockHeaderProvider, ctx *ContractContext, head *types.Header, method string, isDebug bool) *WasmEnv {
	return &WasmEnv{
		id:                    1,
		headerProvider:        blockHeaderProvider,
		appState:              appState,
		ctx:                   ctx,
		head:                  head,
		contractStoreCache:    map[common.Address]map[string]*contractValue{},
		balancesCache:         map[common.Address]*big.Int{},
		deployedContractCache: map[common.Address]ContractData{},
		method:                method,
		isDebug:               isDebug,
	}
}

func (w *WasmEnv) readContractData(contractAddr common.Address, key []byte) []byte {
	if cache, ok := w.contractStoreCache[contractAddr]; ok {
		if value, ok := cache[string(key)]; ok {
			if value.removed {
				return nil
			}
			return value.value
		}
	}

	if w.parent != nil {
		return w.parent.readContractData(contractAddr, key)
	}

	value := w.appState.State.GetContractValue(contractAddr, key)

	return value
}

func (w *WasmEnv) SetStorage(meter *lib.GasMeter, key []byte, value []byte) {
	ctx := w.ctx
	if len(key) > common.MaxWasmContractStoreKeyLength {
		panic("key is too big")
	}
	addr := ctx.ContractAddr()
	var cache map[string]*contractValue
	var ok bool
	if cache, ok = w.contractStoreCache[addr]; !ok {
		cache = make(map[string]*contractValue)
		w.contractStoreCache[addr] = cache
	}
	cache[string(key)] = &contractValue{
		value:   value,
		removed: false,
	}
	meter.ConsumeGas(costs.GasToWasmGas(uint64(costs.WriteStatePerByteGas * (len(key) + len(value)))))
}

func (w *WasmEnv) GetStorage(meter *lib.GasMeter, key []byte) []byte {
	value := w.readContractData(w.ctx.ContractAddr(), key)
	meter.ConsumeGas(costs.GasToWasmGas(uint64(costs.ReadStatePerByteGas * len(value))))
	return value
}

func (w *WasmEnv) RemoveStorage(meter *lib.GasMeter, key []byte) {
	addr := w.ctx.ContractAddr()
	var cache map[string]*contractValue
	var ok bool
	if cache, ok = w.contractStoreCache[addr]; !ok {
		cache = map[string]*contractValue{}
		w.contractStoreCache[addr] = cache
	}
	cache[string(key)] = &contractValue{removed: true}
	meter.ConsumeGas(costs.GasToWasmGas(costs.RemoveStateGas))
}

func (w *WasmEnv) BlockNumber(meter *lib.GasMeter) uint64 {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadBlockGas))
	return w.head.Height()
}

func (w *WasmEnv) BlockTimestamp(meter *lib.GasMeter) int64 {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadBlockGas))
	return w.head.Time()
}

func (w *WasmEnv) Balance(meter *lib.GasMeter) *big.Int {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadBalanceGas))
	return w.getBalance(w.ctx.ContractAddr())
}

func (w *WasmEnv) NetworkSize(meter *lib.GasMeter) uint64 {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadBlockGas))
	return uint64(w.appState.ValidatorsCache.NetworkSize())
}

func (w *WasmEnv) Identity(meter *lib.GasMeter, address lib.Address) []byte {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadStateGas))
	return w.appState.State.RawIdentity(address)
}

func (w *WasmEnv) CreateSubEnv(contract lib.Address, method string, payAmount *big.Int, isDeploy bool) (lib.HostEnv, error) {
	const maxDepth = 16
	if w.id > maxDepth {
		return nil, errors.New("max recursion depth reached")
	}

	if payAmount.Sign() < 0 {
		return nil, errors.New("value must be non-negative")
	}

	subContractBalance := w.getBalance(contract)
	subContractBalance = new(big.Int).Add(subContractBalance, payAmount)

	subEnv := &WasmEnv{
		id:                    w.id + 1,
		method:                method,
		appState:              w.appState,
		parent:                w,
		ctx:                   w.ctx.CreateSubContext(contract, payAmount),
		contractStoreCache:    map[common.Address]map[string]*contractValue{},
		balancesCache:         map[common.Address]*big.Int{contract: subContractBalance},
		deployedContractCache: map[common.Address]ContractData{},
		head:                  w.head,
		isDebug:               w.isDebug,
	}
	if w.isDebug {
		log.Info("created sub env", "id", subEnv.id, "method", method, "parent method", subEnv.parent.method)
	}
	return subEnv, nil
}

func (w *WasmEnv) GetCode(addr lib.Address) []byte {
	if data, ok := w.deployedContractCache[addr]; ok {
		return data.Code
	}

	if w.parent != nil {
		return w.parent.GetCode(addr)
	}

	return w.appState.State.GetContractCode(addr)
}

func (w *WasmEnv) getBalance(address common.Address) *big.Int {
	if b, ok := w.balancesCache[address]; ok {
		return b
	}
	if w.parent != nil {
		return w.parent.getBalance(address)
	}
	return w.appState.State.GetBalance(address)
}

func (w *WasmEnv) addBalance(address common.Address, amount *big.Int) {
	b := w.getBalance(address)
	w.setBalance(address, new(big.Int).Add(b, amount))
}

func (w *WasmEnv) subBalance(address common.Address, amount *big.Int) {
	b := w.getBalance(address)
	w.setBalance(address, new(big.Int).Sub(b, amount))
}

func (w *WasmEnv) setBalance(address common.Address, amount *big.Int) {
	w.balancesCache[address] = amount
}

func (w *WasmEnv) Deploy(code []byte) {
	contractAddr := w.ctx.ContractAddr()
	w.deployedContractCache[contractAddr] = ContractData{
		Code: code,
	}
}

func (w *WasmEnv) Caller(meter *lib.GasMeter) lib.Address {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadBlockGas))
	return w.ctx.Caller()
}

func (w *WasmEnv) OriginalCaller(meter *lib.GasMeter) lib.Address {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadBlockGas))
	return w.ctx.originCaller
}

func (w *WasmEnv) GlobalState(meter *lib.GasMeter) []byte {
	meter.ConsumeGas(costs.GasToWasmGas(costs.ReadGlobalStateGas))
	return w.appState.State.RawGlobal()
}

func (w *WasmEnv) Commit() {

	if w.isDebug {
		log.Info("commit to wasm env", "id", w.id, "method", w.method)
	}
	if w.parent != nil {
		for contract, cache := range w.contractStoreCache {
			if w.parent.contractStoreCache[contract] == nil {
				w.parent.contractStoreCache[contract] = map[string]*contractValue{}
			}
			for k, v := range cache {
				w.parent.contractStoreCache[contract][k] = v
			}
		}
		for addr, b := range w.balancesCache {
			w.parent.balancesCache[addr] = b
		}
		for contract, data := range w.deployedContractCache {
			w.parent.deployedContractCache[contract] = data
		}
		for _, e := range w.events {
			w.parent.events = append(w.parent.events, e)
		}
		return
	}

	for contract, cache := range w.contractStoreCache {
		for k, v := range cache {
			if v.removed {
				w.appState.State.RemoveContractValue(contract, []byte(k))
			} else {
				w.appState.State.SetContractValue(contract, []byte(k), v.value)
			}
		}
	}
	for addr, b := range w.balancesCache {
		w.appState.State.SetBalance(addr, b)
	}
	for contract, data := range w.deployedContractCache {
		w.appState.State.DeployWasmContract(contract, data.Code)
	}
}

func (w *WasmEnv) InternalCommit() []*types.TxEvent {
	w.Commit()
	return w.events
}
