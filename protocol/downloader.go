package protocol

import (
	"fmt"
	"github.com/pkg/errors"
	"idena-go/blockchain"
	"idena-go/blockchain/types"
	"idena-go/common/math"
	"idena-go/ipfs"
	"idena-go/log"
	"time"
)

const (
	BatchSize = 200
)

type Downloader struct {
	pm      *ProtocolManager
	log     log.Logger
	chain   *blockchain.Blockchain
	batches chan *batch
	ipfs    ipfs.Proxy
}

func NewDownloader(pm *ProtocolManager, chain *blockchain.Blockchain, ipfs ipfs.Proxy) *Downloader {
	return &Downloader{
		pm:    pm,
		chain: chain,
		log:   log.New("component", "downloader"),
		ipfs:  ipfs,
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
		top := getTopHeight(knownHeights)
		if head.Height() >= top {
			d.log.Info(fmt.Sprintf("Node is synchronized"))
			return
		}
		d.batches = make(chan *batch, 10)
		term := make(chan interface{})
		completed := make(chan interface{})
		go d.consumeBlocks(term, completed)

		from := head.Height() + 1
	loop:
		for ; from <= top; {
			for peer, height := range knownHeights {
				if height < from {
					continue
				}
				to := math.Min(from+BatchSize, height)
				if err, batch := d.pm.GetBlocksRange(peer, from, to); err != nil {
					continue
				} else {
					select {
					case d.batches <- batch:
					case <-term:
						break loop
					}
				}
				from = to + 1
			}
		}
		d.log.Info(fmt.Sprintf("All blocks was requested. Wait applying of blocks"))
		close(completed)
		<-term

		//TODO : we may have downloaded unprocessed batches which can be useful
	}
}

func (d *Downloader) consumeBlocks(term chan interface{}, completed chan interface{}) {
	defer close(term)
	for {
		timeout := time.After(time.Second * 15)

		select {
		case batch := <-d.batches:
			if err := d.processBatch(batch); err != nil {
				d.log.Warn("failed to process batch", "err", err)
				return
			}
			continue
		default:
		}

		select {
		case batch := <-d.batches:
			if err := d.processBatch(batch); err != nil {
				d.log.Warn("failed to process batch", "err", err)
				return
			}
			break
		case <-completed:
			return
		case <-timeout:
			return
		}
	}
}

func (d *Downloader) downloadBatch(from, to uint64, ignoredPeer string) *batch {
	knownHeights := d.pm.GetKnownHeights()
	if knownHeights == nil {
		return nil
	}
	for peerId, height := range knownHeights {
		if (peerId != ignoredPeer || len(knownHeights) == 1) && height >= to {
			if err, batch := d.pm.GetBlocksRange(peerId, from, to); err != nil {
				continue
			} else {
				return batch
			}
		}
	}
	return nil
}

func (d *Downloader) processBatch(batch *batch) error {
	d.log.Info("Start process batch", "from", batch.from, "to", batch.to)
	for i := batch.from; i <= batch.to; i++ {
		timeout := time.After(time.Second * 10)

		reload := func() error {
			b := d.downloadBatch(i, batch.to, batch.p.id)
			if b == nil {
				return errors.New(fmt.Sprintf("Batch (%v-%v) can't be loaded", i, batch.to))
			}
			return d.processBatch(b)
		}

		select {
		case header := <-batch.headers:
			if block, err := d.GetBlock(header); err != nil {
				d.log.Error("fail to retrieve block", "err", err)
				return reload()
			} else {
				if err := d.chain.AddBlock(block); err != nil {
					d.log.Warn(fmt.Sprintf("Block from peer %v is invalid: %v", batch.p.id, err))
					time.Sleep(time.Second)
					// TODO: ban bad peer
					return reload()
				}
			}
		case <-timeout:
			d.log.Warn("process batch - timeout was reached")
			return reload()
		}
	}
	d.log.Info("Finish process batch", "from", batch.from, "to", batch.to)
	return nil
}

func (d *Downloader) GetBlock(header *types.Header) (*types.Block, error) {
	if header.EmptyBlockHeader != nil {
		return &types.Block{
			Header: header,
			Body:   &types.Body{},
		}, nil
	}
	if txs, err := d.ipfs.Get(header.ProposedHeader.IpfsHash); err != nil {
		return nil, err
	} else {
		if len(txs) > 0 {
			d.log.Debug("Retrieve block body from ipfs", "hash", header.Hash().Hex())
		}
		body := &types.Body{}
		body.FromBytes(txs)
		return &types.Block{
			Header: header,
			Body:   body,
		}, nil
	}
}
