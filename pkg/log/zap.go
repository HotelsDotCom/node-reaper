/*
Copyright 2018 Expedia Inc.

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

package log

import (
	"bytes"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ZapLevelMapping maps Zap log levels to node-reaper log levels
var ZapLevelMapping = map[Level]zapcore.Level{
	INFO:  zap.InfoLevel,
	DEBUG: zap.DebugLevel,
}

// Zap provides a sugared logger implementation based on zap
type Zap struct {
	logger        *zap.SugaredLogger
	cfg           *zap.Config
	prependString func(string) string
	prependSlice  func([]interface{}) []interface{}
}

// New creates an opinionated zap logger
func (z Zap) New(level Level) (Logger, error) {
	encoderCfg := zapcore.EncoderConfig{
		// Keys can be anything except an empty string.
		TimeKey:        "T",
		LevelKey:       "L",
		NameKey:        "N",
		MessageKey:     "M",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	cfg := zap.Config{
		Level:       zap.NewAtomicLevelAt(ZapLevelMapping[level]),
		Development: false,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:         "console",
		EncoderConfig:    encoderCfg,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, _ := cfg.Build()
	z.cfg = &cfg
	z.logger = logger.Sugar()
	z.prependString = func(s string) string {
		return s
	}
	z.prependSlice = func(s []interface{}) []interface{} {
		return s
	}

	return &z, nil
}

// NewWithConfig creates a zap logger with the provided zap config
func (z Zap) NewWithConfig(cfg zap.Config) (Logger, error) {
	logger, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	z.cfg = &cfg
	z.logger = logger.Sugar()

	return &z, nil
}

var prependBuffer bytes.Buffer

// WithFields returns a clone of the logger which logs with prepended fields
func (z Zap) WithFields(fields []Field, prefix string, suffix string) Logger {
	z.prependString = func(s string) string {
		if len(fields) == 0 {
			return ""
		}
		prependBuffer.Reset()
		for _, f := range fields {
			prependBuffer.WriteString(prefix)
			prependBuffer.WriteString(fmt.Sprintf(fmt.Sprintf("%%-%ds", f.Width), f.Field))
			prependBuffer.WriteString(suffix)
		}
		prependBuffer.WriteString(s)
		return prependBuffer.String()
	}
	z.prependSlice = func(args []interface{}) []interface{} {
		return append([]interface{}{z.prependString("")}, args...)
	}
	return &z
}

// Debug logs with level Debug
func (z *Zap) Debug(args ...interface{}) {
	z.logger.Debug(z.prependSlice(args)...)
}

// Debugf formats and logs with level Debug
func (z *Zap) Debugf(template string, args ...interface{}) {
	z.logger.Debugf(z.prependString(template), args...)
}

// Info logs with level Info
func (z *Zap) Info(args ...interface{}) {
	z.logger.Info(z.prependSlice(args)...)
}

// Infof formats and logs with level Info
func (z *Zap) Infof(template string, args ...interface{}) {
	z.logger.Infof(z.prependString(template), args...)
}

// Warn logs with level Warn
func (z *Zap) Warn(args ...interface{}) {
	z.logger.Warn(z.prependSlice(args)...)
}

// Warnf formats and logs with level Warn
func (z *Zap) Warnf(template string, args ...interface{}) {
	z.logger.Warnf(z.prependString(template), args...)
}

// Warningf formats and logs with level Warn
func (z *Zap) Warningf(template string, args ...interface{}) {
	z.logger.Warnf(z.prependString(template), args...)
}

// Error logs with level Error
func (z *Zap) Error(args ...interface{}) {
	z.logger.Error(z.prependSlice(args)...)
}

// Errorf formats and logs with level Error
func (z *Zap) Errorf(template string, args ...interface{}) {
	z.logger.Errorf(z.prependString(template), args...)
}
