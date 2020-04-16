package protocol

import (
	"context"
	"fmt"
	"github.com/coreos/go-semver/semver"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/entry"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/maputil"
	"github.com/idena-network/idena-go/config"
	"github.com/idena-network/idena-go/core/flip"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/pengings"
	"github.com/idena-network/idena-go/rlp"
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

	peers           *peerSet
	incomeBlocks    chan *types.Block
	proposals       *pengings.Proposals
	votes           *pengings.Votes
	pushPullManager *PushPullManager

	txpool              mempool.TransactionPool
	flipKeyPool         mempool.FlipKeysPool
	flipper             *flip.Flipper
	txChan              chan *events.NewTxEvent
	flipKeyChan         chan *events.NewFlipKeyEvent
	flipKeysPackageChan chan *events.NewFlipKeysPackageEvent
	incomeBatches       *sync.Map
	batchedLock         sync.Mutex
	bus                 eventbus.Bus
	wrongTime           bool
	appVersion          string

	log          log.Logger
	mutex        sync.Mutex
	pendingPeers map[peer.ID]struct{}
	metrics      *metricCollector
	connManager  *ConnManager
}

type metricCollector struct {
	incomeMessage  func(msg *Msg)
	outcomeMessage func(msg *Msg)
}

func NewIdenaGossipHandler(host core.Host, cfg config.P2P, chain *blockchain.Blockchain, proposals *pengings.Proposals, votes *pengings.Votes, txpool *mempool.TxPool, fp *flip.Flipper, bus eventbus.Bus, flipKeyPool *mempool.KeysPool, appVersion string) *IdenaGossipHandler {
	handler := &IdenaGossipHandler{
		host:                host,
		cfg:                 cfg,
		bcn:                 chain,
		peers:               newPeerSet(),
		incomeBlocks:        make(chan *types.Block, 1000),
		incomeBatches:       &sync.Map{},
		proposals:           proposals,
		votes:               votes,
		pushPullManager:     NewPushPullManager(),
		txpool:              mempool.NewAsyncTxPool(txpool),
		txChan:              make(chan *events.NewTxEvent, 100),
		flipKeyChan:         make(chan *events.NewFlipKeyEvent, 200),
		flipKeysPackageChan: make(chan *events.NewFlipKeysPackageEvent, 200),
		flipper:             fp,
		bus:                 bus,
		flipKeyPool:         mempool.NewAsyncKeysPool(flipKeyPool),
		appVersion:          appVersion,
		log:                 log.New(),
		pendingPeers:        make(map[peer.ID]struct{}),
		metrics:             new(metricCollector),
		connManager:         NewConnManager(host, cfg),
	}
	handler.pushPullManager.AddEntryHolder(pushVote, entry.NewDefaultHolder(3))
	handler.pushPullManager.AddEntryHolder(pushBlock, entry.NewDefaultHolder(3))
	handler.pushPullManager.AddEntryHolder(pushProof, entry.NewDefaultHolder(3))
	handler.pushPullManager.AddEntryHolder(pushFlip, entry.NewDefaultHolder(1))
	handler.pushPullManager.AddEntryHolder(pushKeyPackage, flipKeyPool)
	handler.pushPullManager.AddEntryHolder(pushTx, entry.NewDefaultHolder(1))
	handler.registerMetrics()
	return handler
}

func (h *IdenaGossipHandler) Start() {

	setHandler := func() {
		h.host.SetStreamHandler(IdenaProtocol, h.acceptStream)
		h.connManager = NewConnManager(h.host, h.cfg)
		notifiee := &notifiee{
			connManager: h.connManager,
		}
		h.host.Network().Notify(notifiee)
	}
	setHandler()

	h.bus.Subscribe(events.NewTxEventID, func(e eventbus.Event) {
		newTxEvent := e.(*events.NewTxEvent)
		h.txChan <- newTxEvent
	})
	h.bus.Subscribe(events.NewFlipKeyID, func(e eventbus.Event) {
		newFlipKeyEvent := e.(*events.NewFlipKeyEvent)
		h.flipKeyChan <- newFlipKeyEvent
	})
	h.bus.Subscribe(events.NewFlipKeysPackageID, func(e eventbus.Event) {
		newFlipKeysPackageEvent := e.(*events.NewFlipKeysPackageEvent)
		h.flipKeysPackageChan <- newFlipKeysPackageEvent
	})
	h.bus.Subscribe(events.NewFlipEventID, func(e eventbus.Event) {
		newFlipEvent := e.(*events.NewFlipEvent)
		h.sendFlip(newFlipEvent.Flip)
	})
	h.bus.Subscribe(events.IpfsPortChangedEventId, func(e eventbus.Event) {
		portChangedEvent := e.(*events.IpfsPortChangedEvent)
		h.host = portChangedEvent.Host
		setHandler()
	})

	go h.broadcastLoop()
	go h.checkTime()
	go h.background()
}

