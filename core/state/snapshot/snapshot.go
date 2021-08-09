package snapshot

import (
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/common"
	models "github.com/idena-network/idena-go/protobuf"
)

type Manifest struct {
	Root   common.Hash
	Height uint64
	Cid    []byte
	CidV2  []byte
}

func (m *Manifest) ToBytes() ([]byte, error) {
	protoObj := &models.ProtoManifest{
		Cid:    m.Cid,
		Height: m.Height,
		Root:   m.Root[:],
		CidV2:  m.CidV2,
	}
	return proto.Marshal(protoObj)
}

func (m *Manifest) FromBytes(data []byte) error {
	protoObj := new(models.ProtoManifest)
	if err := proto.Unmarshal(data, protoObj); err != nil {
		return err
	}
	m.Root = common.BytesToHash(protoObj.Root)
	m.Height = protoObj.Height
	m.Cid = protoObj.Cid
	m.CidV2 = protoObj.CidV2
	return nil
}
