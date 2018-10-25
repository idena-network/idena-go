package protocol

import (
	"github.com/deckarep/golang-set"
	"idena-go/blockchain"
	"idena-go/p2p"
)

const (
	MaxKnownBlocks = 300
	MaxKwownTxs    = 2000
	MaxKnownProofs = 1000
)

type peer struct {
	*p2p.Peer
	id              string
	knownTxs        mapset.Set             // Set of transaction hashes known to be known by this peer
	knownBlocks     mapset.Set             // Set of block hashes known to be known by this peer
	knownVotes      mapset.Set             // Set of hashes of votes known to be known by this peer
	knownProofs     mapset.Set             // Set of hashes of proposer proofs known to be known by this peer
	queuedTxs       chan struct{}          // Queue of transactions to broadcast to the peer
	queuedProofs    chan *proposeProof     // Queue of proposer proofs to broadcast to the peer
	queuedBlocks    chan *blockchain.Block // Queue of blocks to broadcast to the peer
	queuedProposals chan *blockchain.Block
	term            chan struct{}
}

func (p *peer) SendBlockAsync(block *blockchain.Block) {

	p.queuedBlocks <- block

}

func (p *peer) SendHeader(header *blockchain.Header, code uint64) {
	p2p.Send(p.Connection(), code, header)
}

func (p *peer) RequestLastBlock() {
	p2p.Send(p.Connection(), GetHead, struct{}{})
}
func (p *peer) RequestBlock(height uint64) {
	p2p.Send(p.Connection(), GetBlockByHeight, &getBlockByHeightRequest{
		height: height,
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
		case <-p.term:
			return
		}
	}
}
func (p *peer) SendProofAsync(proof *proposeProof) {
	p.queuedProofs <- proof
	p.markProof(proof)
}
func (p *peer) ProposeBlockAsync(block *blockchain.Block) {
	p.queuedProposals <- block
	p.markBlock(block)
}

func (p *peer) markBlock(block *blockchain.Block) {
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
