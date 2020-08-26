package protocol

import (
	"bytes"
	"fmt"
	"github.com/idena-network/idena-go/log"
	"github.com/rcrowley/go-metrics"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
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
	peerRateMetrics := rm.getPeerRateMetrics(peerId)
	peerRateMetrics.addIn(size, duration)
}

func (rm *peersRateMetrics) addOut(peerId string, size int, duration time.Duration) {
	if !rm.enabled() {
		return
	}
	peerRateMetrics := rm.getPeerRateMetrics(peerId)
	peerRateMetrics.addOut(size, duration)
}

func (rm *peersRateMetrics) loopLog(logger log.Logger) {
	for {
		time.Sleep(time.Second * 30)

		rm.mutex.Lock()
		rmsByPeer := rm.rmsByPeer
		rm.rmsByPeer = make(map[string]*rateMetrics)
		rm.mutex.Unlock()

		writer := new(tabwriter.Writer)
		buffer := new(bytes.Buffer)
		writer.Init(buffer, 8, 8, 1, ' ', 0)
		fmt.Fprintf(writer, "\n %s\t%s\t%s\t%s\t%s\t%s\t%s\t", "peer", "bytesSent", "duration", "rate(kb/s)",
			"bytesReceived", "duration", "rate(kb/s)")

		empty := true
		for peerId, prm := range rmsByPeer {
			sizeIn, durationIn := prm.in.getAndReset()
			sizeOut, durationOut := prm.out.getAndReset()
			var rateIn, rateOut float64
			if durationIn > 0 {
				rateIn = float64(sizeIn) / float64(1024) / float64(durationIn) * float64(time.Second)
			}
			if durationOut > 0 {
				rateOut = float64(sizeOut) / float64(1024) / float64(durationOut) * float64(time.Second)
			}
			fmt.Fprintf(writer, "\n %s\t%d\t%v\t%s\t%d\t%v\t%s\t", peerId, sizeOut, durationOut,
				strconv.FormatFloat(rateOut, 'f', 3, 64), sizeIn, durationIn,
				strconv.FormatFloat(rateIn, 'f', 3, 64))
			empty = false
		}
		writer.Flush()
		if !empty {
			logger.Info("rate metric" + buffer.String())
		}
	}
}

