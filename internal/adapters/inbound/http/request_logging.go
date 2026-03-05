package httpapi

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/flyluman/aelora/internal/platform/config"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap/zapcore"
)

type requestLogPolicy struct {
	defaultSuccessLevel zapcore.Level
	defaultSampleEvery  int
	healthSuccessLevel  zapcore.Level
	healthSampleEvery   int
	metricsSuccessLevel zapcore.Level
	metricsSampleEvery  int
	messagePostLevel    zapcore.Level
	messagePostEvery    int
	counters            sync.Map
}

type requestLogDecision struct {
	level zapcore.Level
	log   bool
}

func newRequestLogPolicy(cfg config.Config) *requestLogPolicy {
	return &requestLogPolicy{
		defaultSuccessLevel: parseLogLevel(cfg.RequestLogDefaultSuccessLevel, zapcore.InfoLevel),
		defaultSampleEvery:  normalizeSampleEvery(cfg.RequestLogDefaultSampleEvery, 1),
		healthSuccessLevel:  parseLogLevel(cfg.RequestLogHealthSuccessLevel, zapcore.DebugLevel),
		healthSampleEvery:   normalizeSampleEvery(cfg.RequestLogHealthSampleEvery, 25),
		metricsSuccessLevel: parseLogLevel(cfg.RequestLogMetricsSuccessLevel, zapcore.DebugLevel),
		metricsSampleEvery:  normalizeSampleEvery(cfg.RequestLogMetricsSampleEvery, 50),
		messagePostLevel:    parseLogLevel(cfg.RequestLogMessageSuccessLevel, zapcore.InfoLevel),
		messagePostEvery:    normalizeSampleEvery(cfg.RequestLogMessageSampleEvery, 1),
	}
}

func (p *requestLogPolicy) decide(c echo.Context, statusCode int) requestLogDecision {
	if statusCode >= 500 {
		return requestLogDecision{level: zapcore.ErrorLevel, log: true}
	}
	if statusCode >= 400 {
		return requestLogDecision{level: zapcore.WarnLevel, log: true}
	}

	route := requestRoute(c)
	switch route {
	case "GET /livez", "GET /readyz", "GET /healthz":
		return requestLogDecision{
			level: p.healthSuccessLevel,
			log:   p.shouldSample(route, statusCode, p.healthSampleEvery),
		}
	case "GET /metrics":
		return requestLogDecision{
			level: p.metricsSuccessLevel,
			log:   p.shouldSample(route, statusCode, p.metricsSampleEvery),
		}
	case "POST /v1/rooms/:room_id/messages":
		return requestLogDecision{
			level: p.messagePostLevel,
			log:   p.shouldSample(route, statusCode, p.messagePostEvery),
		}
	default:
		return requestLogDecision{
			level: p.defaultSuccessLevel,
			log:   p.shouldSample(route, statusCode, p.defaultSampleEvery),
		}
	}
}

func (p *requestLogPolicy) shouldSample(route string, statusCode, sampleEvery int) bool {
	if sampleEvery <= 1 {
		return true
	}
	key := route + "|" + strconv.Itoa(statusCode)
	counter, _ := p.counters.LoadOrStore(key, &atomic.Uint64{})
	return counter.(*atomic.Uint64).Add(1)%uint64(sampleEvery) == 1
}

func normalizeSampleEvery(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func parseLogLevel(raw string, fallback zapcore.Level) zapcore.Level {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(raw))); err != nil {
		return fallback
	}
	return level
}
