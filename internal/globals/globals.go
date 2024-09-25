package globals

import (
	"context"
	"hashrouter/api"
	"hashrouter/internal/proxy"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Application = ApplicationT{
		Context:   context.Background(),
		ProxyPool: make(map[string]*proxy.ProxyT),
	}
)

// ApplicationT TODO
type ApplicationT struct {
	Context context.Context
	Config  api.ConfigT

	// ProxyPool represents a global pool of pointers to all the proxies.
	// This will allow access to their properties later,
	// for checking their status and exposing their 'health' and 'readiness' using only one shared webserver.
	ProxyPool map[string]*proxy.ProxyT
}

// SetLogger TODO
func GetLogger(logLevel string, disableTrace bool) (logger *zap.SugaredLogger, err error) {
	parsedLogLevel, err := zap.ParseAtomicLevel(logLevel)
	if err != nil {
		return logger, err
	}

	// Initialize the logger
	loggerConfig := zap.NewProductionConfig()
	if disableTrace {
		loggerConfig.DisableStacktrace = true
		loggerConfig.DisableCaller = true
	}

	loggerConfig.EncoderConfig.TimeKey = "timestamp"
	loggerConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)
	loggerConfig.Level.SetLevel(parsedLogLevel.Level())

	// Configure the logger
	loggerObj, err := loggerConfig.Build()
	if err != nil {
		return logger, err
	}

	return loggerObj.Sugar(), nil
}
