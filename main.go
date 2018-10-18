package main

import (
	"bufio"
	"fmt"
	"idena-go/crypto"
	"idena-go/log"
	"idena-go/p2p"
	"idena-go/p2p/enode"
	"idena-go/p2p/nat"
	"idena-go/test-chat-protocol"
	"os"
	"strings"
)

func main() {

	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	logger := log.Root().New()
	chat1, _ := test_chat_protocol.NewChat()
	//chat2, _ := test_chat_protocol.NewChat()
	sk, _ := crypto.GenerateKey()
	srv := &p2p.Server{
		Config: p2p.Config{
			PrivateKey: sk,
			ListenAddr: ":30303",
			MaxPeers:   25,
			NAT:        nat.Any(),
			Protocols:  []p2p.Protocol{chat1.Protocol},
		},
	}
	//sk2, _ := crypto.GenerateKey()
	//srv2 := &p2p.Server{
	//	Config: p2p.Config{
	//		PrivateKey: sk2,
	//		ListenAddr: ":30304",
	//		MaxPeers:   25,
	//		NAT:        nat.Any(),
	//		Protocols:  []p2p.Protocol{chat2.Protocol},
	//	},
	//}

	//data := []byte{0x1, 0x2, 0x3, 0x1, 0x2, 0x3, 0x1, 0x2, 0x3, 0x1, 0x2, 0x3, 0x1, 0x2, 0x3, 0x1, 0x2, 0x3, 0x1, 0x2, 0x3, 0x1, 0x2, 0x3, 0x1, 0x2, 0x3, 0x1, 0x2, 0x3, 0x1, 0x2}
	//
	//k, _ := p256.NewVRFSigner(sk)
	//
	//pk, _ := p256.NewVRFVerifier(sk.Public().(*ecdsa.PublicKey))
	//
	//h1, proof := k.Evaluate(data)
	//
	//h2, _ := pk.ProofToHash(data, proof)

	srv.Start()
	//srv2.Start()
	//
	//srv2.AddPeer(srv.Self())
	//srv2.AddTrustedPeer(srv.Self())
	reader := bufio.NewReader(os.Stdin)
	for {

		switch text, _ := reader.ReadString('\n'); strings.Trim(text, "\n\r") {
		case "me":
			fmt.Println(srv.NodeInfo().Enode)
		case "exit":
			return;
		case "add":
			peer, _ :=reader.ReadString('\n');
			node, err := enode.ParseV4(strings.Trim(peer, "\n\r "))
			if err!= nil{
				logger.Error("Can't parse enode url")
				continue
			}
			srv.AddPeer(node)
			srv.AddTrustedPeer(node)
		case "peers":
			for _, p := range srv.Peers() {
				fmt.Println(p.Info())
			}
		default:
			for _, p := range srv.Peers() {
				p2p.Send(p.Connection(), 18, text)
			}
		}
	}
	//fmt.Println(p)
}
