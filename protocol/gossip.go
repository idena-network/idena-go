package protocol

import (
	"context"
	"fmt"
	"github.com/coreos/go-semver/semver"
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/maputil"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/flip"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/pengings"
	core "github.com/libp2p/go-libp2p-core"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var IdenaProtocol core.ProtocolID = "/idena/gossip/1.0.0"

var (
	batchId = uint32(1)
)

type IdenaGossipHandler struct {
	host core.Host
	cfg  config.P2P
	bcn  *blockchain.Blockchain

	peers        *peerSet
	incomeBlocks chan *types.Block
	proposals    *pengings.Proposals
	votes        *pengings.Votes

	txpool              mempool.TransactionPool
	flipKeyPool         *mempool.KeysPool
	flipper             *flip.Flipper
	txChan              chan *events.NewTxEvent
	flipKeyChan         chan *events.NewFlipKeyEvent
	flipKeysPackageChan chan *events.NewFlipKeysPackageEvent
	incomeBatches       *sync.Map
	batchedLock         sync.Mutex
	bus                 eventbus.Bus
	wrongTime           bool
	appVersion          string
	bannedPeers         mapset.Set
	refreshCh           chan struct{}
	log                 log.Logger
	mutex               sync.Mutex
	filteredConnections []network.Conn
	pendingPeers        map[peer.ID]struct{}
	discTimes           map[peer.ID]time.Time
	discMutex           sync.Mutex
}

func NewIdenaGossipHandler(host core.Host, cfg config.P2P, chain *blockchain.Blockchain, proposals *pengings.Proposals, votes *pengings.Votes, txpool *mempool.TxPool, fp *flip.Flipper, bus eventbus.Bus, flipKeyPool *mempool.KeysPool, appVersion string) *IdenaGossipHandler {
	handler := &IdenaGossipHandler{
		host:                host,
		cfg:                 cfg,
		bcn:                 chain,
		peers:               newPeerSet(cfg.MaxPeers),
		incomeBlocks:        make(chan *types.Block, 1000),
		incomeBatches:       &sync.Map{},
		proposals:           proposals,
		votes:               votes,
		txpool:              mempool.NewAsyncTxPool(txpool),
		txChan:              make(chan *events.NewTxEvent, 100),
		flipKeyChan:         make(chan *events.NewFlipKeyEvent, 200),
		flipKeysPackageChan: make(chan *events.NewFlipKeysPackageEvent, 200),
		flipper:             fp,
		bus:                 bus,
		flipKeyPool:         flipKeyPool,
		appVersion:          appVersion,
		bannedPeers:         mapset.NewSet(),
		refreshCh:           make(chan struct{}, 1),
		log:                 log.New(),
		pendingPeers:        make(map[peer.ID]struct{}),
		discTimes:           make(map[peer.ID]time.Time),
	}
	return handler
}

func (h *IdenaGossipHandler) Start() {

	h.host.SetStreamHandler(IdenaProtocol, h.streamHandler)
	notifiee := &idenaNotifiee{
		handler: h,
	}
	h.host.Network().Notify(notifiee)
	go func() {
		ticker := time.NewTicker(time.Second * 15)
		for {
			select {
			case <-ticker.C:
				h.refreshConnections()
			}
		}
	}()

	_ = h.bus.Subscribe(events.NewTxEventID, func(e eventbus.Event) {
		newTxEvent := e.(*events.NewTxEvent)
		h.txChan <- newTxEvent
	})
	_ = h.bus.Subscribe(events.NewFlipKeyID, func(e eventbus.Event) {
		newFlipKeyEvent := e.(*events.NewFlipKeyEvent)
		h.flipKeyChan <- newFlipKeyEvent
	})
	_ = h.bus.Subscribe(events.NewFlipKeysPackageID, func(e eventbus.Event) {
		newFlipKeysPackageEvent := e.(*events.NewFlipKeysPackageEvent)
		h.flipKeysPackageChan <- newFlipKeysPackageEvent
	})
	_ = h.bus.Subscribe(events.NewFlipEventID, func(e eventbus.Event) {
		newFlipEvent := e.(*events.NewFlipEvent)
		h.broadcastFlipCid(newFlipEvent.FlipCid)
	})

	go h.broadcastLoop()
	go h.checkTime()
}

func (h *IdenaGossipHandler) checkTime() {
	for {
		h.wrongTime = !checkClockDrift()
		time.Sleep(time.Minute)
	}
}

func (h *IdenaGossipHandler) WrongTime() bool {
	return h.wrongTime
}

func (h *IdenaGossipHandler) handle(p *protoPeer) error {
	msg, err := p.ReadMsg()
	if err != nil {
		return err
	}
	switch msg.Code {

	case BlocksRange:
		var response blockRange
		if err := msg.Decode(&response); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.log.Trace("Income blocks range", "batchId", response.BatchId)
		if ib, ok := h.incomeBatches.Load(p.id); ok {
			peerBatches := ib.(*sync.Map)
			if pb, ok := peerBatches.Load(response.BatchId); ok {
				batch := pb.(*batch)
				for _, b := range response.Blocks {
					batch.headers <- b
					p.setHeight(b.Header.Height())
				}
				close(batch.headers)
				h.batchedLock.Lock()
				peerBatches.Delete(response.BatchId)
				if maputil.IsSyncMapEmpty(peerBatches) {
					h.incomeBatches.Delete(p.id)
				}
				h.batchedLock.Unlock()
			}
		}

	case ProposeProof:
		query := new(proposeProof)
		if err := msg.Decode(query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}

		if h.isProcessed(query) {
			return nil
		}
		p.markPayload(query)
		// if peer proposes this msg it should be on `query.Round-1` height
		p.setHeight(query.Round - 1)
		if ok, _ := h.proposals.AddProposeProof(query.Proof, query.Hash, query.PubKey, query.Round); ok {
			h.proposeProof(query)
		}
	case ProposeBlock:
		block := new(types.Block)
		if err := msg.Decode(block); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if h.isProcessed(block) {
			return nil
		}
		p.markPayload(block)
		// if peer proposes this msg it should be on `query.Round-1` height
		p.setHeight(block.Height() - 1)
		if ok, _ := h.proposals.AddProposedBlock(block, p.id, time.Now().UTC()); ok {
			h.ProposeBlock(block)
		}
	case Vote:
		vote := new(types.Vote)
		if err := msg.Decode(vote); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if h.isProcessed(vote) {
			return nil
		}
		p.markPayload(vote)
		p.setPotentialHeight(vote.Header.Round - 1)
		if h.votes.AddVote(vote) {
			h.SendVote(vote)
		}
	case NewTx:
		tx := new(types.Transaction)
		if err := msg.Decode(tx); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if h.isProcessed(tx) {
			return nil
		}
		p.markPayload(tx)
		h.txpool.Add(tx)
	case GetBlockByHash:
		var query getBlockBodyRequest
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		block := h.bcn.GetBlock(query.Hash)
		if block != nil {
			p.sendMsg(ProposeBlock, block, false)
		}
	case GetBlocksRange:
		var query getBlocksRangeRequest
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		h.provideBlocks(p, query.BatchId, query.From, query.To)
	case GetForkBlockRange:
		query := new(getForkBlocksRangeRequest)
		if err := msg.Decode(query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		h.provideForkBlocks(p, query.BatchId, query.Blocks)
	case FlipBody:
		f := new(types.Flip)
		if err := msg.Decode(f); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if h.isProcessed(f) {
			return nil
		}
		p.markPayload(f)
		h.flipper.AddNewFlip(f, false)
	case FlipKey:
		flipKey := new(types.PublicFlipKey)
		if err := msg.Decode(flipKey); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		h.flipKeyPool.AddPublicFlipKey(flipKey, false)
	case SnapshotManifest:
		manifest := new(snapshot.Manifest)
		if err := msg.Decode(manifest); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.manifest = manifest
	case PushFlipCid:
		cid := new(flipCid)
		if err := msg.Decode(cid); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if h.isProcessed(cid) {
			return nil
		}
		p.markPayload(cid)
		if !h.flipper.Has(cid.Cid) {
			p.sendMsg(PullFlip, cid, false)
		}
	case PullFlip:
		cid := new(flipCid)
		if err := msg.Decode(cid); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if f, err := h.flipper.ReadFlip(cid.Cid); err == nil {
			p.sendMsg(FlipBody, f, false)
		}
	case FlipKeysPackage:
		keysPackage := new(types.PrivateFlipKeysPackage)
		if err := msg.Decode(keysPackage); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if h.isProcessed(keysPackage) {
			return nil
		}
		p.markPayload(keysPackage)
		h.flipKeyPool.AddPrivateKeysPackage(keysPackage, false)
	case FlipKeysPackageCid:
		packageCid := new(types.PrivateFlipKeysPackageCid)
		if err := msg.Decode(packageCid); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if h.isProcessed(packageCid) {
			return nil
		}
		p.markPayload(packageCid)
		h.flipKeyPool.AddPrivateKeysPackageCid(packageCid)
	}

	return nil
}

func (h *IdenaGossipHandler) streamHandler(stream network.Stream) {
	id := stream.Conn().RemotePeer()
	if h.bannedPeers.Contains(id) {
		stream.Reset()
		return
	}
	if discTime, ok := h.discTimes[id]; ok && time.Now().UTC().Sub(discTime) < ReconnectAfterDiscTimeout {
		return
	}

	h.runPeer(stream)
}

func (h *IdenaGossipHandler) runPeer(stream network.Stream) (*protoPeer, error) {
	peerId := stream.Conn().RemotePeer()
	h.mutex.Lock()
	if p := h.peers.Peer(peerId); p != nil {
		h.mutex.Unlock()
		return p, errors.New("peer already connected")
	}
	if _, ok := h.pendingPeers[peerId]; ok {
		h.mutex.Unlock()
		return nil, errors.New("peer is already connecting")
	}
	h.pendingPeers[peerId] = struct{}{}
	h.mutex.Unlock()

	defer func() {
		h.mutex.Lock()
		delete(h.pendingPeers, peerId)
		h.mutex.Unlock()
	}()

	peer := newPeer(stream, h.cfg.MaxDelay)

	if err := peer.Handshake(h.bcn.Network(), h.bcn.Head.Height(), h.bcn.Genesis(), h.appVersion); err != nil {
		current := semver.New(h.appVersion)
		if other, errS := semver.NewVersion(peer.appVersion); errS != nil || other.Major > current.Major || other.Minor >= current.Minor && other.Major == current.Major {
			peer.log.Debug("Idena handshake failed", "err", err)
		}
		return nil, err
	}
	h.peers.Register(peer)
	h.host.ConnManager().TagPeer(peer.id, "idena", IdenaProtocolWeight)

	go h.runListening(peer)
	go peer.broadcast()
	go h.syncTxPool(peer)
	go h.syncFlipKeyPool(peer)
	h.sendManifest(peer)

	h.log.Debug("Peer connected", "id", peer.id.Pretty())
	return peer, nil
}

func (h *IdenaGossipHandler) unregisterPeer(peerId peer.ID) {
	peer := h.peers.Peer(peerId)
	if peer == nil {
		return
	}
	if err := h.peers.Unregister(peerId); err != nil {
		return
	}
	close(peer.term)
	peer.disconnect()
	h.log.Debug("Peer disconnected", "id", peerId.Pretty())
	h.host.ConnManager().UntagPeer(peerId, "idena")
	h.discMutex.Lock()
	h.discTimes[peerId] = time.Now().UTC()
	h.discMutex.Unlock()
}

func (h *IdenaGossipHandler) findOrOpenStream(conn network.Conn) (network.Stream, error) {
	streams := conn.GetStreams()

	var idenaStream network.Stream
	for _, s := range streams {
		if s.Protocol() == IdenaProtocol {
			idenaStream = s
		}
	}
	if idenaStream != nil {
		return idenaStream, nil
	}

	h.log.Trace("try to open a new stream", "peer", conn.RemotePeer())
	return h.newStream(conn.RemotePeer())
}

func (h *IdenaGossipHandler) refreshPeers() {
	go func() {
		for _, c := range h.filteredConnections {
			idenaStream, err := h.findOrOpenStream(c)
			if err != nil {
				h.log.Debug("failed to open a new stream", "err", err)
			}
			if idenaStream == nil {
				continue
			}
			peer := h.peers.Peer(idenaStream.Conn().RemotePeer())
			if peer == nil {
				h.runPeer(idenaStream)
			}
		}
	}()
}

func (h *IdenaGossipHandler) refreshConnections() {

	go func() {
		select {
		case h.refreshCh <- struct{}{}:
		default:
			return
		}
		defer func() {
			<-h.refreshCh
		}()
		h.filteredConnections = h.filterUselessConnections(h.host.Network().Conns())
		h.refreshPeers()
	}()
}

func (h *IdenaGossipHandler) newStream(peerID peer.ID) (network.Stream, error) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	stream, err := h.host.NewStream(ctx, peerID, IdenaProtocol)

	select {
	case <-ctx.Done():
		err = errors.New("timeout while opening idena stream")
	default:
		break
	}
	cancel()
	return stream, err
}

func (h *IdenaGossipHandler) filterUselessConnections(conns []network.Conn) []network.Conn {
	var result []network.Conn
	for _, c := range conns {
		id := c.RemotePeer()
		if discTime, ok := h.discTimes[id]; (!ok ||
			time.Now().UTC().Sub(discTime) > ReconnectAfterDiscTimeout ||
			h.peers.Len() < h.cfg.MaxPeers) && !h.bannedPeers.Contains(id) {
			result = append(result, c)
		}
	}

	return result
}

func (h *IdenaGossipHandler) BanPeer(peerId peer.ID, reason error) {
	h.bannedPeers.Add(peerId)
	if h.bannedPeers.Cardinality() > MaxBannedPeers {
		h.bannedPeers.Pop()
	}

	peer := h.peers.Peer(peerId)
	if peer != nil {
		if reason != nil {
			peer.log.Info("peer has been banned", "reason", reason)
		}
		peer.stream.Close()
	}
}

func (h *IdenaGossipHandler) isProcessed(payload interface{}) bool {
	return h.peers.HasPayload(payload)
}

func (h *IdenaGossipHandler) provideBlocks(p *protoPeer, batchId uint32, from uint64, to uint64) {
	var result []*block
	p.log.Trace("blocks requested", "from", from, "to", to)
	for i := from; i <= to; i++ {
		b := h.bcn.GetBlockHeaderByHeight(i)
		if b != nil {
			result = append(result, &block{
				Header:       b,
				Cert:         h.bcn.GetCertificate(b.Hash()),
				IdentityDiff: h.bcn.GetIdentityDiff(b.Height()),
			})
		} else {
			p.log.Warn("Do not have requested block", "height", i)
			break
		}
	}
	p.log.Trace("blocks returned", "len", len(result))
	p.sendMsg(BlocksRange, &blockRange{
		BatchId: batchId,
		Blocks:  result,
	}, false)
}

func (h *IdenaGossipHandler) provideForkBlocks(p *protoPeer, batchId uint32, blocks []common.Hash) {
	var result []*block

	bundles := h.bcn.ReadBlockForForkedPeer(blocks)
	for _, b := range bundles {
		result = append(result, &block{
			Header: b.Block.Header,
			Cert:   b.Cert,
		})
	}
	p.log.Info("Peer is in fork. Providing own blocks", "cnt", len(bundles))
	p.sendMsg(BlocksRange, &blockRange{
		BatchId: batchId,
		Blocks:  result,
	}, false)
}

func (h *IdenaGossipHandler) runListening(peer *protoPeer) {
	defer h.unregisterPeer(peer.id)
	for {
		if err := h.handle(peer); err != nil {
			peer.log.Debug("Idena message handling failed", "err", err)
			return
		}
	}
}

func (h *IdenaGossipHandler) GetKnownHeights() map[peer.ID]uint64 {
	result := make(map[peer.ID]uint64)
	peers := h.peers.Peers()
	if len(peers) == 0 {
		return nil
	}
	for _, peer := range peers {
		result[peer.id] = peer.knownHeight
	}
	return result
}

func (h *IdenaGossipHandler) GetKnownManifests() map[peer.ID]*snapshot.Manifest {
	result := make(map[peer.ID]*snapshot.Manifest)
	peers := h.peers.Peers()
	if len(peers) == 0 {
		return result
	}
	for _, peer := range peers {
		if peer.manifest != nil {
			result[peer.id] = peer.manifest
		}
	}
	return result
}

func (h *IdenaGossipHandler) GetBlocksRange(peerId peer.ID, from uint64, to uint64) (*batch, error) {
	peer := h.peers.Peer(peerId)
	if peer == nil {
		return nil, errors.New("protoPeer is not found")
	}

	b := &batch{
		from:    from,
		to:      to,
		p:       peer,
		headers: make(chan *block, to-from+1),
	}
	h.batchedLock.Lock()
	peerBatches, ok := h.incomeBatches.Load(peerId)
	if !ok {
		peerBatches = &sync.Map{}
		h.incomeBatches.Store(peerId, peerBatches)
	}
	id := atomic.AddUint32(&batchId, 1)
	peerBatches.(*sync.Map).Store(id, b)
	h.batchedLock.Unlock()
	peer.sendMsg(GetBlocksRange, &getBlocksRangeRequest{
		BatchId: batchId,
		From:    from,
		To:      to,
	}, false)
	return b, nil
}

func (h *IdenaGossipHandler) GetForkBlockRange(peerId peer.ID, ownBlocks []common.Hash) (*batch, error) {
	peer := h.peers.Peer(peerId)
	if peer == nil {
		return nil, errors.New("peer is not found")
	}
	b := &batch{
		p:       peer,
		headers: make(chan *block, 100),
	}
	h.batchedLock.Lock()
	peerBatches, ok := h.incomeBatches.Load(peerId)
	if !ok {
		peerBatches = &sync.Map{}
		h.incomeBatches.Store(peerId, peerBatches)
	}
	id := atomic.AddUint32(&batchId, 1)
	peerBatches.(*sync.Map).Store(id, b)
	h.batchedLock.Unlock()
	peer.sendMsg(GetForkBlockRange, &getForkBlocksRangeRequest{
		BatchId: batchId,
		Blocks:  ownBlocks,
	}, false)
	return b, nil
}

func (h *IdenaGossipHandler) ProposeProof(round uint64, hash common.Hash, proof []byte, pubKey []byte) {
	payload := &proposeProof{
		Round:  round,
		Hash:   hash,
		PubKey: pubKey,
		Proof:  proof,
	}
	h.proposeProof(payload)
}

func (h *IdenaGossipHandler) proposeProof(payload *proposeProof) {
	h.peers.SendWithFilter(ProposeProof, payload, false)
}

func (h *IdenaGossipHandler) ProposeBlock(block *types.Block) {
	h.peers.SendWithFilter(ProposeBlock, block, false)
}
func (h *IdenaGossipHandler) SendVote(vote *types.Vote) {
	h.peers.SendWithFilter(Vote, vote, false)
}

func errResp(code int, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

func (h *IdenaGossipHandler) broadcastLoop() {
	for {
		select {
		case tx := <-h.txChan:
			h.broadcastTx(tx.Tx, tx.Own)
		case key := <-h.flipKeyChan:
			h.broadcastFlipKey(key.Key, key.Own)
		case key := <-h.flipKeysPackageChan:
			h.broadcastFlipKeysPackage(key.Key, key.Own)
		}
	}
}
func (h *IdenaGossipHandler) broadcastTx(tx *types.Transaction, own bool) {
	h.peers.SendWithFilter(NewTx, tx, own)
}

func (h *IdenaGossipHandler) broadcastFlipCid(cid []byte) {
	h.peers.SendWithFilter(PushFlipCid, &flipCid{cid}, false)
}

func (h *IdenaGossipHandler) broadcastFlipKey(flipKey *types.PublicFlipKey, own bool) {
	h.peers.SendWithFilter(FlipKey, flipKey, own)
}
func (h *IdenaGossipHandler) broadcastFlipKeysPackage(flipKeysPackage *types.PrivateFlipKeysPackage, own bool) {
	h.peers.SendWithFilter(FlipKeysPackage, flipKeysPackage, own)
}

func (h *IdenaGossipHandler) RequestBlockByHash(hash common.Hash) {
	h.peers.Send(GetBlockByHash, &getBlockBodyRequest{
		Hash: hash,
	})
}

func (h *IdenaGossipHandler) syncTxPool(p *protoPeer) {
	pending := h.txpool.GetPendingTransaction()
	for _, tx := range pending {
		p.sendMsg(NewTx, tx, false)
		p.markPayload(tx)
	}
}

func (h *IdenaGossipHandler) sendManifest(p *protoPeer) {
	manifest := h.bcn.ReadSnapshotManifest()
	if manifest == nil {
		return
	}
	p.sendMsg(SnapshotManifest, manifest, true)
}

func (h *IdenaGossipHandler) syncFlipKeyPool(p *protoPeer) {
	keys := h.flipKeyPool.GetFlipKeys()
	for _, key := range keys {
		p.sendMsg(FlipKey, key, false)
	}

	keysPackages := h.flipKeyPool.GetFlipPackagesCids()
	for _, flipPackageCid := range keysPackages {
		p.sendMsg(FlipKeysPackageCid, flipPackageCid, false)
	}
}
func (h *IdenaGossipHandler) PotentialForwardPeers(round uint64) []peer.ID {
	var result []peer.ID
	for _, p := range h.peers.Peers() {
		if p.potentialHeight >= round {
			result = append(result, p.id)
		}
	}
	return result
}

func (h *IdenaGossipHandler) HasPeers() bool {
	return len(h.peers.peers) > 0
}
func (h *IdenaGossipHandler) PeersCount() int {
	return len(h.peers.peers)
}
func (h *IdenaGossipHandler) Peers() []*protoPeer {
	return h.peers.Peers()
}

func (h *IdenaGossipHandler) PeerHeights() []uint64 {
	result := make([]uint64, 0)
	peers := h.peers.Peers()
	for _, peer := range peers {
		result = append(result, peer.knownHeight)
	}
	return result
}

func (h *IdenaGossipHandler) Endpoint() string {
	addrs := h.host.Network().ListenAddresses()
	for _, a := range addrs {
		addrStr := a.String()
		if strings.Contains(addrStr, "ip4") {
			return fmt.Sprintf("%s/ipfs/%s", addrStr, h.host.ID().Pretty())
		}
	}
	return h.host.ID().Pretty()
}

func (h *IdenaGossipHandler) AddPeer(peerId peer.ID, addr string) error {
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)

	err = h.host.Connect(ctx, peer.AddrInfo{
		ID:    peerId,
		Addrs: []multiaddr.Multiaddr{ma},
	})
	cancel()
	return err
}

type idenaNotifiee struct {
	handler *IdenaGossipHandler
}

func (i *idenaNotifiee) Listen(network.Network, multiaddr.Multiaddr) {

}

func (i *idenaNotifiee) ListenClose(network.Network, multiaddr.Multiaddr) {

}

func (i *idenaNotifiee) Connected(net network.Network, conn network.Conn) {
}

func (i *idenaNotifiee) Disconnected(net network.Network, conn network.Conn) {
}

func (i *idenaNotifiee) OpenedStream(net network.Network, conn network.Stream) {
}

func (i *idenaNotifiee) ClosedStream(net network.Network, stream network.Stream) {
}
