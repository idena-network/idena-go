package consensus

import (
	"github.com/idena-network/idena-go/blockchain"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/protocol"
	"time"
)

const (
	minPeersWithNextBlock = 3
	checkHeightsTimeout   = time.Second * 30
)

type ForwardPeersProvider interface {
	PotentialForwardPeers(round uint64) []string
	PeerHeights() []uint64
}

type nextBlockDetector struct {
	fpProvider    ForwardPeersProvider
	blockSeeker   protocol.BlockSeeker
	activeSeeking chan *types.BlockBundle
	chain         *blockchain.Blockchain
	lastCheck     time.Time
}

func newNextBlockDetector(fpProvider ForwardPeersProvider, blockSeeker protocol.BlockSeeker, chain *blockchain.Blockchain) *nextBlockDetector {
	return &nextBlockDetector{fpProvider: fpProvider, blockSeeker: blockSeeker, chain: chain}
}

func (d *nextBlockDetector) nextBlockExist(round uint64, emptyBlockHash common.Hash) bool {

	if d.activeSeeking == nil {
		if time.Now().UTC().Sub(d.lastCheck) < checkHeightsTimeout {
			return false
		}
		heights := d.fpProvider.PeerHeights()

		fPeers := 0
		for _, h := range heights {
			if h >= round {
				fPeers++
			}
		}
		// all peers are ahead and minPeersWithNextBlock is reached
		if fPeers == len(heights) && fPeers >= minPeersWithNextBlock {
			return true
		}

		forwardPeers := d.fpProvider.PotentialForwardPeers(round)
		if len(forwardPeers) > 0 {
			d.activeSeeking = d.blockSeeker.SeekBlocks(round, round, forwardPeers)
		} else {
			d.lastCheck = time.Now().UTC()
			return false
		}
	}

	timeout := time.After(time.Second * 2)
	select {
	case bundle, ok := <-d.activeSeeking:
		if !ok || bundle == nil || bundle.Cert.Empty() ||
			d.chain.ValidateBlockCertOnHead(bundle.Block.Header, bundle.Cert) != nil {
			// read all bundles - reset state
			d.activeSeeking = nil
			return false
		}

		if bundle.Block.IsEmpty() {
			if bundle.Block.Hash() == emptyBlockHash {
				d.activeSeeking = nil
				return true
			}
		} else {
			if d.chain.ValidateBlock(bundle.Block, nil) == nil {
				d.activeSeeking = nil
				return true
			}
		}
	case <-timeout:
		return false
	}
	return false
}

func (d *nextBlockDetector) complete() {
	d.activeSeeking = nil
	d.lastCheck = time.Now().UTC()
}
