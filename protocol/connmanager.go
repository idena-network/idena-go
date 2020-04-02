package protocol

import (
	"context"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/config"
	core "github.com/libp2p/go-libp2p-core"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-yamux"
	"github.com/pkg/errors"
	"math/rand"
	"sync"
	"time"
)

const (
	dialPeerAttempts = 10
)

var FailedToDialPeer = errors.New("failed to select peer")
var NoPeersToDial = errors.New("no peers to dial")

type ConnManager struct {
	bannedPeers       mapset.Set
	activeConnections map[peer.ID]network.Conn
	discTimes         map[peer.ID]time.Time
	resetTimes        map[peer.ID]time.Time

	inboundPeers  map[peer.ID]struct{}
	outboundPeers map[peer.ID]struct{}

	peerMutex sync.RWMutex
	connMutex sync.Mutex
	host      core.Host
	cfg       config.P2P
}

func NewConnManager(host core.Host, cfg config.P2P) *ConnManager {
	return &ConnManager{
		host:              host,
		cfg:               cfg,
		bannedPeers:       mapset.NewSet(),
		activeConnections: make(map[peer.ID]network.Conn),
		inboundPeers:      make(map[peer.ID]struct{}),
		outboundPeers:     make(map[peer.ID]struct{}),

		discTimes:  make(map[peer.ID]time.Time),
		resetTimes: make(map[peer.ID]time.Time),
	}
}

func (m *ConnManager) CanConnect(id peer.ID) bool {
	if m.bannedPeers.Contains(id) {
		return false
	}
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()
	if discTime, ok := m.discTimes[id]; ok && time.Now().UTC().Sub(discTime) < ReconnectAfterDiscTimeout {
		return false
	}
	return true
}

func (m *ConnManager) Connected(id peer.ID, inbound bool) {
	m.peerMutex.Lock()
	defer m.peerMutex.Unlock()
	if inbound {
		m.inboundPeers[id] = struct{}{}
	} else {
		m.outboundPeers[id] = struct{}{}
	}
}

func (m *ConnManager) Disconnected(id peer.ID, reason error) {
	m.peerMutex.Lock()
	defer m.peerMutex.Unlock()

	m.discTimes[id] = time.Now().UTC()
	if reason == yamux.ErrStreamReset {
		m.resetTimes[id] = time.Now().UTC()
	}
	delete(m.inboundPeers, id)
	delete(m.outboundPeers, id)
}

func (m *ConnManager) BanPeer(id peer.ID) {
	m.bannedPeers.Add(id)
	if m.bannedPeers.Cardinality() > MaxBannedPeers {
		m.bannedPeers.Pop()
	}
}

func (m *ConnManager) DialRandomPeer() (network.Stream, error) {
	m.connMutex.Lock()
	conns := make([]network.Conn, 0, len(m.activeConnections))
	for _, c := range m.activeConnections {
		conns = append(conns, c)
	}
	m.connMutex.Unlock()

	filteredConns := make([]network.Conn, 0, len(conns))

	for _, c := range conns {
		id := c.RemotePeer()
		m.peerMutex.RLock()
		_, inbound := m.inboundPeers[id]
		_, outbound := m.outboundPeers[id]
		m.peerMutex.RUnlock()
		if !inbound && !outbound && m.CanConnect(id) {
			filteredConns = append(filteredConns, c)
		}
	}

	if len(filteredConns) == 0 {
		return nil, NoPeersToDial
	}

	for attempt := 0; attempt < dialPeerAttempts; attempt++ {
		idx := rand.Intn(len(filteredConns))
		conn := filteredConns[idx]

		if stream, err := m.findOrOpenStream(conn); err == nil {
			return stream, nil
		}
		if len(filteredConns) == 1 {
			break
		}
		filteredConns[idx] = filteredConns[len(filteredConns)-1]
		filteredConns[len(filteredConns)-1] = nil
		filteredConns = filteredConns[:len(filteredConns)-1]
	}
	return nil, FailedToDialPeer
}

func (m *ConnManager) findOrOpenStream(conn network.Conn) (network.Stream, error) {
	streams := conn.GetStreams()
	for _, s := range streams {
		if s.Protocol() == IdenaProtocol {
			return s, nil
		}
	}
	return m.newStream(conn.RemotePeer())
}

func (m *ConnManager) newStream(peerID peer.ID) (network.Stream, error) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	stream, err := m.host.NewStream(ctx, peerID, IdenaProtocol)

	select {
	case <-ctx.Done():
		err = errors.New("timeout while opening idena stream")
	default:
		break
	}
	cancel()
	return stream, err
}

func (m *ConnManager) filterUselessConnections(conns []network.Conn) []network.Conn {
	var result []network.Conn
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()
	for _, c := range conns {
		id := c.RemotePeer()
		if m.bannedPeers.Contains(id) {
			continue
		}
		if discTime, ok := m.discTimes[id]; ok && time.Now().UTC().Sub(discTime) < ReconnectAfterDiscTimeout {
			continue
		}
		if resetTime, ok := m.resetTimes[id]; ok && time.Now().UTC().Sub(resetTime) < ReconnectAfterResetTimeout {
			continue
		}
		if m.host.Network().Connectedness(id) != network.Connected {
			continue
		}

		if protos, err := m.host.Peerstore().SupportsProtocols(id, string(IdenaProtocol)); err == nil && len(protos) > 0 {
			result = append(result, c)
		}
	}
	return result
}

func (m *ConnManager) AddConnection(conn network.Conn) {

	go func() {
		id := conn.RemotePeer()
		if m.bannedPeers.Contains(id) {
			return
		}
		time.Sleep(time.Second * 5)

		m.connMutex.Lock()
		m.activeConnections[id] = conn
		m.connMutex.Unlock()
	}()
}

func (m *ConnManager) RemoveConnection(conn network.Conn) {
	m.connMutex.Lock()
	delete(m.activeConnections, conn.RemotePeer())
	m.connMutex.Unlock()
}

func (m *ConnManager) CanAcceptStream() bool {
	return len(m.inboundPeers) < m.cfg.MaxInboundPeers
}

func (m *ConnManager) CanDial() bool {
	return len(m.outboundPeers) < m.cfg.MaxOutboundPeers
}

func (m *ConnManager) GetRandomInboundPeer() peer.ID {
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()

	for k, _ := range m.inboundPeers {
		return k
	}
	return ""
}

func (m *ConnManager) GetRandomOutboundPeer() peer.ID {
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()

	for k, _ := range m.outboundPeers {
		return k
	}
	return ""
}
