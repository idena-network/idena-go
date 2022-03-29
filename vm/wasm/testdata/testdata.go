package testdata

import "embed"

//go:embed optimized.wasm
//go:embed erc20.wasm
var content embed.FS


func Testdata1() ([]byte,error) {
	return content.ReadFile("optimized.wasm")
}

func Erc20() ([]byte,error) {
	return content.ReadFile("erc20.wasm")
}