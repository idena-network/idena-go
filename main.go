package main

import (
	"fmt"
	"github.com/google/keytransparency/core/crypto/vrf/p256"
	"idena-go/p2p"
)


func main() {
	var p p2p.Server
	p256.GenerateKey()
	go p.Start()
	fmt.Scanln()
	fmt.Println(p)
}
