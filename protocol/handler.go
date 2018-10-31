package protocol

import (
	"fmt"
	"github.com/pkg/errors"
	"idena-go/blockchain"
	"idena-go/common"
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
)
const (
	DecodeErr = 1
)

type ProtocolManager struct {
	blockchain *blockchain.Blockchain

	peers *peerSet

	heads chan *peerHead

	incomeBlocks chan *blockchain.Block
	proposals    *pengings.Proposals
	votes        *pengings.Votes
}

type getBlockBodyRequest struct {
	hash common.Hash
}

type getBlockByHeightRequest struct {
	height uint64
}

type proposeProof struct {
	hash   common.Hash
	proof  [] byte
	pubKey [] byte
	round  uint64
}

type peerHead struct {
	height uint64
	peer   *peer
}

func NetProtocolManager(chain *blockchain.Blockchain, proposals *pengings.Proposals, votes *pengings.Votes) *ProtocolManager {
	return &ProtocolManager{
		blockchain:   chain,
		peers:        newPeerSet(),
		heads:        make(chan *peerHead),
		incomeBlocks: make(chan *blockchain.Block),
		proposals:    proposals,
		votes:        votes,
	}
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
		var response blockchain.Header
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
		block := pm.blockchain.GetBlockByHeight(query.height)
		p.SendBlockAsync(block)
	case BlockBody:
		var response blockchain.Block
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
		if pm.proposals.AddProposeProof(query.proof, query.hash, query.pubKey, query.round) {
			pm.ProposeProof(query.round, query.hash, query.proof, query.pubKey)
		}
	case ProposeBlock:
		var block blockchain.Block
		if err := msg.Decode(&block); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.markBlock(&block)
		if pm.proposals.AddProposedBlock(&block) {
			pm.ProposeBlock(&block)
		}
	case Vote:
		var vote blockchain.Vote
		if err := msg.Decode(&vote); err != nil {
			return errResp(DecodeErr, "%v: %v", msg, err)
		}
		p.markVote(&vote)
		if pm.votes.AddVote(&vote) {
			pm.SendVote(&vote)
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
func (pm *ProtocolManager) GetBlockFromPeer(peerId string, height uint64) *blockchain.Block {
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
		hash:   hash,
		pubKey: pubKey,
		proof:  proof,
	}
	for _, peer := range pm.peers.PeersWithoutProof(hash) {
		peer.SendProofAsync(msg)
	}
}
func (pm *ProtocolManager) ProposeBlock(block *blockchain.Block) {
	for _, peer := range pm.peers.PeersWithoutBlock(block.Hash()) {
		peer.ProposeBlockAsync(block)
	}
}
func (pm *ProtocolManager) SendVote(vote *blockchain.Vote) {
	for _, peer := range pm.peers.PeersWithoutVote(vote.Hash()) {
		peer.SendVoteAsync(vote)
	}
}

func errResp(code int, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}
