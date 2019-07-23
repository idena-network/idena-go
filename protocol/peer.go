package protocol

import (
	"fmt"
	"github.com/deckarep/golang-set"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/p2p"
	"github.com/pkg/errors"
	"math/rand"
	"time"
)

const (
	MaxKnownBlocks   = 300
	MaxKnownFlips    = 100
	MaxKwownTxs      = 2000
	MaxKnownProofs   = 1000
	MaxKnownVotes    = 100000
	MaxKnownFlipKeys = 10000

	handshakeTimeout = 10 * time.Second
)

type peer struct {
	*p2p.Peer
	rw                p2p.MsgReadWriter
	id                string
	maxDelayMs        int
	knownHeight       uint64
	potentialHeight   uint64
	manifest          *snapshot.Manifest
	knownTxs          mapset.Set // Set of transaction hashes known to be known by this peer
	knownBlocks       mapset.Set // Set of block hashes known to be known by this peer
	knownVotes        mapset.Set // Set of hashes of votes known to be known by this peer
	knownProofs       mapset.Set // Set of hashes of proposer proofs known to be known by this peer
	knownFlips        mapset.Set // Set of hashes of flip transactions known by this peer
	knownFlipKeys     mapset.Set // Set of hashes of flip keys known to be known by this peer
	queuedTxs         chan *types.Transaction
	queuedProofs      chan *proposeProof // Queue of proposer proofs to broadcast to the peer
	queuedBlockRanges chan *blockRange   // Queue of blocks ranges to broadcast to the peer
	queuedProposals   chan *types.Block
	queuedVotes       chan *types.Vote
	queuedFlips       chan *types.Flip
	queuedFlipKeys    chan *types.FlipKey
	queuedRequests    chan *request
	term              chan struct{}
	finished          chan struct{}
}

type request struct {
	msgcode uint64
	data    interface{}
}

func (pm *ProtocolManager) makePeer(p *p2p.Peer, rw p2p.MsgReadWriter, maxDelayMs int) *peer {
	return &peer{
		rw:                rw,
		Peer:              p,
		id:                fmt.Sprintf("%x", p.ID().Bytes()[:8]),
		knownBlocks:       mapset.NewSet(),
		knownTxs:          mapset.NewSet(),
		knownVotes:        mapset.NewSet(),
		knownFlips:        mapset.NewSet(),
		queuedTxs:         make(chan *types.Transaction, 100),
		queuedBlockRanges: make(chan *blockRange, 10),
		queuedVotes:       make(chan *types.Vote, 100),
		queuedProposals:   make(chan *types.Block, 10),
		queuedRequests:    make(chan *request, 20),
		queuedFlips:       make(chan *types.Flip, 10),
		queuedFlipKeys:    make(chan *types.FlipKey, 100),
		knownFlipKeys:     mapset.NewSet(),
		knownProofs:       mapset.NewSet(),
		queuedProofs:      make(chan *proposeProof, 10),
		term:              make(chan struct{}),
		finished:          make(chan struct{}),
		maxDelayMs:        maxDelayMs,
	}
}

func (p *peer) SendBlockRangeAsync(blockRange *blockRange) {
	select {
	case p.queuedBlockRanges <- blockRange:
	case <-p.finished:
	}

}

func (p *peer) SendHeader(header *types.Header, code uint64) {
	select {
	case p.queuedRequests <- &request{msgcode: code, data: header}:
	case <-p.finished:
	}
}

func (p *peer) RequestLastBlock() {
	select {
	case p.queuedRequests <- &request{msgcode: GetHead, data: struct{}{}}:
	case <-p.finished:
	}
}

func (p *peer) RequestBlockByHash(hash common.Hash) {
	select {
	case p.queuedRequests <- &request{msgcode: GetBlockByHash, data: &getBlockBodyRequest{
		Hash: hash,
	}}:
	case <-p.finished:
	}
}

func (p *peer) RequestBlocksRange(batchId uint32, from uint64, to uint64) {

	select {
	case p.queuedRequests <- &request{msgcode: GetBlocksRange, data: &getBlocksRangeRequest{
		BatchId: batchId,
		From:    from,
		To:      to,
	}}:
	case <-p.finished:
	}

}

func (p *peer) broadcast() {
	defer p.Log().Info("Peer exited from broadcast loop")
	defer close(p.finished)
	for {
		if p.maxDelayMs > 0 {
			delay := time.Duration(rand.Int31n(int32(p.maxDelayMs)))
			time.Sleep(delay * time.Millisecond)
		}
		select {
		case blockRange := <-p.queuedBlockRanges:
			if err := p2p.Send(p.rw, BlocksRange, blockRange); err != nil {
				p.Log().Error(err.Error())
				return
			}
		case proof := <-p.queuedProofs:
			if err := p2p.Send(p.rw, ProposeProof, proof); err != nil {
				p.Log().Error(err.Error())
				return
			}
		case block := <-p.queuedProposals:
			if err := p2p.Send(p.rw, ProposeBlock, block); err != nil {
				p.Log().Error(err.Error())
				return
			}
		case vote := <-p.queuedVotes:
			if err := p2p.Send(p.rw, Vote, vote); err != nil {
				p.Log().Error(err.Error())
				return
			}
		case tx := <-p.queuedTxs:
			if err := p2p.Send(p.rw, NewTx, tx); err != nil {
				p.Log().Error(err.Error())
				return
			}
		case flip := <-p.queuedFlips:
			if err := p2p.Send(p.rw, NewFlip, flip); err != nil {
				p.Log().Error(err.Error())
				return
			}
		case flipKey := <-p.queuedFlipKeys:
			if err := p2p.Send(p.rw, FlipKey, flipKey); err != nil {
				p.Log().Error(err.Error())
				return
			}
		case request := <-p.queuedRequests:
			if err := p2p.Send(p.rw, request.msgcode, request.data); err != nil {
				p.Log().Error(err.Error())
				return
			}
		case <-p.term:
			return
		}
	}
}