func (h *IdenaGossipHandler) background() {
	dialTicker := time.NewTicker(time.Second * 15)
	renewTicker := time.NewTimer(time.Minute * 5)

	for {
		select {
		case <-dialTicker.C:
			h.dialPeers()
		case <-renewTicker.C:
			h.renewPeers()
		}
	}
}

func (h *IdenaGossipHandler) checkTime() {
	for {
		h.wrongTime = !checkClockDrift()
		time.Sleep(time.Minute)
	}
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
		proposal := new(types.BlockProposal)
		if err := msg.Decode(proposal); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if h.isProcessed(proposal) {
			return nil
		}
		p.markPayload(proposal)
		if proposal.Block == nil || len(proposal.Signature) == 0 {
			return nil
		}
		// if peer proposes this msg it should be on `query.Round-1` height
		p.setHeight(proposal.Block.Height() - 1)
		if ok, _ := h.proposals.AddProposedBlock(proposal, p.id, time.Now().UTC(), nil); ok {
			h.ProposeBlock(proposal)
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
			p.sendMsg(Block, block, false)
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
		if h.isProcessed(flipKey) {
			return nil
		}
		p.markPayload(flipKey)
		h.flipKeyPool.AddPublicFlipKey(flipKey, false)
	case SnapshotManifest:
		manifest := new(snapshot.Manifest)
		if err := msg.Decode(manifest); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.manifest = manifest
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
	case Push:
		pushHash := new(pushPullHash)
		if err := msg.Decode(pushHash); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}

		if pushHash.Invalid() {
			return nil
		}

		p.markPayload(pushHash)
		h.pushPullManager.addPush(p.id, *pushHash)
	case Pull:
		var pullHash pushPullHash
		if err := msg.Decode(&pullHash); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if pullHash.Invalid() {
			return nil
		}
		if entry, ok := h.pushPullManager.GetEntry(pullHash); ok {
			h.sendEntry(p, pullHash, entry)
		}
	case Block:
		block := new(types.Block)
		if err := msg.Decode(block); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if h.isProcessed(block) {
			return nil
		}
		p.markPayload(block)
		h.proposals.AddBlock(block)
	}

	return nil
}

func (h *IdenaGossipHandler) acceptStream(stream network.Stream) {
	if h.connManager.CanConnect(stream.Conn().RemotePeer()) && h.connManager.CanAcceptStream() {
		h.runPeer(stream, true)
	}
}

func (h *IdenaGossipHandler) runPeer(stream network.Stream, inbound bool) (*protoPeer, error) {
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

	peer := newPeer(stream, h.cfg.MaxDelay, h.metrics)

	if err := peer.Handshake(h.bcn.Network(), h.bcn.Head.Height(), h.bcn.Genesis(), h.appVersion, uint32(h.peers.Len())); err != nil {
		current := semver.New(h.appVersion)
		if other, errS := semver.NewVersion(peer.appVersion); errS != nil || other.Major > current.Major || other.Minor >= current.Minor && other.Major == current.Major {
			peer.log.Debug("Idena handshake failed", "err", err)
		}
		return nil, err
	}
	h.peers.Register(peer)
	h.connManager.Connected(peer.id, inbound)
	h.host.ConnManager().TagPeer(peer.id, "idena", IdenaProtocolWeight)

	go h.runListening(peer)
	go peer.broadcast()

	go h.syncTxPool(peer)
	go h.syncFlipKeyPool(peer)

	h.sendManifest(peer)

	h.log.Info("Peer connected", "id", peer.id.Pretty(), "inbound", inbound)
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
	h.connManager.Disconnected(peerId, peer.transportErr)
	h.host.ConnManager().UntagPeer(peerId, "idena")

	h.log.Info("Peer disconnected", "id", peerId.Pretty())
}

func (h *IdenaGossipHandler) dialPeers() {
	go func() {
		for {
			if !h.connManager.CanDial() {
				return
			}
			stream, err := h.connManager.DialRandomPeer()
			if err != nil {
				h.log.Error("dial failed", "err", err)
				return
			}
			peer := h.peers.Peer(stream.Conn().RemotePeer())
			if peer == nil {
				h.runPeer(stream, false)
			}
		}
	}()
}

func (h *IdenaGossipHandler) renewPeers() {
	if !h.connManager.CanDial() {
		peerId := h.connManager.GetRandomOutboundPeer()
		peer := h.peers.Peer(peerId)
		if peer != nil {
			peer.disconnect()
		}
	}

	if !h.connManager.CanAcceptStream() {
		peerId := h.connManager.GetRandomInboundPeer()
		peer := h.peers.Peer(peerId)
		if peer != nil {
			peer.disconnect()
		}
	}
}

