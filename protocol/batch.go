package protocol

import (
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/core/state"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/libp2p/go-libp2p-core/peer"
)

type batch struct {
	p       *protoPeer
	from    uint64
	to      uint64
	headers chan *block
}

type block struct {
	Header       *types.Header
	Cert         *types.BlockCert         `rlp:"nil"`
	IdentityDiff *state.IdentityStateDiff `rlp:"nil"`
}

type blockPeer struct {
	block
	peerId peer.ID
}

type blockRange struct {
	BatchId uint32
	Blocks  []*block
}

func (r *blockRange) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoGossipBlockRange{
		BatchId: r.BatchId,
	}
	for _, item := range r.Blocks {
		b := new(models.ProtoGossipBlockRange_Block)
		if item.Header != nil {
			b.Header = item.Header.ToProto()
		}
		if item.Cert != nil {
			b.Cert = item.Cert.ToProto()
		}
		if item.IdentityDiff != nil {
			b.Diff = item.IdentityDiff.ToProto()
		}
		protoObj.Blocks = append(protoObj.Blocks, b)
	}
	return proto.Marshal(protoObj)
}

func (r *blockRange) FromBytes(data []byte) error {
	protoObj := new(models.ProtoGossipBlockRange)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}

	r.BatchId = protoObj.BatchId
	for _, item := range protoObj.Blocks {
		b := new(block)
		if item.Header != nil {
			b.Header = new(types.Header).FromProto(item.Header)
		}
		if item.Cert != nil {
			b.Cert = new(types.BlockCert).FromProto(item.Cert)
		}
		if item.Diff != nil {
			b.IdentityDiff = new(state.IdentityStateDiff).FromProto(item.Diff)
		}
		r.Blocks = append(r.Blocks, b)
	}
	return nil
}
