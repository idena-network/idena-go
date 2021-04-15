package api

import (
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/ipfs"
	cid2 "github.com/ipfs/go-cid"
)

type IpfsApi struct {
	ipfsProxy ipfs.Proxy
}

// NewNetApi creates a new NetApi instance
func NewIpfsApi(ipfsProxy ipfs.Proxy) *IpfsApi {
	return &IpfsApi{ipfsProxy}
}

func (api *IpfsApi) Cid(data hexutil.Bytes) (string, error) {
	cid, err := api.ipfsProxy.Cid(data)
	if err != nil {
		return "", err
	}
	return cid.String(), nil
}

func (api *IpfsApi) Add(data hexutil.Bytes, pin bool) (string, error) {
	cid, err := api.ipfsProxy.Add(data, pin)
	if err != nil {
		return "", err
	}
	return cid.String(), nil
}

func (api *IpfsApi) Get(cid string) (hexutil.Bytes, error) {
	c, err := cid2.Decode(cid)
	if err != nil {
		return nil, err
	}
	return api.ipfsProxy.Get(c.Bytes(), ipfs.CustomData)
}
