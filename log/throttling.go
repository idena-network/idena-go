package log

import (
	"github.com/patrickmn/go-cache"
	"time"
)

type ThrottlingLogger interface {
	Trace(msg string, ctx ...interface{})
	Debug(msg string, ctx ...interface{})
	Info(msg string, ctx ...interface{})
	Warn(msg string, ctx ...interface{})
	Error(msg string, ctx ...interface{})
	Crit(msg string, ctx ...interface{})
}

func NewThrottlingLogger(baseLogger Logger) ThrottlingLogger {
	res := &throttlingLogger{
		logger: baseLogger,
		cache:  cache.New(time.Minute, time.Minute*5),
	}
	return res
}

type throttlingLogger struct {
	logger Logger
	cache  *cache.Cache
}

func (t *throttlingLogger) Trace(msg string, ctx ...interface{}) {
	t.logIfNeeded(msg, t.logger.Trace, ctx...)
}

func (t *throttlingLogger) Debug(msg string, ctx ...interface{}) {
	t.logIfNeeded(msg, t.logger.Debug, ctx...)
}

func (t *throttlingLogger) Info(msg string, ctx ...interface{}) {
	t.logIfNeeded(msg, t.logger.Info, ctx...)
}

func (t *throttlingLogger) Warn(msg string, ctx ...interface{}) {
	t.logIfNeeded(msg, t.logger.Warn, ctx...)
}

func (t *throttlingLogger) Error(msg string, ctx ...interface{}) {
	t.logIfNeeded(msg, t.logger.Error, ctx...)
}

func (t *throttlingLogger) Crit(msg string, ctx ...interface{}) {
	t.logIfNeeded(msg, t.logger.Crit, ctx...)
}

func (t *throttlingLogger) logIfNeeded(msg string, log func(msg string, ctx ...interface{}), ctx ...interface{}) {
	if err := t.cache.Add(msg, struct{}{}, cache.DefaultExpiration); err == nil {
		log(msg, ctx...)
	}
}
