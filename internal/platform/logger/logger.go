package logger

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	mu          sync.Mutex
	closeSIGHUP func()
)

func New(serviceName, appEnv, level string) *zap.Logger {
	cfg := productionConfig()

	if err := cfg.Level.UnmarshalText([]byte(level)); err != nil {
		cfg.Level.SetLevel(zap.InfoLevel)
	}

	log, err := cfg.Build(zap.AddStacktrace(zap.ErrorLevel))
	if err != nil {
		fallback, fallbackErr := productionConfig().Build(zap.AddStacktrace(zap.ErrorLevel))
		if fallbackErr != nil {
			return zap.NewNop().With(zap.String("service", serviceName), zap.String("env", appEnv))
		}
		return fallback.With(zap.String("service", serviceName), zap.String("env", appEnv))
	}

	atomicLevel := cfg.Level

	mu.Lock()
	if closeSIGHUP == nil {
		shutdownCh := make(chan struct{})
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGHUP)
		closeSIGHUP = func() {
			signal.Stop(sigCh)
			close(shutdownCh)
		}
		go func() {
			for {
				select {
				case <-sigCh:
					newLevel := os.Getenv("AELORA_LOG_LEVEL")
					if newLevel != "" {
						var l zapcore.Level
						if err := l.UnmarshalText([]byte(newLevel)); err == nil {
							atomicLevel.SetLevel(l)
						}
					}
				case <-shutdownCh:
					return
				}
			}
		}()
	}
	mu.Unlock()

	return log.With(zap.String("service", serviceName), zap.String("env", appEnv))
}

func Shutdown() {
	mu.Lock()
	defer mu.Unlock()
	if closeSIGHUP != nil {
		closeSIGHUP()
		closeSIGHUP = nil
	}
}

func productionConfig() zap.Config {
	cfg := zap.NewProductionConfig()
	cfg.Encoding = "json"
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339Nano)
	cfg.EncoderConfig.MessageKey = "msg"
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.CallerKey = "caller"
	cfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	cfg.DisableCaller = false
	return cfg
}
