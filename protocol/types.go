package protocol

import (
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/rlp"
	"github.com/pkg/errors"
	"time"
)

const (
	DecodeErr                  = 1
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

func (msg *Msg) Decode(val interface{}) error {
	if err := rlp.DecodeBytes(msg.Payload, val); err != nil {
		return errors.Errorf("invalid message (code %x) %v", msg.Code, err)
	}
	return nil
}

type handshakeData struct {
	NetworkId    types.Network
	Height       uint64
	GenesisBlock common.Hash
	Timestamp    uint64
	AppVersion   string
	Peers        uint32
}

type getBlockBodyRequest struct {
	Hash common.Hash
}

type getBlocksRangeRequest struct {
	BatchId uint32
	From    uint64
	To      uint64
}

type getForkBlocksRangeRequest struct {
	BatchId uint32
	Blocks  []common.Hash
}

type proposeProof struct {
	Hash   common.Hash
	Proof  []byte
	PubKey []byte
	Round  uint64
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

func (h *pushPullHash) String() string {
	return string(h.Type) + string(h.Hash.Bytes())
}

func (h *pushPullHash) Invalid() bool {
	return h.Type < pushVote || h.Type > pushTx
}
