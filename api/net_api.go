package api

import (
	"idena-go/p2p"
	"idena-go/p2p/enode"
	"idena-go/protocol"
)

// NetApi offers helper utils
type NetApi struct {
	pm  *protocol.ProtocolManager
	srv *p2p.Server
}

// NewNetApi creates a new NetApi instance
func NewNetApi(pm *protocol.ProtocolManager, srv *p2p.Server) *NetApi {
	return &NetApi{pm, srv}
}

func (api *NetApi) PeersCount() int {
	return api.pm.PeersCount()
}

type Peer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	RemoteAddr string `json:"addr"`
}

func (api *NetApi) Peers() []Peer {
	peers := make([]Peer, 0)

	for _, peer := range api.pm.Peers() {
		item := Peer{
			ID:         peer.ID().String(),
			Name:       peer.Name(),
			RemoteAddr: peer.RemoteAddr().String(),
		}

		peers = append(peers, item)
	}

	return peers
}

func (api *NetApi) AddPeer(url string) error {
	if n, err := enode.ParseV4(url); err != nil {
		return err
	} else {
		api.srv.AddPeer(n)
		return nil
	}
}
