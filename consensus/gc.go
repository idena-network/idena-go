package consensus

import (
	"fmt"
	"github.com/idena-network/idena-go/common/math"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/events"
	"time"
)

func (engine *Engine) maybeIpfsGC() {

	if !engine.cfg.IpfsConf.Gc.Enabled {
		return
	}

	start := time.Now().UTC()

	if start.Before(engine.nextIpfsGC) {
		return
	}

	defer func() {
		engine.nextIpfsGC = start.Add(engine.cfg.IpfsConf.Gc.Interval)
		engine.log.Info(fmt.Sprintf("ipfs gc next time: %v", engine.nextIpfsGC))
	}()

	if engine.appState.State.ValidationPeriod() != state.NonePeriod {
		engine.log.Info(fmt.Sprintf("ipfs gc skipped due to validation"))
		return
	}

	deadline := engine.appState.State.NextValidationTime().Add(-engine.cfg.IpfsConf.Gc.IntervalBeforeValidation)

	if start.After(deadline) {
		engine.log.Info(fmt.Sprintf("ipfs gc skipped due to soon validation"))
		return
	}

	timeout := time.Duration(math.Min(uint64(engine.cfg.IpfsConf.Gc.Timeout), uint64(deadline.Sub(start))))
	engine.log.Info("ipfs gc starting")
	ctx, cancel := engine.ipfsProxy.GC()

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(engine.cfg.IpfsConf.Gc.NotificationDelay):
			message := func(minutes int) string {
				message := fmt.Sprintf("IPFS database compacting: it may take up to %v minute", minutes)
				if minutes > 1 {
					message += "s"
				}
				return message
			}
			minutesLeft := int((timeout - time.Since(start)).Minutes()) + 1
			engine.eventBus.Publish(&events.IpfsGcEvent{Message: message(minutesLeft)})
			ticker := time.NewTicker(time.Minute)
			for {
				select {
				case <-ctx.Done():
					engine.eventBus.Publish(&events.IpfsGcEvent{Completed: true})
					return
				case <-ticker.C:
					if minutesLeft > 1 {
						minutesLeft--
					}
					engine.eventBus.Publish(&events.IpfsGcEvent{Message: message(minutesLeft)})
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		engine.log.Info("ipfs gc completed", "d", time.Since(start))
	case <-time.After(timeout):
		cancel()
		engine.log.Info("ipfs gc cancelled due to timeout", "d", time.Since(start))
	}
}
