package protocol

import (
	"fmt"
	"github.com/coreos/go-semver/semver"
	mapset "github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
	"github.com/idena-network/idena-go/common/maputil"
	"github.com/idena-network/idena-go/core/flip"
	"github.com/idena-network/idena-go/core/mempool"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/events"
	"github.com/idena-network/idena-go/p2p"
	"github.com/idena-network/idena-go/pengings"
	"github.com/pkg/errors"
	"sync"
	"sync/atomic"
	"time"
)

const (
	Handshake          = 0x01
	ProposeBlock       = 0x02
	ProposeProof       = 0x03
	Vote               = 0x04
	NewTx              = 0x05
	GetBlockByHash     = 0x06
	GetBlocksRange     = 0x07
	BlocksRange        = 0x08
	FlipBody           = 0x09
	FlipKey            = 0x0A
	SnapshotManifest   = 0x0B
	PushFlipCid        = 0x0C
	PullFlip           = 0x0D
	GetForkBlockRange  = 0x0E
	FlipKeysPackage    = 0x0F
	FlipKeysPackageCid = 0x10
)
const (
	DecodeErr              = 1
	MaxTimestampLagSeconds = 15
	MaxBannedPeers         = 500000
)

const (
	Version = 1
)

var (
	batchId = uint32(1)
)

type ProtocolManager struct {
	bcn *blockchain.Blockchain

	peers *peerSet

	heads chan *peerHead

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
	config              *p2p.Config
	wrongTime           bool
	appVersion          string
	bannedPeers         mapset.Set
}

type getBlockBodyRequest struct {
	Hash common.Hash
}

type getBlocksRangeRequest struct {
	BatchId uint32
	From    uint64
	To      uint64
}

type getForkBlocksRangeRequest struct {
	BatchId uint32
	Blocks  []common.Hash
}

type proposeProof struct {
	Hash   common.Hash
	Proof  []byte
	PubKey []byte
	Round  uint64
}

type peerHead struct {
	height uint64
	peer   *peer
}

type handshakeData struct {
	NetworkId    types.Network
	Height       uint64
	GenesisBlock common.Hash
	Timestamp    uint64
	Protocol     uint16
	AppVersion   string
}

func NetProtocolManager(chain *blockchain.Blockchain, proposals *pengings.Proposals, votes *pengings.Votes, txpool *mempool.TxPool, fp *flip.Flipper, bus eventbus.Bus, flipKeyPool *mempool.KeysPool, config *p2p.Config, appVersion string) *ProtocolManager {
	return &ProtocolManager{
		bcn:                 chain,
		peers:               newPeerSet(),
		heads:               make(chan *peerHead, 10),
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
		config:              config,
		appVersion:          appVersion,
		bannedPeers:         mapset.NewSet(),
	}
}

func (pm *ProtocolManager) Start() {
	_ = pm.bus.Subscribe(events.NewTxEventID, func(e eventbus.Event) {
		newTxEvent := e.(*events.NewTxEvent)
		pm.txChan <- newTxEvent
	})
	_ = pm.bus.Subscribe(events.NewFlipKeyID, func(e eventbus.Event) {
		newFlipKeyEvent := e.(*events.NewFlipKeyEvent)
		pm.flipKeyChan <- newFlipKeyEvent
	})
	_ = pm.bus.Subscribe(events.NewFlipKeysPackageID, func(e eventbus.Event) {
		newFlipKeysPackageEvent := e.(*events.NewFlipKeysPackageEvent)
		pm.flipKeysPackageChan <- newFlipKeysPackageEvent
	})
	_ = pm.bus.Subscribe(events.NewFlipEventID, func(e eventbus.Event) {
		newFlipEvent := e.(*events.NewFlipEvent)
		pm.broadcastFlipCid(newFlipEvent.FlipCid)
	})

	go pm.broadcastLoop()
	go pm.checkTime()
}

func (pm *ProtocolManager) checkTime() {
	for {
		pm.wrongTime = !checkClockDrift()
		time.Sleep(time.Minute)
	}
}

func (pm *ProtocolManager) WrongTime() bool {
	return pm.wrongTime
}

