package protocol

import (
	"fmt"
	"github.com/pkg/errors"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/flip"
	"idena-go/core/mempool"
	"idena-go/p2p"
	"idena-go/pengings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	Handshake      = 0x01
	GetHead        = 0x02
	Head           = 0x03
	ProposeBlock   = 0x04
	ProposeProof   = 0x05
	Vote           = 0x06
	NewTx          = 0x07
	GetBlockByHash = 0x08
	GetBlocksRange = 0x09
	BlocksRange    = 0x0A
	NewFlip        = 0x0B
)
const (
	DecodeErr = 1
	FlipErr   = 2
)

var (
	batchId = uint32(1)
)

type ProtocolManager struct {
	bcn *blockchain.Blockchain

	peers *peerSet

	heads chan *peerHead

	incomeBlocks  chan *types.Block
	proposals     *pengings.Proposals
	votes         *pengings.Votes
	txpool        *mempool.TxPool
	flipper       *flip.Flipper
	txChan        chan *types.Transaction
	incomeBatches map[string]map[uint32]*batch
	batchedLock   sync.Mutex
}

type getBlockBodyRequest struct {
	Hash common.Hash
}

type getBlockByHeightRequest struct {
	Height uint64
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
}

func NetProtocolManager(chain *blockchain.Blockchain, proposals *pengings.Proposals, votes *pengings.Votes, txpool *mempool.TxPool, fp *flip.Flipper) *ProtocolManager {

	txChan := make(chan *types.Transaction, 100)
	txpool.Subscribe(txChan)
	return &ProtocolManager{
		bcn:           chain,
		peers:         newPeerSet(),
		heads:         make(chan *peerHead, 10),
		incomeBlocks:  make(chan *types.Block, 1000),
		incomeBatches: make(map[string]map[uint32]*batch),
		proposals:     proposals,
		votes:         votes,
		txpool:        txpool,
		txChan:        txChan,
		flipper:       fp,
	}
}

func (pm *ProtocolManager) Start() {
	go pm.broadcastLoop()
}

func (pm *ProtocolManager) handle(p *peer) error {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	defer msg.Discard()
	switch msg.Code {
	case GetHead:
		header := pm.bcn.GetHead()
		p.SendHeader(header, Head)
	case Head:
		var response types.Header
		if err := msg.Decode(&response); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		pm.heads <- &peerHead{
			peer:   p,
			height: response.Height(),
		}
	case BlocksRange:
		var response blockRange
		if err := msg.Decode(&response); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.Log().Trace("Income blocks range", "batchId", response.BatchId)
		if peerBatches, ok := pm.incomeBatches[p.id]; ok {
			if batch, ok := peerBatches[response.BatchId]; ok {
				for _, b := range response.Headers {
					batch.headers <- b
				}
				pm.batchedLock.Lock()
				delete(peerBatches, response.BatchId)
				pm.batchedLock.Unlock()
			}
		}

	case ProposeProof:
		var query proposeProof
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.markProof(&query)
		// if peer proposes this msg it should be on `query.Round-1` height
		p.setHeight(query.Round - 1)
		if pm.proposals.AddProposeProof(query.Proof, query.Hash, query.PubKey, query.Round) {
			pm.ProposeProof(query.Round, query.Hash, query.Proof, query.PubKey)
		}
	case ProposeBlock:
		var block types.Block
		if err := msg.Decode(&block); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.markHeader(block.Header)
		// if peer proposes this msg it should be on `query.Round-1` height
		p.setHeight(block.Height() - 1)
		if pm.proposals.AddProposedBlock(&block) {
			pm.ProposeBlock(&block)
		}
	case Vote:
		var vote types.Vote
		if err := msg.Decode(&vote); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.markVote(&vote)
		if pm.votes.AddVote(&vote) {
			pm.SendVote(&vote)
		}
	case NewTx:
		var tx types.Transaction
		if err := msg.Decode(&tx); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.markTx(&tx)
		pm.txpool.Add(&tx)
	case GetBlockByHash:
		var query getBlockBodyRequest
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		block := pm.bcn.GetBlock(query.Hash)
		if block != nil {
			p.ProposeBlockAsync(block)
		}
	case GetBlocksRange:
		var query getBlocksRangeRequest
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		pm.provideBlocks(p, query.BatchId, query.From, query.To)
	case NewFlip:
		var flip types.Flip
		if err := msg.Decode(&flip); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.markFlip(&flip)
		if err := pm.flipper.AddNewFlip(flip); err != nil {
			return errResp(FlipErr, "%v: %v", flip.Tx.Hash(), err)
		}
		pm.BroadcastFlip(&flip)
	}
	return nil
}

