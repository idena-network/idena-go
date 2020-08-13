package protocol

import (
	"fmt"
	"github.com/idena-network/idena-go/log"
	"github.com/rcrowley/go-metrics"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateMetric struct {
	size     int
	duration time.Duration
	mutex    sync.Mutex
}

func (rm *rateMetric) add(size int, duration time.Duration) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	rm.size += size
	rm.duration += duration
}

func (rm *rateMetric) getAndReset() (size int, duration time.Duration) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	size = rm.size
	duration = rm.duration
	rm.size = 0
	rm.duration = 0

	return
}

type rateMetrics struct {
	in  *rateMetric
	out *rateMetric
}

func (rm *rateMetrics) addIn(size int, duration time.Duration) {
	rm.in.add(size, duration)
}

func (rm *rateMetrics) addOut(size int, duration time.Duration) {
	rm.out.add(size, duration)
}

type peersRateMetrics struct {
	rmsByPeer map[string]*rateMetrics
	enabled   func() bool
	mutex     sync.RWMutex
}

func newPeersRateMetrics(enabled func() bool) *peersRateMetrics {
	return &peersRateMetrics{
		enabled:   enabled,
		rmsByPeer: make(map[string]*rateMetrics),
	}
}

func (rm *peersRateMetrics) getPeerRateMetrics(peerId string) *rateMetrics {
	rm.mutex.RLock()
	prm, ok := rm.rmsByPeer[peerId]
	rm.mutex.RUnlock()
	if ok {
		return prm
	}
	rm.mutex.Lock()
	defer rm.mutex.Unlock()
	prm, ok = rm.rmsByPeer[peerId]
	if ok {
		return prm
	}
	prm = &rateMetrics{
		in:  &rateMetric{},
		out: &rateMetric{},
	}
	rm.rmsByPeer[peerId] = prm
	return prm
}

func (rm *peersRateMetrics) addIn(peerId string, size int, duration time.Duration) {
	if !rm.enabled() {
		return
	}
	peersRateMetrics := rm.getPeerRateMetrics(peerId)
	peersRateMetrics.addIn(size, duration)
}

func (rm *peersRateMetrics) addOut(peerId string, size int, duration time.Duration) {
	if !rm.enabled() {
		return
	}
	peersRateMetrics := rm.getPeerRateMetrics(peerId)
	peersRateMetrics.addOut(size, duration)
}

func (rm *peersRateMetrics) loopLog(logger log.Logger) {
	for {
		time.Sleep(time.Second * 5)

		rm.mutex.Lock()
		rmsByPeer := rm.rmsByPeer
		rm.rmsByPeer = make(map[string]*rateMetrics)
		rm.mutex.Unlock()

		getMsg := func(size int, duration time.Duration) string {
			if duration == 0 {
				return ""
			}
			rate := float32(size) / float32(1024) / float32(duration) * float32(time.Second)
			return fmt.Sprintf("b: %v, d: %v, r(kb/s): %v", size, duration, strconv.FormatFloat(float64(rate), 'f', 3, 64))
		}
		var msgItems []string
		for peerId, prm := range rmsByPeer {
			sizeIn, durationIn := prm.in.getAndReset()
			sizeOut, durationOut := prm.out.getAndReset()

			if durationIn == 0 && durationOut == 0 {
				continue
			}

			var peerMsgItems []string
			if msg := getMsg(sizeIn, durationIn); len(msg) > 0 {
				peerMsgItems = append(peerMsgItems, "in: "+msg)
			}
			if msg := getMsg(sizeOut, durationOut); len(msg) > 0 {
				peerMsgItems = append(peerMsgItems, "out: "+msg)
			}
			if len(peerMsgItems) > 0 {
				msgItems = append(msgItems, fmt.Sprintf("%v: %v", peerId, strings.Join(peerMsgItems, ", ")))
			}
		}
		if len(msgItems) > 0 {
			logger.Info(strings.Join(msgItems, ", "))
		}
	}
}

