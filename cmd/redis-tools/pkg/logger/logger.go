/*
Copyright 2023 The RedisOperator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logger

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