func (p *peer) Handshake(network types.Network, height uint64, genesis common.Hash) error {
	errc := make(chan error, 2)
	var handShake handshakeData

	go func() {
		errc <- p2p.Send(p.rw, Handshake, &handshakeData{

			NetworkId:    network,
			Height:       height,
			GenesisBlock: genesis,
		})
	}()
	go func() {
		errc <- p.readStatus(&handShake, network, genesis)
	}()
	timeout := time.NewTimer(handshakeTimeout)
	defer timeout.Stop()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errc:
			if err != nil {
				return err
			}
		case <-timeout.C:
			return p2p.DiscReadTimeout
		}
	}
	p.knownHeight = handShake.Height
	return nil
}

func (p *peer) readStatus(handShake *handshakeData, network types.Network, genesis common.Hash) (err error) {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != Handshake {
		return errors.New(fmt.Sprintf("first msg has code %x (!= %x)", msg.Code, Handshake))
	}
	if err := msg.Decode(&handShake); err != nil {
		return errors.New(fmt.Sprintf("can't decode msg %v: %v", msg, err))
	}
	if handShake.GenesisBlock != genesis {
		return errors.New(fmt.Sprintf("Bad genesis block %x (!= %x)", handShake.GenesisBlock[:8], genesis[:8]))
	}
	if handShake.NetworkId != network {
		return errors.New(fmt.Sprintf("Network mismatch: %d (!= %d)", handShake.NetworkId, network))
	}

	return nil
}

func (p *peer) SendProofAsync(proof *proposeProof) {
	select {
	case p.queuedProofs <- proof:
		p.markProof(proof)
	case <-p.finished:
	}
}
func (p *peer) ProposeBlockAsync(block *types.Block) {
	select {
	case p.queuedProposals <- block:
		p.markHeader(block.Header)
	case <-p.finished:
	}

}

func (p *peer) SendTxAsync(transaction *types.Transaction) {
	select {
	case p.queuedTxs <- transaction:
		p.markTx(transaction)
	case <-p.finished:
	}

}

func (p *peer) SendVoteAsync(vote *types.Vote) {

	select {
	case p.queuedVotes <- vote:
		p.markVote(vote)
	case <-p.finished:
	}
}

func (p *peer) SendFlipAsync(flip *types.Flip) {

	select {
	case p.queuedFlips <- flip:
		p.markFlip(flip)
	case <-p.finished:
	}
}

func (p *peer) SendSnapshotManifest(manifest *snapshot.Manifest) {
	select {
	case p.queuedRequests <- &request{msgcode: SnapshotManifest, data: manifest}:
	case <-p.finished:
	}
}

func (p *peer) SendKeyPackageAsync(flipKey *types.FlipKey) {
	select {
	case p.queuedFlipKeys <- flipKey:
		p.markFlipKey(flipKey)
	case <-p.finished:
	}
}

func (p *peer) markHeader(block *types.Header) {
	if p.knownBlocks.Cardinality() > MaxKnownBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(block.Hash())
}

func (p *peer) markProof(proof *proposeProof) {
	if p.knownProofs.Cardinality() > MaxKnownProofs {
		p.knownProofs.Pop()
	}
	p.knownProofs.Add(proof.Hash)
}
func (p *peer) markVote(vote *types.Vote) {
	if p.knownVotes.Cardinality() > MaxKnownVotes {
		p.knownVotes.Pop()
	}
	p.knownVotes.Add(vote.Hash())
}

func (p *peer) markTx(tx *types.Transaction) {
	if p.knownTxs.Cardinality() > MaxKwownTxs {
		p.knownTxs.Pop()
	}
	p.knownTxs.Add(tx.Hash())
}

func (p *peer) markFlip(flip *types.Flip) {
	if p.knownFlips.Cardinality() > MaxKnownFlips {
		p.knownFlips.Pop()
	}
	p.knownFlips.Add(flip.Tx.Hash())
}

func (p *peer) markFlipKey(flipKey *types.FlipKey) {
	if p.knownFlipKeys.Cardinality() > MaxKnownFlipKeys {
		p.knownFlipKeys.Pop()
	}
	p.knownFlipKeys.Add(flipKey.Hash())
}

func (p *peer) setHeight(newHeight uint64) {
	if newHeight > p.knownHeight {
		p.knownHeight = newHeight
	}
	p.setPotentialHeight(newHeight)
}

func (p *peer) setPotentialHeight(newHeight uint64) {
	if newHeight > p.potentialHeight {
		p.potentialHeight = newHeight
	}
}
