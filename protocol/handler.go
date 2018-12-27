package protocol

import (
	"fmt"
	"github.com/pkg/errors"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/core/mempool"
	"idena-go/p2p"
	"idena-go/pengings"
	"time"
)

const (
	Handshake        = 0x01
	BlockBody        = 0x02
	GetHead          = 0x03
	Head             = 0x04
	GetBlockByHeight = 0x05
	ProposeBlock     = 0x06
	ProposeProof     = 0x07
	Vote             = 0x08
	NewTx            = 0x09
	GetBlockByHash   = 0x0A
	GetBlocksRange   = 0x0B
)
const (
	DecodeErr = 1
)

type ProtocolManager struct {
	blockchain *blockchain.Blockchain

	peers *peerSet

	heads chan *peerHead

	incomeBlocks  chan *types.Block
	proposals     *pengings.Proposals
	votes         *pengings.Votes
	txpool        *mempool.TxPool
	txChan        chan *types.Transaction
	incomeBatches map[string][]*batch
}

type getBlockBodyRequest struct {
	Hash common.Hash
}

type getBlockByHeightRequest struct {
	Height uint64
}

type getBlocksRangeRequest struct {
	From uint64
	To   uint64
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

func NetProtocolManager(chain *blockchain.Blockchain, proposals *pengings.Proposals, votes *pengings.Votes, txpool *mempool.TxPool) *ProtocolManager {

	txChan := make(chan *types.Transaction, 100)
	txpool.Subscribe(txChan)
	return &ProtocolManager{
		blockchain:    chain,
		peers:         newPeerSet(),
		heads:         make(chan *peerHead, 10),
		incomeBlocks:  make(chan *types.Block, 1000),
		incomeBatches: make(map[string][]*batch),
		proposals:     proposals,
		votes:         votes,
		txpool:        txpool,
		txChan:        txChan,
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
		header := pm.blockchain.GetHead()
		p.SendHeader(header.Header, Head)
	case Head:
		var response types.Header
		if err := msg.Decode(&response); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		pm.heads <- &peerHead{
			peer:   p,
			height: response.Height(),
		}
	case GetBlockByHeight:
		var query getBlockByHeightRequest
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		block := pm.blockchain.GetBlockByHeight(query.Height)
		if block != nil {
			p.SendBlockAsync(block)
		}
	case BlockBody:
		var response types.Block
		if err := msg.Decode(&response); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		if peerBatches, ok := pm.incomeBatches[p.id]; ok {
			for _, b := range peerBatches {
				if response.Height() >= b.from && response.Height() <= b.to {
					b.blocks <- &response
					//TODO: delete full batch
					break
				}
			}
		}

	case ProposeProof:
		var query proposeProof
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.markProof(&query)
		if pm.proposals.AddProposeProof(query.Proof, query.Hash, query.PubKey, query.Round) {
			pm.ProposeProof(query.Round, query.Hash, query.Proof, query.PubKey)
		}
	case ProposeBlock:
		var block types.Block
		if err := msg.Decode(&block); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.markBlock(&block)
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
		block := pm.blockchain.GetBlock(query.Hash)
		if block != nil {
			p.ProposeBlockAsync(block)
		}
	case GetBlocksRange:
		var query getBlocksRangeRequest
		if err := msg.Decode(&query); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		go pm.provideBlocks(p, query.From, query.To)
	}
	return nil
}

func (pm *ProtocolManager) provideBlocks(p *peer, from uint64, to uint64) {
	for i := from; i <= to; i++ {
		block := pm.blockchain.GetBlockByHeight(i)
		if block != nil {
			p.SendBlockAsync(block)
		}
	}
}

func (pm *ProtocolManager) HandleNewPeer(p *p2p.Peer, rw p2p.MsgReadWriter) error {
	peer := pm.makePeer(p, rw)
	if err := peer.Handshake(pm.blockchain.Network(), pm.blockchain.Head.Height(), pm.blockchain.Genesis()); err != nil {
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

func (pm *ProtocolManager) GetBlockFromPeer(peerId string, height uint64) *types.Block {
	peer := pm.peers.Peer(peerId)
	if peer == nil {
		return nil
	}
	go peer.RequestBlock(height)
	timeout := time.After(time.Second * 10)
	for {
		select {
		case block := <-pm.incomeBlocks:
			if block.Height() == height {
				return block
			}
		case <-timeout:
			return nil
		}
	}
}

func (pm *ProtocolManager) GetBlocksRange(peerId string, from uint64, to uint64) (error, *batch) {
	peer := pm.peers.Peer(peerId)
	if peer == nil {
		return errors.New("peer is not found"), nil
	}

	batch := &batch{
		from:   from,
		to:     to,
		p:      peer,
		blocks: make(chan *types.Block, to-from),
	}
	pm.incomeBatches[peerId] = append(pm.incomeBatches[peerId], batch)

	peer.RequestBlocksRange(from, to)
	return nil, batch
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
