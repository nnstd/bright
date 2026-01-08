package raft

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/hashicorp/go-hclog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// zapHclogAdapter adapts a zap logger to the hclog.Logger interface
// used by hashicorp/raft
type zapHclogAdapter struct {
	logger *zap.Logger
	name   string
	args   []interface{}
}

// NewHclogAdapter creates an hclog.Logger that writes to a zap logger
func NewHclogAdapter(logger *zap.Logger, name string) hclog.Logger {
	return &zapHclogAdapter{
		logger: logger.Named(name),
		name:   name,
		args:   []interface{}{},
	}
}

// Helper function to convert args to zap fields
func (z *zapHclogAdapter) argsToFields(args []interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(args)/2)

	// Combine instance args with call args
	allArgs := append(z.args, args...)

	for i := 0; i < len(allArgs)-1; i += 2 {
		key, ok := allArgs[i].(string)
		if !ok {
			continue
		}

		value := allArgs[i+1]

		// Convert value to appropriate zap field
		switch v := value.(type) {
		case string:
			fields = append(fields, zap.String(key, v))
		case int:
			fields = append(fields, zap.Int(key, v))
		case int64:
			fields = append(fields, zap.Int64(key, v))
		case uint64:
			fields = append(fields, zap.Uint64(key, v))
		case bool:
			fields = append(fields, zap.Bool(key, v))
		case error:
			fields = append(fields, zap.Error(v))
		default:
			fields = append(fields, zap.Any(key, v))
		}
	}

	return fields
}

// Log implements hclog.Logger
func (z *zapHclogAdapter) Log(level hclog.Level, msg string, args ...interface{}) {
	fields := z.argsToFields(args)

	switch level {
	case hclog.Trace, hclog.Debug:
		z.logger.Debug(msg, fields...)
	case hclog.Info:
		z.logger.Info(msg, fields...)
	case hclog.Warn:
		z.logger.Warn(msg, fields...)
	case hclog.Error:
		z.logger.Error(msg, fields...)
	default:
		z.logger.Info(msg, fields...)
	}
}

// Trace implements hclog.Logger
func (z *zapHclogAdapter) Trace(msg string, args ...interface{}) {
	z.logger.Debug(msg, z.argsToFields(args)...)
}

// Debug implements hclog.Logger
func (z *zapHclogAdapter) Debug(msg string, args ...interface{}) {
	z.logger.Debug(msg, z.argsToFields(args)...)
}

// Info implements hclog.Logger
func (z *zapHclogAdapter) Info(msg string, args ...interface{}) {
	z.logger.Info(msg, z.argsToFields(args)...)
}

// Warn implements hclog.Logger
func (z *zapHclogAdapter) Warn(msg string, args ...interface{}) {
	z.logger.Warn(msg, z.argsToFields(args)...)
}

// Error implements hclog.Logger
func (z *zapHclogAdapter) Error(msg string, args ...interface{}) {
	z.logger.Error(msg, z.argsToFields(args)...)
}

// IsTrace implements hclog.Logger
func (z *zapHclogAdapter) IsTrace() bool {
	return z.logger.Core().Enabled(zapcore.DebugLevel)
}

// IsDebug implements hclog.Logger
func (z *zapHclogAdapter) IsDebug() bool {
	return z.logger.Core().Enabled(zapcore.DebugLevel)
}

// IsInfo implements hclog.Logger
func (z *zapHclogAdapter) IsInfo() bool {
	return z.logger.Core().Enabled(zapcore.InfoLevel)
}

// IsWarn implements hclog.Logger
func (z *zapHclogAdapter) IsWarn() bool {
	return z.logger.Core().Enabled(zapcore.WarnLevel)
}

// IsError implements hclog.Logger
func (z *zapHclogAdapter) IsError() bool {
	return z.logger.Core().Enabled(zapcore.ErrorLevel)
}

// ImpliedArgs implements hclog.Logger
func (z *zapHclogAdapter) ImpliedArgs() []interface{} {
	return z.args
}

// With implements hclog.Logger
func (z *zapHclogAdapter) With(args ...interface{}) hclog.Logger {
	return &zapHclogAdapter{
		logger: z.logger,
		name:   z.name,
		args:   append(z.args, args...),
	}
}

// Name implements hclog.Logger
func (z *zapHclogAdapter) Name() string {
	return z.name
}

// Named implements hclog.Logger
func (z *zapHclogAdapter) Named(name string) hclog.Logger {
	newName := z.name
	if newName != "" {
		newName = newName + "." + name
	} else {
		newName = name
	}

	return &zapHclogAdapter{
		logger: z.logger.Named(name),
		name:   newName,
		args:   z.args,
	}
}

// ResetNamed implements hclog.Logger
func (z *zapHclogAdapter) ResetNamed(name string) hclog.Logger {
	// Get the base logger without any names
	baseLogger := z.logger
	for strings.Contains(z.name, ".") {
		baseLogger = baseLogger.WithOptions(zap.WithCaller(false))
	}

	return &zapHclogAdapter{
		logger: baseLogger.Named(name),
		name:   name,
		args:   []interface{}{},
	}
}

// SetLevel implements hclog.Logger
func (z *zapHclogAdapter) SetLevel(level hclog.Level) {
	// zap loggers don't support runtime level changes in this way
	// This is a no-op in our implementation
}

// GetLevel implements hclog.Logger
func (z *zapHclogAdapter) GetLevel() hclog.Level {
	if z.logger.Core().Enabled(zapcore.DebugLevel) {
		return hclog.Debug
	}
	if z.logger.Core().Enabled(zapcore.InfoLevel) {
		return hclog.Info
	}
	if z.logger.Core().Enabled(zapcore.WarnLevel) {
		return hclog.Warn
	}
	return hclog.Error
}

// StandardLogger implements hclog.Logger
func (z *zapHclogAdapter) StandardLogger(opts *hclog.StandardLoggerOptions) *log.Logger {
	return log.New(&zapWriter{logger: z.logger}, "", 0)
}

// StandardWriter implements hclog.Logger
func (z *zapHclogAdapter) StandardWriter(opts *hclog.StandardLoggerOptions) io.Writer {
	return &zapWriter{logger: z.logger}
}

// zapWriter implements io.Writer to redirect standard logger output to zap
type zapWriter struct {
	logger *zap.Logger
}

func (w *zapWriter) Write(p []byte) (n int, err error) {
	w.logger.Info(strings.TrimSpace(string(p)))
	return len(p), nil
}

// Helper methods for formatted logging

func (z *zapHclogAdapter) format(msg string, args ...interface{}) string {
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}
