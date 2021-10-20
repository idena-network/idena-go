package protocol

import (
	"context"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/config"
	core "github.com/libp2p/go-libp2p-core"
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-yamux"
	"github.com/pkg/errors"
	"math/rand"
	"strings"
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

	ownShardId common.ShardId
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

	if resetTime, ok := m.resetTimes[id]; ok && time.Now().UTC().Sub(resetTime) < ReconnectAfterResetTimeout {
		return false
	}
	if m.host.Network().Connectedness(id) != network.Connected {
		return false
	}
	protos, err := m.host.Peerstore().GetProtocols(id)
	if err != nil {
		return false
	}
	for _, p := range protos {
		if strings.Contains(p, IdenaProtocolPath) {
			return true
		}
	}
	return false
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

func (m *ConnManager) UpdatePeerShardId(id peer.ID, shardId common.ShardId) {
	m.peerMutex.Lock()
	defer m.peerMutex.Unlock()
	if _, ok := m.inboundPeers[id]; ok {
		m.inboundPeers[id] = shardId
		return
	}
	if _, ok := m.outboundPeers[id]; ok {
		m.outboundPeers[id] = shardId
		return
	}
}

func (m *ConnManager) SetShardId(shard common.ShardId) (updated bool) {
	prevShardId := m.ownShardId
	m.ownShardId = shard
	return prevShardId != shard
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
	matcher, _ := helpers.MultistreamSemverMatcher(IdenaProtocol)
	for _, s := range streams {
		if matcher(string(s.Protocol())) {
			return s, nil
		}
	}
	return m.newStream(conn.RemotePeer())
}

func (m *ConnManager) newStream(peerID peer.ID) (network.Stream, error) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	protos, err := m.host.Peerstore().GetProtocols(peerID)
	if err != nil {
		return nil, err
	}

	var idenaProtocol protocol.ID
	for _, p := range protos {
		if strings.Contains(p, IdenaProtocolPath) {
			idenaProtocol = protocol.ID(p)
			break
		}
	}
	if idenaProtocol == "" {
		return nil, errors.New("peer doesn't support idena protocol")
	}
	stream, err := m.host.NewStream(ctx, peerID, idenaProtocol)

	select {
	case <-ctx.Done():
		err = errors.New("timeout while opening idena stream")
	default:
		break
	}

	return stream, err
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

func (m *ConnManager) NeedPeerFromSomeShard(shardsNum int) bool {
	if m.ownShardId != common.MultiShard {
		return false
	}
	for i := common.ShardId(1); i <= common.ShardId(shardsNum); i++ {
		if m.PeersFromShard(i) == 0 {
			return true
		}
	}
	return false
}

func (m *ConnManager) NeedInboundOwnShardPeers() bool {
	if m.ownShardId == common.MultiShard {
		return false
	}
	m.peerMutex.Lock()
	defer m.peerMutex.Unlock()
	var cnt int
	for _, s := range m.inboundPeers {
		if s == m.ownShardId {
			cnt++
		}
	}
	return cnt < m.cfg.MaxInboundOwnShardPeers
}

func (m *ConnManager) NeedOutboundOwnShardPeers() bool {
	if m.ownShardId == common.MultiShard {
		return false
	}
	m.peerMutex.Lock()
	defer m.peerMutex.Unlock()
	var cnt int
	for _, s := range m.outboundPeers {
		if s == m.ownShardId {
			cnt++
		}
	}
	return cnt < m.MaxOutboundOwnPeers()
}

func (m *ConnManager) MaxOutboundPeers() int {
	return m.cfg.MaxOutboundPeers
}

func (m *ConnManager) MaxOutboundOwnPeers() int {
	return m.cfg.MaxOutboundOwnShardPeers
}

func (m *ConnManager) CanDial() bool {
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()
	return len(m.outboundPeers) < m.MaxOutboundPeers()+m.MaxOutboundOwnPeers()
}

func (m *ConnManager) GetRandomPeer(inbound bool) peer.ID {
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()

	var peersMap map[peer.ID]common.ShardId
	if inbound {
		peersMap = m.inboundPeers
	} else {
		peersMap = m.outboundPeers
	}

	for k, s := range peersMap {
		if s == common.MultiShard {
			return k
		}
		if s != m.ownShardId && m.ownShardId != common.MultiShard {
			return k
		}
		if m.peersCntFromShard(s) > 1 {
			return k
		}
	}
	return ""
}

func (m *ConnManager) PeerForDisconnect(inbound bool, newPeerShardId common.ShardId) peer.ID {
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()

	canDisconnect := func(oldPeerShardId common.ShardId) bool {
		if oldPeerShardId == common.MultiShard {
			return true
		}
		if oldPeerShardId == newPeerShardId {
			return false
		}
		if m.ownShardId == common.MultiShard {
			return m.peersCntFromShard(oldPeerShardId) > 1
		}
		return oldPeerShardId != newPeerShardId
	}

	if inbound {
		for k, s := range m.inboundPeers {
			if canDisconnect(s) {
				return k
			}
		}
	} else {
		for k, s := range m.outboundPeers {
			if canDisconnect(s) {
				return k
			}
		}
	}
	return ""
}

func (m *ConnManager) IsFromOwnShards(id common.ShardId) bool {
	if m.ownShardId == common.MultiShard {
		return false
	}
	return m.ownShardId == id
}

func (m *ConnManager) PeersFromShard(shardId common.ShardId) int {
	m.peerMutex.RLock()
	defer m.peerMutex.RUnlock()
	return m.peersCntFromShard(shardId)
}

func (m *ConnManager) peersCntFromShard(shardId common.ShardId) int {
	var cnt int
	for _, s := range m.outboundPeers {
		if s == shardId {
			cnt++
		}
	}
	for _, s := range m.inboundPeers {
		if s == shardId {
			cnt++
		}
	}
	return cnt
}