func (pm *ProtocolManager) handle(p *peer) error {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	defer msg.Discard()
	switch msg.Code {

	case BlocksRange:
		var response blockRange
		if err := msg.Decode(&response); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.Log().Trace("Income blocks range", "batchId", response.BatchId)
		if ib, ok := pm.incomeBatches.Load(p.id); ok {
			peerBatches := ib.(*sync.Map)
			if pb, ok := peerBatches.Load(response.BatchId); ok {
				batch := pb.(*batch)
				for _, b := range response.Blocks {
					batch.headers <- b
					p.setHeight(b.Header.Height())
				}
				close(batch.headers)
				pm.batchedLock.Lock()
				peerBatches.Delete(response.BatchId)
				if maputil.IsSyncMapEmpty(peerBatches) {
					pm.incomeBatches.Delete(p.id)
				}
				pm.batchedLock.Unlock()
			}
		}

	case ProposeProof:
		query := new(proposeProof)
		if err := msg.Decode(query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}

		if pm.isProcessed(query) {
			return nil
		}
		p.markPayload(query)
		// if peer proposes this msg it should be on `query.Round-1` height
		p.setHeight(query.Round - 1)
		if ok, _ := pm.proposals.AddProposeProof(query.Proof, query.Hash, query.PubKey, query.Round); ok {
			pm.proposeProof(query)
		}
	case ProposeBlock:
		block := new(types.Block)
		if err := msg.Decode(block); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if pm.isProcessed(block) {
			return nil
		}
		p.markPayload(block)
		// if peer proposes this msg it should be on `query.Round-1` height
		p.setHeight(block.Height() - 1)
		if ok, _ := pm.proposals.AddProposedBlock(block, p.id, time.Now().UTC()); ok {
			pm.ProposeBlock(block)
		}
	case Vote:
		vote := new(types.Vote)
		if err := msg.Decode(vote); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if pm.isProcessed(vote) {
			return nil
		}
		p.markPayload(vote)
		p.setPotentialHeight(vote.Header.Round - 1)
		if pm.votes.AddVote(vote) {
			pm.SendVote(vote)
		}
	case NewTx:
		tx := new(types.Transaction)
		if err := msg.Decode(tx); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if pm.isProcessed(tx) {
			return nil
		}
		p.markPayload(tx)
		pm.txpool.Add(tx)
	case GetBlockByHash:
		var query getBlockBodyRequest
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		block := pm.bcn.GetBlock(query.Hash)
		if block != nil {
			p.sendMsg(ProposeBlock, block, false)
		}
	case GetBlocksRange:
		var query getBlocksRangeRequest
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		pm.provideBlocks(p, query.BatchId, query.From, query.To)
	case GetForkBlockRange:
		query := new(getForkBlocksRangeRequest)
		if err := msg.Decode(query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		pm.provideForkBlocks(p, query.BatchId, query.Blocks)
	case FlipBody:
		f := new(types.Flip)
		if err := msg.Decode(f); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if pm.isProcessed(f) {
			return nil
		}
		p.markPayload(f)
		pm.flipper.AddNewFlip(f, false)
	case FlipKey:
		flipKey := new(types.PublicFlipKey)
		if err := msg.Decode(flipKey); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if pm.isProcessed(flipKey) {
			return nil
		}
		p.markPayload(flipKey)
		pm.flipKeyPool.AddPublicFlipKey(flipKey, false)
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
		if pm.isProcessed(cid) {
			return nil
		}
		p.markPayload(cid)
		if !pm.flipper.Has(cid.Cid) {
			p.sendMsg(PullFlip, cid, false)
		}
	case PullFlip:
		cid := new(flipCid)
		if err := msg.Decode(cid); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if f, err := pm.flipper.ReadFlip(cid.Cid); err == nil {
			p.sendMsg(FlipBody, f, false)
		}
	case FlipKeysPackage:
		keysPackage := new(types.PrivateFlipKeysPackage)
		if err := msg.Decode(keysPackage); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if pm.isProcessed(keysPackage) {
			return nil
		}
		p.markPayload(keysPackage)
		pm.flipKeyPool.AddPrivateKeysPackage(keysPackage, false)
	case FlipKeysPackageCid:
		packageCid := new(types.PrivateFlipKeysPackageCid)
		if err := msg.Decode(packageCid); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if pm.isProcessed(packageCid) {
			return nil
		}
		p.markPayload(packageCid)
		pm.flipKeyPool.AddPrivateKeysPackageCid(packageCid)
	}

	return nil
}

func (pm *ProtocolManager) isProcessed(payload interface{}) bool {
	return pm.peers.HasPayload(payload)
}

func (pm *ProtocolManager) provideBlocks(p *peer, batchId uint32, from uint64, to uint64) {
	var result []*block
	for i := from; i <= to; i++ {
		b := pm.bcn.GetBlockHeaderByHeight(i)
		if b != nil {
			result = append(result, &block{
				Header:       b,
				Cert:         pm.bcn.GetCertificate(b.Hash()),
				IdentityDiff: pm.bcn.GetIdentityDiff(b.Height()),
			})
			p.Log().Trace("Publish block", "height", b.Height())
		} else {
			p.Log().Warn("Do not have requested block", "height", i)
			break
		}
	}
	p.sendMsg(BlocksRange, &blockRange{
		BatchId: batchId,
		Blocks:  result,
	}, false)
}

func (pm *ProtocolManager) provideForkBlocks(p *peer, batchId uint32, blocks []common.Hash) {
	var result []*block

	bundles := pm.bcn.ReadBlockForForkedPeer(blocks)
	for _, b := range bundles {
		result = append(result, &block{
			Header: b.Block.Header,
			Cert:   b.Cert,
		})
	}
	p.Log().Info("Peer is in fork. Providing own blocks", "cnt", len(bundles))
	p.sendMsg(BlocksRange, &blockRange{
		BatchId: batchId,
		Blocks:  result,
	}, false)
}

func (pm *ProtocolManager) HandleNewPeer(p *p2p.Peer, rw p2p.MsgReadWriter) error {
	id := formatPeerId(p.ID())
	if pm.bannedPeers.Contains(id) {
		return errors.New("Peer is banned")
	}
	peer := pm.makePeer(p, rw, pm.config.MaxDelay)
	if err := peer.Handshake(pm.bcn.Network(), pm.bcn.Head.Height(), pm.bcn.Genesis(), pm.appVersion); err != nil {
		current := semver.New(pm.appVersion)
		if other, errS := semver.NewVersion(peer.appVersion); errS != nil || other.Major >= current.Major || other.Minor >= current.Minor {
			p.Log().Info("Idena handshake failed", "err", err)
		}

		return err
	}
	pm.registerPeer(peer)

	go pm.syncTxPool(peer)
	go pm.syncFlipKeyPool(peer)
	pm.sendManifest(peer)
	defer pm.unregister(peer)
	p.Log().Info("Peer successfully connected", "peerId", p.ID(), "ip", peer.RemoteAddr().String(), "app", peer.appVersion, "proto", peer.protocol)
	return pm.runListening(peer)
}

func (pm *ProtocolManager) runListening(peer *peer) error {
	for {
		if err := pm.handle(peer); err != nil {
			peer.Log().Debug("Idena message handling failed", "err", err)
			return err
		}
	}
}
func (pm *ProtocolManager) registerPeer(peer *peer) {
	pm.peers.Register(peer)
	go peer.broadcast()
}
func (pm *ProtocolManager) unregister(peer *peer) {
	pm.peers.Unregister(peer.id)
	peer.Log().Info("Peer disconnected")
	close(peer.term)
}

func (pm *ProtocolManager) GetKnownHeights() map[string]uint64 {
	result := make(map[string]uint64)
	peers := pm.peers.Peers()
	if len(peers) == 0 {
		return nil
	}
	for _, peer := range peers {
		result[peer.id] = peer.knownHeight
	}
	return result
}

func (pm *ProtocolManager) GetKnownManifests() map[string]*snapshot.Manifest {
	result := make(map[string]*snapshot.Manifest)
	peers := pm.peers.Peers()
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

func (pm *ProtocolManager) GetBlocksRange(peerId string, from uint64, to uint64) (*batch, error) {
	peer := pm.peers.Peer(peerId)
	if peer == nil {
		return nil, errors.New("peer is not found")
	}

	b := &batch{
		from:    from,
		to:      to,
		p:       peer,
		headers: make(chan *block, to-from+1),
	}
	pm.batchedLock.Lock()
	peerBatches, ok := pm.incomeBatches.Load(peerId)
	if !ok {
		peerBatches = &sync.Map{}
		pm.incomeBatches.Store(peerId, peerBatches)
	}
	id := atomic.AddUint32(&batchId, 1)
	peerBatches.(*sync.Map).Store(id, b)
	pm.batchedLock.Unlock()
	peer.sendMsg(GetBlocksRange, &getBlocksRangeRequest{
		BatchId: batchId,
		From:    from,
		To:      to,
	}, false)
	return b, nil
}

func (pm *ProtocolManager) GetForkBlockRange(peerId string, ownBlocks []common.Hash) (*batch, error) {
	peer := pm.peers.Peer(peerId)
	if peer == nil {
		return nil, errors.New("peer is not found")
	}
	b := &batch{
		p:       peer,
		headers: make(chan *block, 100),
	}
	pm.batchedLock.Lock()
	peerBatches, ok := pm.incomeBatches.Load(peerId)
	if !ok {
		peerBatches = &sync.Map{}
		pm.incomeBatches.Store(peerId, peerBatches)
	}
	id := atomic.AddUint32(&batchId, 1)
	peerBatches.(*sync.Map).Store(id, b)
	pm.batchedLock.Unlock()
	peer.sendMsg(GetForkBlockRange, &getForkBlocksRangeRequest{
		BatchId: batchId,
		Blocks:  ownBlocks,
	}, false)
	return b, nil
}

func (pm *ProtocolManager) ProposeProof(round uint64, hash common.Hash, proof []byte, pubKey []byte) {
	payload := &proposeProof{
		Round:  round,
		Hash:   hash,
		PubKey: pubKey,
		Proof:  proof,
	}
	pm.proposeProof(payload)
}

func (pm *ProtocolManager) proposeProof(payload *proposeProof) {
	pm.peers.SendWithFilter(ProposeProof, payload, false)
}

func (pm *ProtocolManager) ProposeBlock(block *types.Block) {
	pm.peers.SendWithFilter(ProposeBlock, block, false)
}
func (pm *ProtocolManager) SendVote(vote *types.Vote) {
	pm.peers.SendWithFilter(Vote, vote, false)
}

func errResp(code int, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

func (pm *ProtocolManager) broadcastLoop() {
	for {
		select {
		case tx := <-pm.txChan:
			pm.broadcastTx(tx.Tx, tx.Own)
		case key := <-pm.flipKeyChan:
			pm.broadcastFlipKey(key.Key, key.Own)
		case key := <-pm.flipKeysPackageChan:
			pm.broadcastFlipKeysPackage(key.Key, key.Own)
		}
	}
}
func (pm *ProtocolManager) broadcastTx(tx *types.Transaction, own bool) {
	pm.peers.SendWithFilter(NewTx, tx, own)
}

func (pm *ProtocolManager) broadcastFlipCid(cid []byte) {
	pm.peers.SendWithFilter(PushFlipCid, &flipCid{cid}, false)
}

func (pm *ProtocolManager) broadcastFlipKey(flipKey *types.PublicFlipKey, own bool) {
	pm.peers.SendWithFilter(FlipKey, flipKey, own)
}

func (pm *ProtocolManager) broadcastFlipKeysPackage(flipKeysPackage *types.PrivateFlipKeysPackage, own bool) {
	pm.peers.SendWithFilter(FlipKeysPackage, flipKeysPackage, own)
}

func (pm *ProtocolManager) RequestBlockByHash(hash common.Hash) {
	pm.peers.Send(GetBlockByHash, &getBlockBodyRequest{
		Hash: hash,
	})
}

func (pm *ProtocolManager) syncTxPool(p *peer) {
	pending := pm.txpool.GetPendingTransaction()
	for _, tx := range pending {
		p.sendMsg(NewTx, tx, false)
		p.markPayload(tx)
	}
}

func (pm *ProtocolManager) sendManifest(p *peer) {
	manifest := pm.bcn.ReadSnapshotManifest()
	if manifest == nil {
		return
	}
	p.sendMsg(SnapshotManifest, manifest, true)
}

func (pm *ProtocolManager) syncFlipKeyPool(p *peer) {
	keys := pm.flipKeyPool.GetFlipKeys()
	for _, key := range keys {
		p.sendMsg(FlipKey, key, false)
	}

	keysPackages := pm.flipKeyPool.GetFlipPackagesCids()
	for _, flipPackageCid := range keysPackages {
		p.sendMsg(FlipKeysPackageCid, flipPackageCid, false)
	}
}
func (pm *ProtocolManager) PotentialForwardPeers(round uint64) []string {
	var result []string
	for _, p := range pm.peers.Peers() {
		if p.potentialHeight >= round {
			result = append(result, p.id)
		}
	}
	return result
}

func (pm *ProtocolManager) HasPeers() bool {
	return len(pm.peers.peers) > 0
}
func (pm *ProtocolManager) PeersCount() int {
	return len(pm.peers.peers)
}
func (pm *ProtocolManager) Peers() []*peer {
	return pm.peers.Peers()
}

func (pm *ProtocolManager) PeerHeights() []uint64 {
	result := make([]uint64, 0)
	peers := pm.peers.Peers()
	for _, peer := range peers {
		result = append(result, peer.knownHeight)
	}
	return result
}

func (pm *ProtocolManager) BanPeer(peerId string, reason error) {
	pm.bannedPeers.Add(peerId)
	if pm.bannedPeers.Cardinality() > MaxBannedPeers {
		pm.bannedPeers.Pop()
	}

	peer := pm.peers.Peer(peerId)
	if peer != nil {
		if reason != nil {
			peer.Log().Info("peer has been banned", "reason", reason)
		}
		peer.Disconnect(p2p.DiscBannedPeer)
	}

}
