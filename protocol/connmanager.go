package protocol

import (
	"context"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/math"
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

	inboundPeers  map[peer.ID]common.ShardId
	outboundPeers map[peer.ID]common.ShardId

	peerMutex sync.RWMutex
	connMutex sync.Mutex
	host      core.Host
	cfg       config.P2P

	shardId common.ShardId
}

func NewConnManager(host core.Host, cfg config.P2P) *ConnManager {
	return &ConnManager{
		host:              host,
		cfg:               cfg,
		bannedPeers:       mapset.NewSet(),
		activeConnections: make(map[peer.ID]network.Conn),
		inboundPeers:      make(map[peer.ID]common.ShardId),
		outboundPeers:     make(map[peer.ID]common.ShardId),
		discTimes:         make(map[peer.ID]time.Time),
		resetTimes:        make(map[peer.ID]time.Time),
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

func (m *ConnManager) Connected(id peer.ID, inbound bool, shardId common.ShardId) {
	m.peerMutex.Lock()
	defer m.peerMutex.Unlock()
	if inbound {
		m.inboundPeers[id] = shardId
	} else {
		m.outboundPeers[id] = shardId
	}
}

func (m *ConnManager) PeersCntFromOtherShards() int {
	m.peerMutex.Lock()
	defer m.peerMutex.Unlock()
	var cnt int
	for _, s := range m.inboundPeers {
		if s != m.shardId && m.shardId != common.MultiShard {
			cnt++
		}
	}
	for _, s := range m.outboundPeers {
		if s != m.shardId && m.shardId != common.MultiShard {
			cnt++
		}
	}
	return cnt
}

func (m *ConnManager) PeersCntFromOwnShard() int {
	m.peerMutex.Lock()
	defer m.peerMutex.Unlock()
	var cnt int
	for _, s := range m.inboundPeers {
		if s == m.shardId || m.shardId == common.MultiShard {
			cnt++
		}
	}
	for _, s := range m.outboundPeers {
		if s == m.shardId || m.shardId == common.MultiShard {
			cnt++
		}
	}
	return cnt
}

func (m *ConnManager) SetShardId(shard common.ShardId) {
	m.shardId = shard
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
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()
	return len(m.inboundPeers) < m.cfg.MaxInboundPeers+m.cfg.MaxInboundOwnShardPeers
}

func (m *ConnManager) NeedInboundOwnShardPeers() bool {
	if m.shardId == common.MultiShard {
		return false
	}
	m.peerMutex.Lock()
	defer m.peerMutex.Unlock()
	var cnt int
	for _, s := range m.inboundPeers {
		if s == m.shardId {
			cnt++
		}
	}
	return cnt < m.cfg.MaxInboundOwnShardPeers
}

func (m *ConnManager) NeedOutboundOwnShardPeers() bool {
	if m.shardId == common.MultiShard {
		return false
	}
	m.peerMutex.Lock()
	defer m.peerMutex.Unlock()
	var cnt int
	for _, s := range m.outboundPeers {
		if s == m.shardId {
			cnt++
		}
	}
	return cnt < m.MaxOutboundOwnPeers()
}

func (m *ConnManager) MaxOutboundPeers() int {
	inBoundPeers := len(m.inboundPeers)
	maxOutbound := m.cfg.MaxOutboundPeers
	if inBoundPeers >= 7 {
		maxOutbound = 1
	}
	return math.MinInt(maxOutbound, m.cfg.MaxOutboundPeers)
}

func (m *ConnManager) MaxOutboundOwnPeers() int {
	inBoundPeers := len(m.inboundPeers)
	maxOutbound := m.cfg.MaxOutboundOwnShardPeers
	if inBoundPeers >= 10 {
		maxOutbound = 1
	}
	return math.MinInt(maxOutbound, m.cfg.MaxOutboundOwnShardPeers)
}

func (m *ConnManager) CanDial() bool {
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()
	return len(m.outboundPeers) < m.MaxOutboundPeers()+m.MaxOutboundOwnPeers()
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

func (m *ConnManager) GetRandomPeerFromAnotherShard(inbound bool) peer.ID {
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()
	if inbound {
		for k, s := range m.inboundPeers {
			if s != m.shardId {
				return k
			}
		}
	} else {
		for k, s := range m.outboundPeers {
			if s != m.shardId {
				return k
			}
		}
	}
	return ""
}

func (m *ConnManager) IsFromOwnShards(id common.ShardId) bool {
	if m.shardId == common.MultiShard {
		return false
	}
	return m.shardId == id
}
