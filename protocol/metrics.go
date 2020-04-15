package protocol

import (
	"fmt"
	"github.com/idena-network/idena-go/log"
	"github.com/rcrowley/go-metrics"
	"time"
)

func (h *IdenaGossipHandler) registerMetrics() {

	totalSent := metrics.GetOrRegisterCounter("bytes_sent.total", metrics.DefaultRegistry)
	totalReceived := metrics.GetOrRegisterCounter("bytes_received.total", metrics.DefaultRegistry)

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
			return "glockRange"
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

	h.metrics.incomeMessage = func(msg *Msg) {
		if !h.cfg.CollectMetrics {
			return
		}
		collector := metrics.GetOrRegisterCounter("bytes_received."+msgCodeToString(msg.Code), metrics.DefaultRegistry)
		collector.Inc(int64(len(msg.Payload)))

		counter := metrics.GetOrRegisterCounter("msg_received."+msgCodeToString(msg.Code), metrics.DefaultRegistry)
		counter.Inc(1)

		totalReceived.Inc(int64(len(msg.Payload)))
	}

	h.metrics.outcomeMessage = func(msg *Msg) {
		if !h.cfg.CollectMetrics {
			return
		}
		collector := metrics.GetOrRegisterCounter("bytes_sent."+msgCodeToString(msg.Code), metrics.DefaultRegistry)
		collector.Inc(int64(len(msg.Payload)))

		counter := metrics.GetOrRegisterCounter("msg_sent."+msgCodeToString(msg.Code), metrics.DefaultRegistry)
		counter.Inc(1)

		totalSent.Inc(int64(len(msg.Payload)))
	}

	if h.cfg.CollectMetrics {
		go metrics.Log(metrics.DefaultRegistry, time.Second*20, metricsLog{h.log})
	}
}

type metricsLog struct {
	log log.Logger
}

func (m metricsLog) Printf(format string, v ...interface{}) {
	log.Info(fmt.Sprintf(format, v...))
}
