package protocol

import (
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	models "github.com/idena-network/idena-go/protobuf"
	"time"
)

const (
	DecodeErr                  = 1
	ValidationErr              = 2
	MaxTimestampLagSeconds     = 15
	MaxBannedPeers             = 500000
	IdenaProtocolWeight        = 25
	ReconnectAfterDiscTimeout  = time.Minute * 1
	ReconnectAfterResetTimeout = time.Minute * 3
)

type request struct {
	msgcode uint64
	data    interface{}
}

type Msg struct {
	Code    uint64
	Payload []byte
}

func (msg *Msg) ToBytes() ([]byte, error) {
	protoMsg := &models.ProtoMsg{
		Code:    msg.Code,
		Payload: msg.Payload,
	}
	return proto.Marshal(protoMsg)
}

func (msg *Msg) FromBytes(data []byte) error {
	protoMsg := new(models.ProtoMsg)
	if err := proto.Unmarshal(data, protoMsg); err != nil {
		return err
	}
	msg.Code = protoMsg.Code
	msg.Payload = protoMsg.Payload
	return nil
}

type handshakeData struct {
	NetworkId    types.Network
	Height       uint64
	GenesisBlock common.Hash
	Timestamp    int64
	AppVersion   string
	Peers        uint32
}

func (h *handshakeData) ToBytes() ([]byte, error) {
	protoHandshake := &models.ProtoHandshake{
		NetworkId:  h.NetworkId,
		Height:     h.Height,
		Genesis:    h.GenesisBlock[:],
		Timestamp:  h.Timestamp,
		AppVersion: h.AppVersion,
		Peers:      h.Peers,
	}
	return proto.Marshal(protoHandshake)
}

func (h *handshakeData) FromBytes(data []byte) error {
	protoHandshake := new(models.ProtoHandshake)
	if err := proto.Unmarshal(data, protoHandshake); err != nil {
		return err
	}
	h.NetworkId = protoHandshake.NetworkId
	h.Height = protoHandshake.Height
	h.GenesisBlock = common.BytesToHash(protoHandshake.Genesis)
	h.Timestamp = protoHandshake.Timestamp
	h.AppVersion = protoHandshake.AppVersion
	h.Peers = protoHandshake.Peers
	return nil
}

type pushType uint8

const (
	pushVote       pushType = 1
	pushBlock      pushType = 2
	pushProof      pushType = 3
	pushFlip       pushType = 4
	pushKeyPackage pushType = 5
	pushTx         pushType = 6
)

type pushPullHash struct {
	Type pushType
	Hash common.Hash128
}

func (h *pushPullHash) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoPullPushHash{
		Type: uint32(h.Type),
		Hash: h.Hash[:],
	}
	return proto.Marshal(protoObj)
}

func (h *pushPullHash) FromBytes(data []byte) error {
	protoObj := new(models.ProtoPullPushHash)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	h.Hash = common.BytesToHash128(protoObj.Hash)
	h.Type = pushType(protoObj.Type)
	return nil
}

func (h *pushPullHash) String() string {
	return string(h.Type) + string(h.Hash.Bytes())
}

func (h *pushPullHash) IsValid() bool {
	return h.Type >= pushVote && h.Type <= pushTx
}
