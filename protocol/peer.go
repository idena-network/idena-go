package protocol

import (
	"fmt"
	"github.com/coreos/go-semver/semver"
	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state/snapshot"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/log"
	models "github.com/idena-network/idena-go/protobuf"
	s2 "github.com/klauspost/compress/s2"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-msgio"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"math"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	handshakeTimeout         = 20 * time.Second
	msgCacheAliveTime        = 3 * time.Minute
	flipKeyMsgCacheAliveTime = 10 * time.Minute
	msgCacheGcTime           = 5 * time.Minute

	maxTimeoutsBeforeBan = 7

	minCompressionSize = 386 // bytes

	pushQueueSize    = 30000
	flipKeyQueueSize = 30000

	queuedRequestsSize             = 15000
	queuedHighPriorityRequestsSize = 4000
)

type compression = byte

const (
	noCompression compression = 0
	s2Compression compression = 1
)

type syncHeight struct {
	value uint64
	lock  sync.RWMutex
}

type queueItem struct {
	payload interface{}
	shardId common.ShardId
}

func (s *syncHeight) Store(value uint64) {
	s.lock.Lock()
	if value > s.value {
		s.value = value
	}
	s.lock.Unlock()
}

func (s *syncHeight) Read() uint64 {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.value
}

type protoPeer struct {
	id                   peer.ID
	prettyId             string
	stream               network.Stream
	rw                   msgio.ReadWriteCloser
	maxDelayMs           int
	knownHeight          *syncHeight
	potentialHeight      *syncHeight
	manifestLock         sync.Mutex
	manifest             *snapshot.Manifest
	queuedRequests       chan *request
	highPriorityRequests chan *request
	pushQueue            chan *queueItem
	flipKeyQueue         chan *queueItem
	term                 chan struct{}
	finished             chan struct{}
	msgCache             *cache.Cache
	appVersion           string
	timeouts             int
	log                  log.Logger
	throttlingLogger     log.ThrottlingLogger
	createdAt            time.Time
	transportErr         chan error
	peers                uint32
	metrics              *metricCollector
	skippedRequestsCount uint32
	shardId              common.ShardId
	version              *semver.Version
	closed               bool
	supportedFeatures    map[PeerFeature]struct{}
	disconnectReason     string
}

func newPeer(stream network.Stream, maxDelayMs int, metrics *metricCollector) *protoPeer {
	stream.Conn().RemotePeer()
	rw := msgio.NewReadWriter(stream)

	id := stream.Conn().RemotePeer()
	prettyId := id.Pretty()
	logger := log.New("id", prettyId)
	throttlingLogger := log.NewThrottlingLogger(logger)

	parts := strings.Split(string(stream.Protocol()), "/")
	vers, err := semver.NewVersion(parts[len(parts)-1])
	if err != nil {
		vers, _ = semver.NewVersion("1.0.0")
	}

	p := &protoPeer{
		id:                   id,
		prettyId:             prettyId,
		stream:               stream,
		rw:                   rw,
		queuedRequests:       make(chan *request, queuedRequestsSize),
		highPriorityRequests: make(chan *request, queuedHighPriorityRequestsSize),
		pushQueue:            make(chan *queueItem, pushQueueSize),
		flipKeyQueue:         make(chan *queueItem, flipKeyQueueSize),
		term:                 make(chan struct{}),
		finished:             make(chan struct{}),
		maxDelayMs:           maxDelayMs,
		msgCache:             cache.New(msgCacheAliveTime, msgCacheGcTime),
		log:                  logger,
		throttlingLogger:     throttlingLogger,
		createdAt:            time.Now().UTC(),
		metrics:              metrics,
		transportErr:         make(chan error, 1),
		knownHeight:          &syncHeight{},
		potentialHeight:      &syncHeight{},
		version:              vers,
		supportedFeatures:    map[PeerFeature]struct{}{},
	}
	SetSupportedFeatures(p)
	return p
}

func (p *protoPeer) addPushToBatch(payload interface{}, shardId common.ShardId) {
	select {
	case p.pushQueue <- &queueItem{payload: payload, shardId: shardId}:
		atomic.StoreUint32(&p.skippedRequestsCount, 0)
	case <-p.finished:
	default:
		atomic.AddUint32(&p.skippedRequestsCount, 1)
		if p.skippedRequestsCount > queuedRequestsSize {
			p.throttlingLogger.Warn("Skipped requests limit reached for pushes", "addr", p.stream.Conn().RemoteMultiaddr().String())
			p.disconnect("too many skipped pushes")
		}
	}
}

