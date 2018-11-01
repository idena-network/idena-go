package protocol

import (
	"fmt"
	"github.com/deckarep/golang-set"
	"idena-go/blockchain/types"
	"idena-go/common"
	"idena-go/p2p"
)

const (
	MaxKnownBlocks = 300
	MaxKwownTxs    = 2000
	MaxKnownProofs = 1000
	MaxKnownVotes  = 100000
)

type peer struct {
	*p2p.Peer
	id              string
	knownTxs        mapset.Set // Set of transaction hashes known to be known by this peer
	knownBlocks     mapset.Set // Set of block hashes known to be known by this peer
	knownVotes      mapset.Set // Set of hashes of votes known to be known by this peer
	knownProofs     mapset.Set // Set of hashes of proposer proofs known to be known by this peer
	queuedTxs       chan *types.Transaction
	queuedProofs    chan *proposeProof // Queue of proposer proofs to broadcast to the peer
	queuedBlocks    chan *types.Block  // Queue of blocks to broadcast to the peer
	queuedProposals chan *types.Block
	queuedVotes     chan *types.Vote
	term            chan struct{}
}

func (pm *ProtocolManager) makePeer(p *p2p.Peer) *peer {
	return &peer{
		Peer:         p,
		id:           fmt.Sprintf("%x", p.ID().Bytes()[:8]),
		knownBlocks:  mapset.NewSet(),
		knownTxs:     mapset.NewSet(),
		knownVotes:   mapset.NewSet(),
		queuedTxs:    make(chan *types.Transaction),
		queuedBlocks: make(chan *types.Block),
		knownProofs:  mapset.NewSet(),
		queuedProofs: make(chan *proposeProof),
		term:         make(chan struct{}),
	}
}

func (p *peer) SendBlockAsync(block *types.Block) {
	p.queuedBlocks <- block
}

func (p *peer) SendHeader(header *types.Header, code uint64) {
	p2p.Send(p.Connection(), code, header)
}

func (p *peer) RequestLastBlock() {
	p2p.Send(p.Connection(), GetHead, struct{}{})
}
func (p *peer) RequestBlock(height uint64) {
	p2p.Send(p.Connection(), GetBlockByHeight, &getBlockByHeightRequest{
		Height: height,
	})
}

func (p *peer) RequestBlockByHash(hash common.Hash) {
	p2p.Send(p.Connection(), GetBlockByHash, &getBlockBodyRequest{
		Hash: hash,
	})
}

func (p *peer) broadcast() {
	for {
		select {
		case block := <-p.queuedBlocks:
			if err := p2p.Send(p.Connection(), BlockBody, block); err != nil {
				return
			}

		case proof := <-p.queuedProofs:
			if err := p2p.Send(p.Connection(), ProposeProof, proof); err != nil {
				return
			}

		case block := <-p.queuedProposals:
			if err := p2p.Send(p.Connection(), ProposeBlock, block); err != nil {
				return
			}
		case vote := <-p.queuedVotes:
			if err := p2p.Send(p.Connection(), Vote, vote); err != nil {
				return
			}
		case tx := <-p.queuedTxs:
			if err := p2p.Send(p.Connection(), NewTx, tx); err != nil {
				return
			}
		case <-p.term:
			return
		}
	}
}
func (p *peer) SendProofAsync(proof *proposeProof) {
	p.queuedProofs <- proof
	p.markProof(proof)
}
func (p *peer) ProposeBlockAsync(block *types.Block) {
	p.queuedProposals <- block
	p.markBlock(block)
}

func (p *peer) SendTxAsync(transaction *types.Transaction) {
	p.queuedTxs <- transaction
	p.markTx(transaction)
}

func (p *peer) markBlock(block *types.Block) {
	if p.knownBlocks.Cardinality() > MaxKnownBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(block)
}

func (p *peer) markProof(proof *proposeProof) {
	if p.knownProofs.Cardinality() > MaxKnownProofs {
		p.knownProofs.Pop()
	}
	p.knownProofs.Add(proof)
}
func (p *peer) markVote(vote *types.Vote) {
	if p.knownVotes.Cardinality() > MaxKnownVotes {
		p.knownVotes.Pop()
	}
	p.knownVotes.Add(vote)
}
func (p *peer) SendVoteAsync(vote *types.Vote) {
	p.queuedVotes <- vote
	p.markVote(vote)
}
func (p *peer) markTx(tx *types.Transaction) {
	if p.knownTxs.Cardinality() > MaxKwownTxs {
		p.knownTxs.Pop()
	}
	p.knownTxs.Add(tx)
}

