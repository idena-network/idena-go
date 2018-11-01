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
	BlockBody        = 21
	GetHead          = 22
	Head             = 23
	GetBlockByHeight = 24
	ProposeBlock     = 25
	ProposeProof     = 26
	Vote             = 27
	NewTx            = 28
	GetBlockByHash   = 29
)
const (
	DecodeErr = 1
)

type ProtocolManager struct {
	blockchain *blockchain.Blockchain

	peers *peerSet

	heads chan *peerHead

	incomeBlocks chan *types.Block
	proposals    *pengings.Proposals
	votes        *pengings.Votes
	txpool       *mempool.TxPool
	txChan       chan *types.Transaction
}

type getBlockBodyRequest struct {
	Hash common.Hash
}

type getBlockByHeightRequest struct {
	Height uint64
}

type proposeProof struct {
	Hash   common.Hash
	Proof  [] byte
	PubKey [] byte
	Round  uint64
}

type peerHead struct {
	height uint64
	peer   *peer
}

func NetProtocolManager(chain *blockchain.Blockchain, proposals *pengings.Proposals, votes *pengings.Votes, txpool *mempool.TxPool) *ProtocolManager {

	txChan := make(chan *types.Transaction, 100)
	txpool.Subscribe(txChan)
	return &ProtocolManager{
		blockchain:   chain,
		peers:        newPeerSet(),
		heads:        make(chan *peerHead, 10),
		incomeBlocks: make(chan *types.Block, 100),
		proposals:    proposals,
		votes:        votes,
		txpool:       txpool,
		txChan:       txChan,
	}
}

func (pm *ProtocolManager) Start() {
	go pm.broadcastLoop()
}

func (pm *ProtocolManager) handle(p *peer) error {
	msg, err := p.Connection().ReadMsg()
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
		pm.incomeBlocks <- &response
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
	}
	return nil
}

func (pm *ProtocolManager) HandleNewPeer(p *p2p.Peer, rw p2p.MsgReadWriter) error {
	peer := pm.makePeer(p)

	pm.registerPeer(peer)
	defer pm.unregister(peer)
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
func (pm *ProtocolManager) ProposeProof(round uint64, hash common.Hash, proof []byte, pubKey []byte) {
	msg := &proposeProof{
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