func (h *IdenaGossipHandler) BanPeer(peerId peer.ID, reason error) {
	h.connManager.BanPeer(peerId)

	peer := h.peers.Peer(peerId)
	if peer != nil {
		if reason != nil {
			peer.log.Info("peer has been banned", "reason", reason)
		}
		peer.stream.Reset()
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

func (h *IdenaGossipHandler) broadcastLoop() {
	for {
		select {
		case tx := <-h.txChan:
			h.broadcastTx(tx.Tx, tx.Own)
		case key := <-h.flipKeyChan:
			h.broadcastFlipKey(key.Key, key.Own)
		case key := <-h.flipKeysPackageChan:
			h.broadcastFlipKeysPackage(key.Key, key.Own)
		case pullReq := <-h.pushPullManager.Requests():
			h.sendPull(pullReq.peer, pullReq.hash)
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
	hash := pushPullHash{
		Type: pushProof,
		Hash: rlp.Hash128(payload),
	}
	h.pushPullManager.AddEntry(hash, payload)
	h.sendPush(hash)
}

func (h *IdenaGossipHandler) ProposeBlock(block *types.BlockProposal) {
	hash := pushPullHash{
		Type: pushBlock,
		Hash: rlp.Hash128(block),
	}
	h.pushPullManager.AddEntry(hash, block)
	h.sendPush(hash)
}
func (h *IdenaGossipHandler) SendVote(vote *types.Vote) {
	hash := pushPullHash{
		Type: pushVote,
		Hash: rlp.Hash128(vote),
	}
	h.pushPullManager.AddEntry(hash, vote)
	h.sendPush(hash)
}

func (h *IdenaGossipHandler) sendPush(hash pushPullHash) {
	h.peers.SendWithFilter(Push, hash, false)
}

func (h *IdenaGossipHandler) sendEntry(p *protoPeer, hash pushPullHash, entry interface{}) {
	switch hash.Type {
	case pushVote:
		p.sendMsg(Vote, entry, false)
	case pushBlock:
		p.sendMsg(ProposeBlock, entry, false)
	case pushProof:
		p.sendMsg(ProposeProof, entry, false)
	case pushFlip:
		p.sendMsg(FlipBody, entry, false)
	case pushKeyPackage:
		p.sendMsg(FlipKeysPackage, entry, false)
	case pushTx:
		p.sendMsg(NewTx, entry, false)
	default:
	}
}

func errResp(code int, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

func (h *IdenaGossipHandler) broadcastTx(tx *types.Transaction, own bool) {

	hash := pushPullHash{
		Type: pushTx,
		Hash: rlp.Hash128(tx),
	}
	h.pushPullManager.AddEntry(hash, tx)
	h.peers.SendWithFilter(Push, hash, own)
}

func (h *IdenaGossipHandler) sendFlip(flip *types.Flip) {

	hash := pushPullHash{
		Type: pushFlip,
		Hash: rlp.Hash128(flip),
	}
	h.pushPullManager.AddEntry(hash, flip)
	h.sendPush(hash)
}

func (h *IdenaGossipHandler) broadcastFlipKey(flipKey *types.PublicFlipKey, own bool) {
	h.peers.SendWithFilter(FlipKey, flipKey, own)
}
func (h *IdenaGossipHandler) broadcastFlipKeysPackage(flipKeysPackage *types.PrivateFlipKeysPackage, own bool) {
	hash := pushPullHash{
		Type: pushKeyPackage,
		Hash: rlp.Hash128(flipKeysPackage),
	}
	h.sendPush(hash)
}

func (h *IdenaGossipHandler) sendPull(peerId peer.ID, hash pushPullHash) {
	peer := h.peers.Peer(peerId)
	if peer != nil {
		peer.sendMsg(Pull, hash, false)
	}
}

func (h *IdenaGossipHandler) RequestBlockByHash(hash common.Hash) {
	h.peers.Send(GetBlockByHash, &getBlockBodyRequest{
		Hash: hash,
	})
}

func (h *IdenaGossipHandler) syncTxPool(p *protoPeer) {
	pending := h.txpool.GetPendingTransaction()
	for _, tx := range pending {
		payload := pushPullHash{
			Type: pushTx,
			Hash: rlp.Hash128(tx),
		}
		h.pushPullManager.AddEntry(payload, tx)
		p.sendMsg(Push, payload, false)
		p.markPayload(payload)
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

	keysPackages := h.flipKeyPool.GetFlipPackagesHashes()
	for _, hash := range keysPackages {
		payload := pushPullHash{
			Type: pushKeyPackage,
			Hash: hash,
		}
		p.sendMsg(Push, payload, false)
		p.markPayload(payload)
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

func (h *IdenaGossipHandler) AddPeer(url string) error {
	ma, err := multiaddr.NewMultiaddr(url)

	if err != nil {
		return err
	}

	transportAddr, peerId := peer.SplitAddr(ma)

	if transportAddr == nil || peerId == "" {
		return errors.New("invalid url")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)

	err = h.host.Connect(ctx, peer.AddrInfo{
		ID:    peerId,
		Addrs: []multiaddr.Multiaddr{transportAddr},
	})
	cancel()
	return err
}

func (h *IdenaGossipHandler) WrongTime() bool {
	return h.wrongTime
}

func (h *IdenaGossipHandler) IsConnected(id peer.ID) bool {
	return h.peers.Peer(id) != nil
}