func (h *IdenaGossipHandler) registerMetrics() {

	totalSent := metrics.GetOrRegisterCounter("bytes_sent.total", metrics.DefaultRegistry)
	totalReceived := metrics.GetOrRegisterCounter("bytes_received.total", metrics.DefaultRegistry)
	compressTotal := metrics.GetOrRegisterCounter("compress-diff.total", metrics.DefaultRegistry)
	rate := newPeersRateMetrics(h.isCeremony)

	msgCodeToString := func(code uint64) string {
		switch code {
		case Handshake:
			return "handshake"
		case ProposeBlock:
			return "proposeBlock"
		case ProposeProof:
			return "proposeProof"
		case Vote:
			return "vote"
		case NewTx:
			return "newTx"
		case GetBlockByHash:
			return "getBlockByHash"
		case GetBlocksRange:
			return "getBlocksRange"
		case BlocksRange:
			return "blockRange"
		case FlipBody:
			return "flipBody"
		case FlipKey:
			return "flipKey"
		case SnapshotManifest:
			return "snapshotManifest"
		case Push:
			return "push"
		case Pull:
			return "pull"
		case GetForkBlockRange:
			return "getForkBlockRange"
		case FlipKeysPackage:
			return "flipKeysPackage"
		case Block:
			return "block"
		default:
			return fmt.Sprintf("unknown code %v", code)
		}
	}

	loopCleanup := func() {
		codes := []uint64{
			Handshake,
			ProposeBlock,
			ProposeProof,
			Vote,
			NewTx,
			GetBlockByHash,
			GetBlocksRange,
			BlocksRange,
			FlipBody,
			FlipKey,
			SnapshotManifest,
			GetForkBlockRange,
			FlipKeysPackage,
			Push,
			Pull,
			Block,
		}
		for {
			time.Sleep(time.Hour)
			totalSent.Clear()
			totalReceived.Clear()
			compressTotal.Clear()
			for _, code := range codes {
				metrics.Unregister("bytes_received." + msgCodeToString(code))
				metrics.Unregister("msg_received." + msgCodeToString(code))
				metrics.Unregister("bytes_sent." + msgCodeToString(code))
				metrics.Unregister("msg_sent." + msgCodeToString(code))
				metrics.Unregister("compress-diff." + msgCodeToString(code))
			}
		}
	}

	h.metrics.incomeMessage = func(code uint64, size int, duration time.Duration, peerId string) {
		if h.cfg.DisableMetrics {
			return
		}
		collector := metrics.GetOrRegisterCounter("bytes_received."+msgCodeToString(code), metrics.DefaultRegistry)
		collector.Inc(int64(size))

		counter := metrics.GetOrRegisterCounter("msg_received."+msgCodeToString(code), metrics.DefaultRegistry)
		counter.Inc(1)

		totalReceived.Inc(int64(size))

		rate.addIn(peerId, size, duration)
	}

	h.metrics.outcomeMessage = func(code uint64, size int, duration time.Duration, peerId string) {
		if h.cfg.DisableMetrics {
			return
		}
		collector := metrics.GetOrRegisterCounter("bytes_sent."+msgCodeToString(code), metrics.DefaultRegistry)
		collector.Inc(int64(size))

		counter := metrics.GetOrRegisterCounter("msg_sent."+msgCodeToString(code), metrics.DefaultRegistry)
		counter.Inc(1)

		totalSent.Inc(int64(size))

		rate.addOut(peerId, size, duration)
	}

	h.metrics.compress = func(code uint64, size int) {
		if h.cfg.DisableMetrics {
			return
		}
		compressCnt := metrics.GetOrRegisterCounter("compress-diff."+msgCodeToString(code), metrics.DefaultRegistry)
		compressCnt.Inc(int64(size))
		compressTotal.Inc(int64(size))
	}

	if !h.cfg.DisableMetrics {
		go metrics.Log(metrics.DefaultRegistry, time.Second*20, metricsLog{h.log})
		go rate.loopLog(h.log)
		go loopCleanup()
	}
}

type metricsLog struct {
	log log.Logger
}

func (m metricsLog) Printf(format string, v ...interface{}) {
	format = strings.TrimSuffix(format, "\n")
	log.Info(fmt.Sprintf(format, v...))
}
