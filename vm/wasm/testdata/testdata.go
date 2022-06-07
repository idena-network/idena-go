package testdata

import "embed"

//go:embed optimized.wasm
//go:embed erc20.wasm
//go:embed inc_func.wasm
//go:embed sum_func.wasm
var content embed.FS

func Testdata1() ([]byte, error) {
	return content.ReadFile("optimized.wasm")
}

func Erc20() ([]byte, error) {
	return content.ReadFile("erc20.wasm")
}

func IncFunc() ([]byte, error) {
	return content.ReadFile("inc_func.wasm")
}

func SumFunc() ([]byte, error) {
	return content.ReadFile("sum_func.wasm")
}
