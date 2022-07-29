package costs

const ComputeHashGas = 100

const ReadGlobalStateGas = 10
const ReadStateGas = 10
const ReadBlockGas = 5

const ReadStatePerByteGas = 10
const WriteStatePerByteGas = 20
const RemoveStateGas = 5
const ReadIdentityStateGas = 1

const MoveBalanceGas = 30
const DeployContractGas = 200
const BurnAllGas = 10

const EmitEventPerByteGas = 10
const EmitEventBase = 100

const WasmGasMultiplier = 100

func GasToWasmGas(gas uint64) uint64 {
	return gas * WasmGasMultiplier
}

func WasmGasToGas(gas uint64) uint64 {
	return gas / WasmGasMultiplier
}
