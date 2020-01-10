package protocol

import (
	"fmt"
	"github.com/golang/snappy"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/log"
	"github.com/idena-network/idena-go/rlp"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-msgio"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"math"
	"math/rand"
	"time"
)

const (
	handshakeTimeout     = 20 * time.Second
	msgCacheAliveTime    = 3 * time.Minute
	msgCacheGcTime       = 5 * time.Minute
	maxTimeoutsBeforeBan = 7
)

type protoPeer struct {
	id                   peer.ID
	stream               network.Stream
	rw                   msgio.ReadWriteCloser
	maxDelayMs           int
	knownHeight          uint64
	potentialHeight      uint64
	manifest             *snapshot.Manifest
	queuedRequests       chan *request
	highPriorityRequests chan *request
	term                 chan struct{}
	finished             chan struct{}
	msgCache             *cache.Cache
	appVersion           string
	timeouts             int
	log                  log.Logger
	createdAt            time.Time
	readErr              error
}

func newPeer(stream network.Stream, maxDelayMs int) *protoPeer {
	stream.Conn().RemotePeer()
	rw := msgio.NewReadWriter(stream)

	p := &protoPeer{
		id:                   stream.Conn().RemotePeer(),
		stream:               stream,
		rw:                   rw,
		queuedRequests:       make(chan *request, 10000),
		highPriorityRequests: make(chan *request, 500),
		term:                 make(chan struct{}),
		finished:             make(chan struct{}),
		maxDelayMs:           maxDelayMs,
		msgCache:             cache.New(msgCacheAliveTime, msgCacheGcTime),
		log:                  log.New("id", stream.Conn().RemotePeer().Pretty()),
		createdAt:            time.Now().UTC(),
	}
	return p
}

func (p *protoPeer) sendMsg(msgcode uint64, payload interface{}, highPriority bool) {
	if highPriority {
		select {
		case p.highPriorityRequests <- &request{msgcode: msgcode, data: payload}:
		case <-p.finished:
		}
	} else {
		select {
		case p.queuedRequests <- &request{msgcode: msgcode, data: payload}:
		case <-p.finished:
		}
	}
}

func (p *protoPeer) broadcast() {
	defer close(p.finished)
	send := func(request *request) error {
		msg := makeMsg(request.msgcode, request.data)

		if err := p.rw.WriteMsg(msg); err != nil {
			p.log.Error(err.Error())
			return err
		}
		return nil
	}
	for {
		if p.maxDelayMs > 0 {
			delay := time.Duration(rand.Int31n(int32(p.maxDelayMs)))
			time.Sleep(delay * time.Millisecond)
		}

		select {
		case request := <-p.highPriorityRequests:
			if send(request) != nil {
				return
			}
			continue
		default:
		}

		select {
		case request := <-p.highPriorityRequests:
			if send(request) != nil {
				return
			}
		case request := <-p.queuedRequests:
			if send(request) != nil {
				return
			}
		case <-p.term:
			return
		}
	}
}

func makeMsg(msgcode uint64, payload interface{}) []byte {
	data, err := rlp.EncodeToBytes(payload)
	if err != nil {
		panic(err)
	}
	msg, err := rlp.EncodeToBytes(&Msg{Code: msgcode, Payload: data})
	if err != nil {
		panic(err)
	}
	return snappy.Encode(nil, msg)
}

func (p *protoPeer) Handshake(network types.Network, height uint64, genesis common.Hash, appVersion string) error {
	errc := make(chan error, 2)
	handShake := new(handshakeData)
	p.log.Trace("start handshake")
	go func() {
		msg := makeMsg(Handshake, &handshakeData{
			NetworkId:    network,
			Height:       height,
			GenesisBlock: genesis,
			Timestamp:    uint64(time.Now().UTC().Unix()),
			AppVersion:   appVersion,
		})
		errc <- p.rw.WriteMsg(msg)
		p.log.Trace("handshake message sent")
	}()
	go func() {
		errc <- p.readStatus(handShake, network, genesis)
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
			return errors.New("handshake timeout")
		}
	}
	p.knownHeight = handShake.Height
	return nil
}

func (p *protoPeer) ReadMsg() (*Msg, error) {
	msg, err := p.rw.ReadMsg()
	defer p.rw.ReleaseMsg(msg)
	if err != nil {
		return nil, err
	}
	msg, err = snappy.Decode(nil, msg)
	if err != nil {
		return nil, err
	}
	result := new(Msg)
	if err := rlp.DecodeBytes(msg, result)
		err != nil {
		return nil, err
	}
	return result, nil
}

func (p *protoPeer) readStatus(handShake *handshakeData, network types.Network, genesis common.Hash) (err error) {
	p.log.Trace("read handshake data")
	msg, err := p.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != Handshake {
		return errors.New(fmt.Sprintf("first msg has code %x (!= %x)", msg.Code, Handshake))
	}
	if err := msg.Decode(&handShake); err != nil {
		return errors.New(fmt.Sprintf("can't decode msg %v: %v", msg, err))
	}
	p.appVersion = handShake.AppVersion
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

func (p *protoPeer) markPayload(payload interface{}) {
	p.markKey(msgKey(payload))
}

func (p *protoPeer) markKey(key string) {
	p.msgCache.Add(key, struct{}{}, cache.DefaultExpiration)
}

func msgKey(data interface{}) string {
	hash := rlp.Hash(data)
	return string(hash[:])
}

func (p *protoPeer) setHeight(newHeight uint64) {
	if newHeight > p.knownHeight {
		p.knownHeight = newHeight
	}
	p.setPotentialHeight(newHeight)
}

func (p *protoPeer) setPotentialHeight(newHeight uint64) {
	if newHeight > p.potentialHeight {
		p.potentialHeight = newHeight
	}
}

func (p *protoPeer) addTimeout() (shouldBeBanned bool) {
	p.timeouts++
	return p.timeouts > maxTimeoutsBeforeBan
}

func (p *protoPeer) resetTimeouts() {
	p.timeouts = 0
}

func (p *protoPeer) disconnect() {
	p.stream.Reset()
}

func (p *protoPeer) ID() string {
	return p.id.Pretty()
}

func (p *protoPeer) RemoteAddr() string {
	return p.stream.Conn().RemoteMultiaddr().String()
}
