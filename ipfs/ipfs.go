package ipfs

import (
	"github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-cid"
)

type IpfsProxy interface {
	Add(data []byte) (cid.Cid, error)
	Get(key []byte) ([]byte, error)
	Pin(key []byte) error
	Cid(data []byte) (cid.Cid, error)
}

type ipfsProxy struct {
}

func NewIpfsProxy() IpfsProxy {
	return &ipfsProxy{

	}
}

func (ipfsProxy) Add(data []byte) (cid.Cid, error) {
	panic("implement me")
}

func (ipfsProxy) Get(key []byte) ([]byte, error) {
	panic("implement me")
}

func (ipfsProxy) Pin(key []byte) error {
	panic("implement me")
}

func (ipfsProxy) Cid(data []byte) (cid.Cid, error) {
	format := cid.V0Builder{}
	return format.Sum(data)
}
