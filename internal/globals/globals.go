package globals

import (
	"context"
	"hashrouter/api"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Application = ApplicationT{
		Context: context.Background(),
	}
)

// ApplicationT TODO
type ApplicationT struct {
	Context context.Context
	Logger  zap.SugaredLogger
	Config  api.ConfigT

	//
}

// SetLogger TODO
func SetLogger(logLevel string, disableTrace bool) (err error) {
	parsedLogLevel, err := zap.ParseAtomicLevel(logLevel)
	if err != nil {
		return err
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
	logger, err := loggerConfig.Build()
	if err != nil {
		return err
	}

	Application.Logger = *logger.Sugar()
	return nil
}