func (h *IdenaGossipHandler) registerMetrics() {

	totalSent := metrics.GetOrRegisterCounter("bs.total", metrics.DefaultRegistry)
	totalReceived := metrics.GetOrRegisterCounter("br.total", metrics.DefaultRegistry)
	compressTotal := metrics.GetOrRegisterCounter("cd.total", metrics.DefaultRegistry)
	rate := newPeersRateMetrics(h.ceremonyChecker.IsRunning)

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

	sortedMetricCodes := []uint64{
		Block,
		BlocksRange,
		FlipBody,
		FlipKey,
		FlipKeysPackage,
		GetBlockByHash,
		GetBlocksRange,
		GetForkBlockRange,
		Handshake,
		NewTx,
		ProposeBlock,
		ProposeProof,
		Pull,
		Push,
		SnapshotManifest,
		Vote,
	}

	loopLog := func() {
		startTime := time.Now()

		cleanUp := func() {
			totalSent.Clear()
			totalReceived.Clear()
			compressTotal.Clear()
			for _, code := range sortedMetricCodes {
				metrics.Unregister("br." + msgCodeToString(code))
				metrics.Unregister("mr." + msgCodeToString(code))
				metrics.Unregister("bs." + msgCodeToString(code))
				metrics.Unregister("ms." + msgCodeToString(code))
				metrics.Unregister("cd." + msgCodeToString(code))
			}
			startTime = time.Now()
		}

		const codeTotal = "total"
		type metricData struct {
			bytesSent        int64
			bytesReceived    int64
			messagesSent     int64
			messagesReceived int64
		}
		metricCodesMap := make(map[string]struct{})
		for _, metricCode := range sortedMetricCodes {
			metricCodesMap[msgCodeToString(metricCode)] = struct{}{}
		}
		metricCodesMap[codeTotal] = struct{}{}

		logMetrics := func() {
			metricsData := make(map[string]*metricData)
			metrics.DefaultRegistry.Each(func(name string, i interface{}) {
				switch metric := i.(type) {
				case metrics.Counter:
					nameParts := strings.Split(name, ".")
					if len(nameParts) != 2 {
						return
					}
					code := nameParts[1]
					if _, ok := metricCodesMap[code]; !ok {
						return
					}
					data, ok := metricsData[code]
					if !ok {
						data = &metricData{}
						metricsData[code] = data
					}
					metricType := nameParts[0]
					switch metricType {
					case "bs":
						data.bytesSent = metric.Count()
					case "br":
						data.bytesReceived = metric.Count()
					case "ms":
						data.messagesSent = metric.Count()
					case "mr":
						data.messagesReceived = metric.Count()
					}
				}
			})
			if len(metricsData) > 0 {
				writer := new(tabwriter.Writer)
				buffer := new(bytes.Buffer)
				writer.Init(buffer, 8, 8, 1, ' ', 0)
				fmt.Fprintf(writer, "\n %s\t%s\t%s\t%s\t%s\t", "name", "bytesSent", "bytesReceived", "msgSent", "msgReceived")
				for _, metricCode := range sortedMetricCodes {
					strCode := msgCodeToString(metricCode)
					data, ok := metricsData[strCode]
					if !ok {
						continue
					}
					fmt.Fprintf(writer, "\n %s\t%d\t%d\t%d\t%d\t", strCode, data.bytesSent, data.bytesReceived, data.messagesSent, data.messagesReceived)
				}
				if data, ok := metricsData[codeTotal]; ok {
					fmt.Fprintf(writer, "\n %s\t%d\t%d\t%d\t%d\t", codeTotal, data.bytesSent, data.bytesReceived, data.messagesSent, data.messagesReceived)
				}
				writer.Flush()
				log.Info(fmt.Sprintf("metric since %v", startTime.UTC().String()) + buffer.String())
			}
		}
		for {
			time.Sleep(time.Minute * 5)
			logMetrics()
			if time.Now().Sub(startTime) > time.Hour {
				cleanUp()
			}
		}
	}

	h.metrics.incomeMessage = func(code uint64, size int, duration time.Duration, peerId string) {
		if h.cfg.DisableMetrics {
			return
		}
		collector := metrics.GetOrRegisterCounter("br."+msgCodeToString(code), metrics.DefaultRegistry)
		collector.Inc(int64(size))

		counter := metrics.GetOrRegisterCounter("mr."+msgCodeToString(code), metrics.DefaultRegistry)
		counter.Inc(1)

		totalReceived.Inc(int64(size))

		rate.addIn(peerId, size, duration)
	}

	h.metrics.outcomeMessage = func(code uint64, size int, duration time.Duration, peerId string) {
		if h.cfg.DisableMetrics {
			return
		}
		collector := metrics.GetOrRegisterCounter("bs."+msgCodeToString(code), metrics.DefaultRegistry)
		collector.Inc(int64(size))

		counter := metrics.GetOrRegisterCounter("ms."+msgCodeToString(code), metrics.DefaultRegistry)
		counter.Inc(1)

		totalSent.Inc(int64(size))

		rate.addOut(peerId, size, duration)
	}

	h.metrics.compress = func(code uint64, size int) {
		//if h.cfg.DisableMetrics {
		//	return
		//}
		//compressCnt := metrics.GetOrRegisterCounter("cd."+msgCodeToString(code), metrics.DefaultRegistry)
		//compressCnt.Inc(int64(size))
		//compressTotal.Inc(int64(size))
	}

	if !h.cfg.DisableMetrics {
		go loopLog()
		go rate.loopLog(h.log)
	}
}
