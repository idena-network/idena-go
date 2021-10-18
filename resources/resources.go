package resources

import (
	"embed"
	"io"
)


//go:embed header.tar
//go:embed identitystatedb.tar
//go:embed statedb.tar
var content embed.FS


func IntermediateGenesisHeader() ([]byte,error) {
	return content.ReadFile("header.tar")
}

func IdentityStateDb() (io.ReadCloser, error) {
	return content.Open("identitystatedb.tar")
}

func StateDb() (io.ReadCloser, error) {
	return content.Open("statedb.tar")
}

func PredefinedState() ([]byte,error) {
	return content.ReadFile("stategen.out")
}






