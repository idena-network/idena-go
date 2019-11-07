package profile

import (
	"github.com/idena-network/idena-go/ipfs"
	"github.com/idena-network/idena-go/rlp"
	"github.com/pkg/errors"
)

const (
	maxIpfsDataSize = 1024 * 600
)

type Manager struct {
	ipfsProxy ipfs.Proxy
}

type Profile struct {
	Nickname []byte `rlp:"nil"`
	Info     []byte `rlp:"nil"`
}

func NewProfileManager(ipfsProxy ipfs.Proxy) *Manager {
	return &Manager{
		ipfsProxy: ipfsProxy,
	}
}

func (pm *Manager) AddProfile(pr Profile) ([]byte, error) {
	encodedData, _ := rlp.EncodeToBytes(pr)
	if len(encodedData) > maxIpfsDataSize {
		return nil, errors.Errorf("profile data is too big, max expected size %v, actual %v",
			maxIpfsDataSize, len(encodedData))
	}
	hash, err := pm.ipfsProxy.Add(encodedData)
	if err != nil {
		return nil, err
	}
	return hash.Bytes(), nil
}

func (pm *Manager) GetProfile(hash []byte) (Profile, error) {
	encodedData, err := pm.ipfsProxy.Get(hash)
	if err != nil {
		return Profile{}, err
	}
	res := Profile{}
	if err := rlp.DecodeBytes(encodedData, &res); err != nil {
		return Profile{}, err
	}
	return res, nil
}
