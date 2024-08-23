package util

import (
	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func NewLogger(c *cli.Context) logr.Logger {
	var (
		level, _    = zapcore.ParseLevel(c.String("zap-log-level"))
		timeEncoder zapcore.TimeEncoder
		dev         = false
	)
	if c != nil {
		dev = c.Bool("zap-devel")
		if err := timeEncoder.UnmarshalText([]byte(c.String("zap-time-encoding"))); err != nil {
			panic(err)
		}
	}

	opts := &zap.Options{
		Development:     dev,
		Level:           level,
		TimeEncoder:     timeEncoder,
		StacktraceLevel: zapcore.PanicLevel,
	}
	return logr.New(zap.New(zap.UseFlagOptions(opts)).GetSink())
}
