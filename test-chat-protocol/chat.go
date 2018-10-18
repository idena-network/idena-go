package test_chat_protocol

import (
	"bytes"
	"fmt"
	"idena-go/p2p"
)

type Chat struct {
	Protocol p2p.Protocol
}

func NewChat() (*Chat, error) {
	chat := Chat{
		Protocol: p2p.Protocol{
			Name:    "AppProtocol",
			Version: 1,
			Length:  20,
			Run: func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
				for {
					msg, err := rw.ReadMsg()
					if err != nil {
						return err
					}
					buf := new(bytes.Buffer)
					buf.ReadFrom(msg.Payload)
					fmt.Println(string(buf.Bytes()[1:]))
				}
			},
		},
	}
	return &chat, nil
}
