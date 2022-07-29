package wasm

import (
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-wasm-binding/lib"
)

func ComputeContractAddrWithUnpackedArgs(code []byte, args [][]byte, nonce []byte) common.Address {
	return ComputeContractAddr(code, lib.PackArguments(args), nonce)
}

func ComputeContractAddr(code []byte, args []byte, nonce []byte) common.Address {
	codeHash := crypto.Hash(code)
	println("packed args", common.ToHex(args))
	return ComputeContractAddrByHash(codeHash[:], args, nonce)
}

func ComputeContractAddrByHash(codeHash []byte, args []byte, nonce []byte) common.Address {
	hash := crypto.Hash(append(append(codeHash[:], args...), nonce...))
	var result common.Address
	result.SetBytes(hash[:])
	return result
}
