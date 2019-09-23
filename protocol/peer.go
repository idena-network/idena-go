package protocol

import (
	"fmt"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/p2p"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"math"
	"math/rand"
	"time"
)

const (
	handshakeTimeout = 10 * time.Second
)

type peer struct {
	*p2p.Peer
	rw              p2p.MsgReadWriter
	id              string
	maxDelayMs      int
	knownHeight     uint64
	potentialHeight uint64
	manifest        *snapshot.Manifest
	queuedRequests  chan *request
	term            chan struct{}
	finished        chan struct{}
	msgCache        *cache.Cache
}

type request struct {
	msgcode uint64
	data    interface{}
}

func (pm *ProtocolManager) makePeer(p *p2p.Peer, rw p2p.MsgReadWriter, maxDelayMs int) *peer {
	return &peer{
		rw:             rw,
		Peer:           p,
		id:             fmt.Sprintf("%x", p.ID().Bytes()[:8]),
		queuedRequests: make(chan *request, 500),
		term:           make(chan struct{}),
		finished:       make(chan struct{}),
		maxDelayMs:     maxDelayMs,
		msgCache:       cache.New(3*time.Minute, 5*time.Minute),
	}
}

func (p *peer) sendMsg(msgcode uint64, payload interface{}) {
	select {
	case p.queuedRequests <- &request{msgcode: msgcode, data: payload}:
	case <-p.finished:
	}
}

func (p *peer) broadcast() {
	defer close(p.finished)
	for {
		if p.maxDelayMs > 0 {
			delay := time.Duration(rand.Int31n(int32(p.maxDelayMs)))
			time.Sleep(delay * time.Millisecond)
		}
		select {
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
			Timestamp:    uint64(time.Now().UTC().Unix()),
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
		return errors.New(fmt.Sprintf("bad genesis block %x (!= %x)", handShake.GenesisBlock[:8], genesis[:8]))
	}
	if handShake.NetworkId != network {
		return errors.New(fmt.Sprintf("network mismatch: %d (!= %d)", handShake.NetworkId, network))
	}
	diff := math.Abs(float64(time.Now().UTC().Unix() - int64(handShake.Timestamp)))
	if diff > MaxTimestampLagSeconds {
		return errors.New(fmt.Sprintf("time difference is too big (%v sec)", diff))
	}

	return nil
}

func (p *peer) markMessage(key string) {
	p.msgCache.Add(key, struct{}{}, cache.DefaultExpiration)
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