func (p *protoPeer) addFlipKeyToBatch(payload interface{}, shardId common.ShardId) {
	select {
	case p.flipKeyQueue <- &queueItem{payload: payload, shardId: shardId}:
		atomic.StoreUint32(&p.skippedRequestsCount, 0)
	case <-p.finished:
	default:
		atomic.AddUint32(&p.skippedRequestsCount, 1)
		if p.skippedRequestsCount > queuedRequestsSize {
			p.throttlingLogger.Warn("Skipped requests limit reached for flip keys", "addr", p.stream.Conn().RemoteMultiaddr().String())
			p.disconnect("too many skipped flip keys")
		}
	}
}

func (p *protoPeer) sendMsg(msgcode uint64, payload interface{}, shardId common.ShardId, highPriority bool) {
	if highPriority {
		timer := time.NewTimer(time.Second * 5)
		defer timer.Stop()
		select {
		case p.highPriorityRequests <- &request{msgcode: msgcode, data: payload, shardId: shardId}:
		case <-timer.C:
			p.log.Error("TIMEOUT while sending message (high priority)", "addr", p.stream.Conn().RemoteMultiaddr().String(), "len", len(p.highPriorityRequests))
			p.disconnect("timeout while sending message (high priority)")
		case <-p.finished:
		}
	} else {
		if msgcode == Push || msgcode == FlipKey {
			if _, ok := p.supportedFeatures[Batches]; ok {
				switch msgcode {
				case Push:
					p.addPushToBatch(payload, shardId)
				case FlipKey:
					p.addFlipKeyToBatch(payload, shardId)
				}
				return
			}
		}

		select {
		case p.queuedRequests <- &request{msgcode: msgcode, data: payload, shardId: shardId}:
			atomic.StoreUint32(&p.skippedRequestsCount, 0)
		case <-p.finished:
		default:
			atomic.AddUint32(&p.skippedRequestsCount, 1)
			if p.skippedRequestsCount > queuedRequestsSize/2 {
				p.throttlingLogger.Warn("Skipped requests limit reached", "addr", p.stream.Conn().RemoteMultiaddr().String())
				p.disconnect("too many skipped requests")
			}
		}
	}
}

func (p *protoPeer) makeBatches() {
	const batchSize = 100

	convertItem := func(queueItem *queueItem) *batchItem {
		var data []byte
		switch queueItem.payload.(type) {
		case pushPullHash:
			pushPull := queueItem.payload.(pushPullHash)
			data, _ = pushPull.ToBytes()
		case *types.PublicFlipKey:
			flipKey := queueItem.payload.(*types.PublicFlipKey)
			data, _ = flipKey.ToBytes()
		}
		return &batchItem{Payload: data, ShardId: queueItem.shardId}
	}

	for {
		select {
		case push := <-p.pushQueue:
			batch := new(msgBatch)
			batch.Data = make([]*batchItem, 0, batchSize)
			batch.Data = append(batch.Data, convertItem(push))
		pushLoop:
			for i := 0; i < batchSize-1; i++ {
				select {
				case push = <-p.pushQueue:
					batch.Data = append(batch.Data, convertItem(push))
				default:
					break pushLoop
				}
			}
			p.sendMsg(BatchPush, batch, common.MultiShard, false)
		case flipKey := <-p.flipKeyQueue:
			batch := new(msgBatch)
			batch.Data = make([]*batchItem, 0, batchSize)
			batch.Data = append(batch.Data, convertItem(flipKey))
		flipKeyLoop:
			for i := 0; i < batchSize-1; i++ {
				select {
				case flipKey = <-p.pushQueue:
					batch.Data = append(batch.Data, convertItem(flipKey))
				default:
					break flipKeyLoop
				}
			}
			p.sendMsg(BatchFlipKey, batch, common.MultiShard, false)
		case <-p.term:
			return
		}
	}
}

