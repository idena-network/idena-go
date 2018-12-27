package protocol

import (
	"fmt"
	"idena-go/blockchain"
	"idena-go/common/math"
	"idena-go/log"
	"time"
)

const (
	BatchSize = 100
)

type Downloader struct {
	pm      *ProtocolManager
	log     log.Logger
	chain   *blockchain.Blockchain
	batches chan *batch
}

func NewDownloader(pm *ProtocolManager, chain *blockchain.Blockchain) *Downloader {
	return &Downloader{
		pm:      pm,
		chain:   chain,
		log:     log.New(),
		batches: make(chan *batch, 10),
	}
}

func getTopHeight(heights map[string]uint64) uint64 {
	max := uint64(0)
	for _, value := range heights {
		if value > max {
			max = value
		}
	}
	return max
}

func (d *Downloader) SyncBlockchain() {
	for {
		knownHeights := d.pm.GetKnownHeights()
		if knownHeights == nil {
			d.log.Info(fmt.Sprintf("Peers are not found. Assume node is synchronized"))
			break
		}
		head := d.chain.Head

		if head.Height() >= getTopHeight(knownHeights) {
			d.log.Info(fmt.Sprintf("Node is synchronized"))
			return
		}

		term := make(chan interface{})
		completed := make(chan interface{})
		go d.consumeBlocks(term, completed)

		from := head.Height() + 1
		for peer, height := range knownHeights {
			if height < from {
				continue
			}
			to := math.Min(from+BatchSize, height)
			if err, batch := d.pm.GetBlocksRange(peer, from, to); err != nil {
				continue
			} else {
				d.batches <- batch
			}
			from = to
		}
		d.log.Info(fmt.Sprintf("All blocks requested. Wait block applying"))
		close(completed)
		<-term
	}
}

func (d *Downloader) consumeBlocks(term chan interface{}, completed chan interface{}) {

	for {
		timeout := time.After(time.Second * 5)

		select {
		case batch := <-d.batches:
			d.processBatch(batch)
			continue
		default:
		}

		select {
		case batch := <-d.batches:
			d.processBatch(batch)
			break
		case <-completed:
			close(term)
			return
		case <-timeout:
			close(term)
			return
		}
	}
}

func (d *Downloader) processBatch(batch *batch) {
	for i := batch.from; i <= batch.to; i++ {
		timeout := time.After(time.Second * 10)
		select {
		case block := <-batch.blocks:
			if err := d.chain.AddBlock(block); err != nil {
				d.log.Warn(fmt.Sprintf("Block from peer %v is invalid: %v", batch.p.id, err))
				//TODO : reload batch from another peer, disconnect and ban that peer
			}
		case <-timeout:
			//TODO : download batch from another peer
		}
	}
}
