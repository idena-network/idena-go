package protocol

import (
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/multiformats/go-multiaddr"
)

type notifiee struct {
	connManager *ConnManager
}

func (n *notifiee) Listen(network.Network, multiaddr.Multiaddr) {

}

func (n *notifiee) ListenClose(network.Network, multiaddr.Multiaddr) {

}

func (n *notifiee) Connected(net network.Network, conn network.Conn) {
	n.connManager.AddConnection(conn)
}

func (n *notifiee) Disconnected(net network.Network, conn network.Conn) {
	n.connManager.RemoveConnection(conn)
}

func (n *notifiee) OpenedStream(net network.Network, conn network.Stream) {
}

func (n *notifiee) ClosedStream(net network.Network, stream network.Stream) {
}