func (p *protoPeer) broadcast() {
	go p.makeBatches()
	defer close(p.finished)
	defer p.disconnect("")
	send := func(request *request) error {
		msg := makeMsg(request.msgcode, request.data, request.shardId)

		ch := make(chan error, 1)
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		startTime := time.Now()
		go func() {
			ch <- p.rw.WriteMsg(msg)
		}()
		select {
		case err := <-ch:
			if err != nil {
				p.log.Error("error while writing to stream", "err", err)
				select {
				case p.transportErr <- err:
				default:
				}
				return err
			}
		case <-timer.C:
			err := errors.New("TIMEOUT while writing to stream")
			p.log.Error(err.Error(), "addr", p.stream.Conn().RemoteMultiaddr().String())
			return err
		}
		duration := time.Since(startTime)
		p.metrics.outcomeMessage(request.msgcode, len(msg), duration, p.prettyId)
		return nil
	}
	logIfNeeded := func(r *request) {
		if r.msgcode == Push || r.msgcode == NewTx || r.msgcode == FlipKey {
			p.log.Debug(fmt.Sprintf("Sent high priority msg, code %v", r.msgcode))
		}
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
			logIfNeeded(request)
			continue
		default:
		}

		select {
		case request := <-p.highPriorityRequests:
			if send(request) != nil {
				return
			}
			logIfNeeded(request)
		case request := <-p.queuedRequests:
			if send(request) != nil {
				return
			}
		case <-p.term:
			return
		}
	}
}

func makeMsg(msgcode uint64, payload interface{}, shardId common.ShardId) []byte {
	data, err := toBytes(msgcode, payload)
	if err != nil {
		panic(err)
	}
	msg, err := (&Msg{Code: msgcode, Payload: data, ShardId: shardId}).ToBytes()
	if err != nil {
		panic(err)
	}
	return Encode(msgcode, msg)
}

func toBytes(msgcode uint64, payload interface{}) ([]byte, error) {
	switch msgcode {
	case Handshake:
		return payload.(*handshakeData).ToBytes()
	case ProposeBlock:
		return payload.(*types.BlockProposal).ToBytes()
	case ProposeProof:
		return payload.(*types.ProofProposal).ToBytes()
	case Vote:
		return payload.(*types.Vote).ToBytes()
	case NewTx:
		return payload.(*types.Transaction).ToBytes()
	case GetBlockByHash:
		return proto.Marshal(payload.(*models.ProtoGetBlockByHashRequest))
	case GetBlocksRange:
		return proto.Marshal(payload.(*models.ProtoGetBlocksRangeRequest))
	case BlocksRange:
		return payload.(*blockRange).ToBytes()
	case FlipBody:
		return payload.(*types.Flip).ToBytes()
	case FlipKey:
		return payload.(*types.PublicFlipKey).ToBytes()
	case SnapshotManifest:
		return payload.(*snapshot.Manifest).ToBytes()
	case GetForkBlockRange:
		return proto.Marshal(payload.(*models.ProtoGetForkBlockRangeRequest))
	case FlipKeysPackage:
		return payload.(*types.PrivateFlipKeysPackage).ToBytes()
	case Push, Pull:
		pullPush := payload.(pushPullHash)
		return pullPush.ToBytes()
	case Block:
		return payload.(*types.Block).ToBytes()
	case UpdateShardId:
		return payload.(*updateShardId).ToBytes()
	case BatchPush, BatchFlipKey:
		return payload.(*msgBatch).ToBytes()
	case Disconnect:
		return payload.(*disconnect).ToBytes()
	}
	return nil, errors.Errorf("type %T is not serializable", payload)
}

