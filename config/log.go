package config

import (
	"fmt"
	"os"
	"time"

	zaplogfmt "github.com/jsternberg/zap-logfmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewRootLogger creates a new logger object with the given format and log level
// (copied from https://github.com/cosmos/relayer/blob/v2.4.2/cmd/root.go#L174-L202)
func NewRootLogger(format string, debug bool) (*zap.Logger, error) {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = func(ts time.Time, encoder zapcore.PrimitiveArrayEncoder) {
		encoder.AppendString(ts.UTC().Format("2006-01-02T15:04:05.000000Z07:00"))
	}
	config.LevelKey = "lvl"

	var enc zapcore.Encoder
	switch format {
	case "json":
		enc = zapcore.NewJSONEncoder(config)
	case "auto", "console":
		enc = zapcore.NewConsoleEncoder(config)
	case "logfmt":
		enc = zaplogfmt.NewEncoder(config)
	default:
		return nil, fmt.Errorf("unrecognized log format %q", format)
	}

	level := zap.InfoLevel
	if debug {
		level = zap.DebugLevel
	}

	return zap.New(zapcore.NewCore(
		enc,
		os.Stderr,
		level,
	)), nil
}
