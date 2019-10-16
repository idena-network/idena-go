package protocol

import (
	"fmt"
	"github.com/coreos/go-semver/semver"
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/eventbus"
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
	Handshake        = 0x01
	GetHead          = 0x02
	Head             = 0x03
	ProposeBlock     = 0x04
	ProposeProof     = 0x05
	Vote             = 0x06
	NewTx            = 0x07
	GetBlockByHash   = 0x08
	GetBlocksRange   = 0x09
	BlocksRange      = 0x0A
	NewFlip          = 0x0B
	FlipKey          = 0x0C
	SnapshotManifest = 0x0D
)
const (
	DecodeErr              = 1
	MaxTimestampLagSeconds = 15
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

	txpool        *mempool.TxPool
	flipKeyPool   *mempool.KeysPool
	flipper       *flip.Flipper
	txChan        chan *types.Transaction
	flipKeyChan   chan *types.FlipKey
	incomeBatches map[string]map[uint32]*batch
	batchedLock   sync.Mutex
	bus           eventbus.Bus
	config        *p2p.Config
	wrongTime     bool
	appVersion    string
}

type getBlockBodyRequest struct {
	Hash common.Hash
}

type getBlocksRangeRequest struct {
	BatchId uint32
	From    uint64
	To      uint64
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
		bcn:           chain,
		peers:         newPeerSet(),
		heads:         make(chan *peerHead, 10),
		incomeBlocks:  make(chan *types.Block, 1000),
		incomeBatches: make(map[string]map[uint32]*batch),
		proposals:     proposals,
		votes:         votes,
		txpool:        txpool,
		txChan:        make(chan *types.Transaction, 100),
		flipKeyChan:   make(chan *types.FlipKey, 200),
		flipper:       fp,
		bus:           bus,
		flipKeyPool:   flipKeyPool,
		config:        config,
		appVersion:    appVersion,
	}
}

func (pm *ProtocolManager) Start() {
	_ = pm.bus.Subscribe(events.NewTxEventID, func(e eventbus.Event) {
		newTxEvent := e.(*events.NewTxEvent)
		pm.txChan <- newTxEvent.Tx
	})
	_ = pm.bus.Subscribe(events.NewFlipKeyID, func(e eventbus.Event) {
		newFlipKeyEvent := e.(*events.NewFlipKeyEvent)
		pm.flipKeyChan <- newFlipKeyEvent.Key
	})
	_ = pm.bus.Subscribe(events.NewFlipEventID, func(e eventbus.Event) {
		newFlipEvent := e.(*events.NewFlipEvent)
		pm.broadcastFlip(newFlipEvent.Flip)
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
		if peerBatches, ok := pm.incomeBatches[p.id]; ok {
			if batch, ok := peerBatches[response.BatchId]; ok {
				for _, b := range response.Blocks {
					batch.headers <- b
					p.setHeight(b.Header.Height())
				}
				pm.batchedLock.Lock()
				delete(peerBatches, response.BatchId)
				pm.batchedLock.Unlock()
			}
		}

	case ProposeProof:
		query := new(proposeProof)
		if err := msg.Decode(query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if err := p.markPayload(query); err != nil {
			return nil
		}
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
		if err := p.markPayload(block); err != nil {
			return nil
		}
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
		if err := p.markPayload(vote); err != nil {
			return nil
		}
		p.setPotentialHeight(vote.Header.Round - 1)
		if pm.votes.AddVote(vote) {
			pm.SendVote(vote)
		}
	case NewTx:
		tx := new(types.Transaction)
		if err := msg.Decode(tx); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if err := p.markPayload(tx); err != nil {
			return nil
		}
		pm.txpool.Add(tx)
	case GetBlockByHash:
		var query getBlockBodyRequest
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		block := pm.bcn.GetBlock(query.Hash)
		if block != nil {
			p.sendMsg(ProposeBlock, block)
		}
	case GetBlocksRange:
		var query getBlocksRangeRequest
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		pm.provideBlocks(p, query.BatchId, query.From, query.To)
	case NewFlip:
		f := new(types.Flip)
		if err := msg.Decode(f); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if err := p.markPayload(f); err != nil {
			return nil
		}
		pm.flipper.AddNewFlip(f, false)
	case FlipKey:
		flipKey := new(types.FlipKey)
		if err := msg.Decode(flipKey); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		pm.flipKeyPool.Add(flipKey)
	case SnapshotManifest:
		manifest := new(snapshot.Manifest)
		if err := msg.Decode(manifest); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.manifest = manifest
	}

	return nil
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
	})
}

func (pm *ProtocolManager) HandleNewPeer(p *p2p.Peer, rw p2p.MsgReadWriter) error {
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
	p.Log().Info("Peer successfully connected", "peerId", p.ID(), "app", peer.appVersion, "proto", peer.protocol)
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

func (pm *ProtocolManager) GetBlocksRange(peerId string, from uint64, to uint64) (error, *batch) {
	peer := pm.peers.Peer(peerId)
	if peer == nil {
		return errors.New("peer is not found"), nil
	}

	b := &batch{
		from:    from,
		to:      to,
		p:       peer,
		headers: make(chan *block, to-from+1),
	}
	peerBatches, ok := pm.incomeBatches[peerId]

	if !ok {
		peerBatches = make(map[uint32]*batch)
		pm.batchedLock.Lock()
		pm.incomeBatches[peerId] = peerBatches
		pm.batchedLock.Unlock()
	}
	id := atomic.AddUint32(&batchId, 1)
	peerBatches[id] = b
	peer.sendMsg(GetBlocksRange, &getBlocksRangeRequest{
		BatchId: batchId,
		From:    from,
		To:      to,
	})
	return nil, b
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
	pm.peers.SendWithFilter(ProposeProof, payload)
}

func (pm *ProtocolManager) ProposeBlock(block *types.Block) {
	pm.peers.SendWithFilter(ProposeBlock, block)
}
func (pm *ProtocolManager) SendVote(vote *types.Vote) {
	pm.peers.SendWithFilter(Vote, vote)
}

func errResp(code int, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

func (pm *ProtocolManager) broadcastLoop() {
	for {
		select {
		case tx := <-pm.txChan:
			pm.broadcastTx(tx)
		case key := <-pm.flipKeyChan:
			pm.broadcastFlipKey(key)
		}
	}
}
func (pm *ProtocolManager) broadcastTx(tx *types.Transaction) {
	pm.peers.SendWithFilter(NewTx, tx)
}

func (pm *ProtocolManager) broadcastFlip(flip *types.Flip) {
	pm.peers.SendWithFilter(NewFlip, flip)
}

func (pm *ProtocolManager) broadcastFlipKey(flipKey *types.FlipKey) {
	pm.peers.SendWithFilter(FlipKey, flipKey)
}

func (pm *ProtocolManager) RequestBlockByHash(hash common.Hash) {
	pm.peers.Send(GetBlockByHash, &getBlockBodyRequest{
		Hash: hash,
	})
}

func (pm *ProtocolManager) syncTxPool(p *peer) {
	pending := pm.txpool.GetPendingTransaction()
	for _, tx := range pending {
		p.sendMsg(NewTx, tx)
		p.markPayload(tx)
	}
}

func (pm *ProtocolManager) sendManifest(p *peer) {
	manifest := pm.bcn.ReadSnapshotManifest()
	if manifest == nil {
		return
	}
	p.sendMsg(SnapshotManifest, manifest)
}

func (pm *ProtocolManager) syncFlipKeyPool(p *peer) {
	keys := pm.flipKeyPool.GetFlipKeys()
	for _, key := range keys {
		p.sendMsg(FlipKey, key)
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