func (p *protoPeer) Handshake(network types.Network, height uint64, genesis *types.GenesisInfo, appVersion string, peersCount uint32, shardId common.ShardId) error {
	errc := make(chan error, 2)
	handShake := new(handshakeData)
	p.log.Trace("start handshake")
	go func() {
		data := &handshakeData{
			NetworkId:    network,
			Height:       height,
			GenesisBlock: genesis.Genesis.Hash(),
			Timestamp:    time.Now().UTC().Unix(),
			AppVersion:   appVersion,
			Peers:        peersCount,
			ShardId:      shardId,
		}
		if genesis.OldGenesis != nil {
			hash := genesis.OldGenesis.Hash()
			data.OldGenesis = &hash
		}

		msg := makeMsg(Handshake, data, 0)
		errc <- p.rw.WriteMsg(msg)
		p.log.Trace("handshake message sent", "shardId", shardId)
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
	p.knownHeight.Store(handShake.Height)
	p.peers = handShake.Peers
	p.shardId = handShake.ShardId
	return nil
}

func Decode(src []byte) ([]byte, error) {

	if len(src) == 0 {
		return src, errors.New("msg is empty")
	}

	switch src[0] {
	case noCompression:
		return src[1:], nil
	case s2Compression:
		return s2.Decode(nil, src[1:])
	default:
		return nil, errors.New("unknown compression")
	}
}

func Encode(msgcode uint64, src []byte) []byte {
	if msgcode == FlipKeysPackage || len(src) < minCompressionSize {
		return append([]byte{noCompression}, src...)
	}
	return append([]byte{s2Compression}, s2.Encode(nil, src)...)
}

func (p *protoPeer) ReadMsg() (*Msg, error) {
	startTime := time.Now()
	compressedMsg, err := p.rw.ReadMsg()
	defer p.rw.ReleaseMsg(compressedMsg)
	if err != nil {
		select {
		case p.transportErr <- err:
		default:
		}
		return nil, err
	}
	duration := time.Since(startTime)
	data, err := Decode(compressedMsg)
	if err != nil {
		return nil, err
	}
	result := new(Msg)
	if err := result.FromBytes(data); err != nil {
		return nil, err
	}
	p.metrics.incomeMessage(result.Code, len(compressedMsg), duration, p.prettyId)
	p.metrics.compress(result.Code, len(data)-len(compressedMsg))
	return result, nil
}

func (p *protoPeer) readStatus(handShake *handshakeData, network types.Network, genesis *types.GenesisInfo) (err error) {
	p.log.Trace("read handshake data")
	msg, err := p.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != Handshake {
		return errors.New(fmt.Sprintf("first msg has code %x (!= %x)", msg.Code, Handshake))
	}
	if err := handShake.FromBytes(msg.Payload); err != nil {
		return errors.New(fmt.Sprintf("can't decode handshake %v: %v", msg, err))
	}
	p.appVersion = handShake.AppVersion
	if !genesis.EqualAny(handShake.GenesisBlock, handShake.OldGenesis) {
		return errors.New(fmt.Sprintf("bad genesis block %x (!= %x)", handShake.GenesisBlock[:8], genesis.Genesis.Hash().Bytes()[:8]))
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

func (p *protoPeer) markKey(key string) {
	p.markKeyWithExpiration(key, cache.DefaultExpiration)
}

func (p *protoPeer) unmarkKey(key string) {
	p.msgCache.Delete(key)
}

func (p *protoPeer) markKeyWithExpiration(key string, expiration time.Duration) {
	p.msgCache.Add(key, struct{}{}, expiration)
}

func msgKey(data []byte) string {
	hash := crypto.Hash(data)
	return string(hash[:])
}

func (p *protoPeer) setHeight(newHeight uint64) {
	if newHeight > p.knownHeight.Read() {
		p.knownHeight.Store(newHeight)
	}
	p.setPotentialHeight(newHeight)
}

func (p *protoPeer) setPotentialHeight(newHeight uint64) {
	if newHeight > p.potentialHeight.Read() {
		p.potentialHeight.Store(newHeight)
	}
}

func (p *protoPeer) addTimeout() (shouldBeBanned bool) {
	p.timeouts++
	return p.timeouts > maxTimeoutsBeforeBan
}

func (p *protoPeer) resetTimeouts() {
	p.timeouts = 0
}

func (p *protoPeer) disconnect(reason string) {
	if reason != "" {
		var dc = &disconnect{reason}
		msg := makeMsg(Disconnect, dc, common.MultiShard)
		p.rw.WriteMsg(msg)
		time.Sleep(time.Second)
	}
	if err := p.stream.Reset(); err != nil {
		p.log.Error("error while resetting peer stream", "err", err)
	}
}

func (p *protoPeer) ID() string {
	return p.id.Pretty()
}

func (p *protoPeer) RemoteAddr() string {
	return p.stream.Conn().RemoteMultiaddr().String()
}

func (p *protoPeer) Manifest() *snapshot.Manifest {
	p.manifestLock.Lock()
	defer p.manifestLock.Unlock()
	return p.manifest
}
