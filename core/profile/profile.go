package profile

import (
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/ipfs"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/idena-network/idena-go/rlp"
	"github.com/pkg/errors"
)

type Manager struct {
	ipfsProxy ipfs.Proxy
}

type Profile struct {
	Nickname []byte `rlp:"nil"`
	Info     []byte `rlp:"nil"`
}

func (p *Profile) ToBytes() ([]byte, error) {
	protoProfile := new(models.ProtoProfile)
	protoProfile.Info = p.Info
	protoProfile.Nickname = p.Nickname
	return proto.Marshal(protoProfile)
}

func (p *Profile) FromBytes(data []byte) error {
	protoProfile := new(models.ProtoProfile)
	if err := proto.Unmarshal(data, protoProfile); err != nil {
		return err
	}
	p.Nickname = protoProfile.Nickname
	p.Info = protoProfile.Info
	return nil
}

func NewProfileManager(ipfsProxy ipfs.Proxy) *Manager {
	return &Manager{
		ipfsProxy: ipfsProxy,
	}
}

func (pm *Manager) AddProfile(pr Profile) ([]byte, error) {
	encodedData, _ := pr.ToBytes()
	if len(encodedData) > common.MaxProfileSize {
		return nil, errors.Errorf("profile data is too big, max expected size %v, actual %v",
			common.MaxProfileSize, len(encodedData))
	}
	hash, err := pm.ipfsProxy.Add(encodedData, true)
	if err != nil {
		return nil, err
	}
	return hash.Bytes(), nil
}

func (pm *Manager) GetProfile(hash []byte) (Profile, error) {
	encodedData, err := pm.ipfsProxy.Get(hash, ipfs.Profile)
	if err != nil {
		return Profile{}, err
	}
	res := &Profile{}

	// TODO: remove RLP support in future releases
	if err := res.FromBytes(encodedData); err != nil {
		if err := rlp.DecodeBytes(encodedData, res); err != nil {
			return Profile{}, err
		}
	}

	return *res, nil
}