func (pm *ProtocolManager) provideBlocks(p *peer, batchId uint32, from uint64, to uint64) {
	var result []*types.Header
	for i := from; i <= to; i++ {
		block := pm.bcn.GetBlockHeaderByHeight(i)
		if block != nil {
			result = append(result, block)
			p.Log().Trace("Publish block", "height", block.Height())
		} else {
			p.Log().Warn("Do not have requested block", "height", block.Height())
			return
		}
	}
	p.SendBlockRangeAsync(&blockRange{
		BatchId: batchId,
		Headers: result,
	})
}

func (pm *ProtocolManager) HandleNewPeer(p *p2p.Peer, rw p2p.MsgReadWriter) error {
	peer := pm.makePeer(p, rw)
	if err := peer.Handshake(pm.bcn.Network(), pm.bcn.Head.Height(), pm.bcn.Genesis()); err != nil {
		p.Log().Info("Idena handshake failed", "err", err)
		return err
	}
	pm.syncTxPool(peer)
	pm.registerPeer(peer)
	defer pm.unregister(peer)
	p.Log().Info("Peer successfully connected", "peerId", p.ID())
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
	close(peer.term)
}
func (pm *ProtocolManager) GetBestHeight() (uint64, string, error) {
	peers := pm.peers.Peers()
	if len(peers) == 0 {
		return 0, "", nil
	}
	for _, peer := range peers {
		peer.RequestLastBlock()
	}
	timeout := time.After(time.Second * 10)
	var best *peerHead
waiting:
	for i := 0; i < len(peers); i++ {
		select {
		case head := <-pm.heads:
			if best == nil || best.height < head.height {
				best = head
			}
		case <-timeout:
			break waiting
		}
	}
	if best == nil {
		return 0, "", errors.New("timeout")
	}
	return best.height, best.peer.id, nil
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

func (pm *ProtocolManager) GetBlocksRange(peerId string, from uint64, to uint64) (error, *batch) {
	peer := pm.peers.Peer(peerId)
	if peer == nil {
		return errors.New("peer is not found"), nil
	}

	b := &batch{
		from:    from,
		to:      to,
		p:       peer,
		headers: make(chan *types.Header, to-from+1),
	}
	peerBatches, ok := pm.incomeBatches[peerId]

	if !ok {
		peerBatches = make(map[uint32]*batch)
		pm.batchedLock.Lock()
		pm.incomeBatches[peerId] = peerBatches
		pm.batchedLock.Unlock()
	}
	peerBatches[batchId] = b
	peer.RequestBlocksRange(batchId, from, to)
	atomic.AddUint32(&batchId, 1)
	return nil, b
}

func (pm *ProtocolManager) ProposeProof(round uint64, hash common.Hash, proof []byte, pubKey []byte) {
	msg := &proposeProof{
		Round:  round,
		Hash:   hash,
		PubKey: pubKey,
		Proof:  proof,
	}
	for _, peer := range pm.peers.PeersWithoutProof(hash) {
		peer.SendProofAsync(msg)
	}
}
func (pm *ProtocolManager) ProposeBlock(block *types.Block) {
	for _, peer := range pm.peers.PeersWithoutBlock(block.Hash()) {
		peer.ProposeBlockAsync(block)
	}
}
func (pm *ProtocolManager) SendVote(vote *types.Vote) {
	for _, peer := range pm.peers.PeersWithoutVote(vote.Hash()) {
		peer.SendVoteAsync(vote)
	}
}

func errResp(code int, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

func (pm *ProtocolManager) broadcastLoop() {
	for {
		select {
		case tx := <-pm.txChan:
			pm.BroadcastTx(tx)
		}
	}
}
func (pm *ProtocolManager) BroadcastTx(transaction *types.Transaction) {
	for _, peer := range pm.peers.PeersWithoutTx(transaction.Hash()) {
		peer.SendTxAsync(transaction)
	}
}

func (pm *ProtocolManager) BroadcastFlip(flip *types.Flip) {
	for _, peer := range pm.peers.PeersWithoutFlip(flip.Tx.Hash()) {
		peer.SendFlipAsync(flip)
	}
}

func (pm *ProtocolManager) RequestBlockByHash(hash common.Hash) {
	for _, peer := range pm.peers.Peers() {
		peer.RequestBlockByHash(hash)
	}
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

func (pm *ProtocolManager) syncTxPool(p *peer) {
	pending := pm.txpool.GetPendingTransaction()
	for _, tx := range pending {
		p.SendTxAsync(tx)
	}
}
