// Package observability provides structured, redacted operational diagnostics.
package observability

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Options configures independent console and optional rotating JSONL outputs.
type Options struct {
	Level, Format, File   string
	MaxSizeMB, MaxBackups int
	Secrets               []string
	Console, Diagnostic   io.Writer
}

type loggerKey struct{}

// WithLogger binds an immutable logger to one request or background operation.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// Logger retrieves an operation logger, falling back to the application default.
func Logger(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

// HasLogger reports whether an operation logger already binds its correlation fields.
func HasLogger(ctx context.Context) bool {
	logger, ok := ctx.Value(loggerKey{}).(*slog.Logger)
	return ok && logger != nil
}

// New validates and opens operational logging outputs before the server starts.
func New(options Options) (*slog.Logger, func() error, error) {
	if options.Console == nil {
		options.Console = os.Stdout
	}
	if options.Diagnostic == nil {
		options.Diagnostic = os.Stderr
	}
	if options.Level == "" {
		options.Level = "info"
	}
	if options.Format == "" {
		options.Format = "text"
	}
	if options.MaxSizeMB == 0 {
		options.MaxSizeMB = 20
	}
	if options.MaxBackups == 0 {
		options.MaxBackups = 5
	}
	levels := map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError}
	level, ok := levels[options.Level]
	if !ok || (options.Format != "text" && options.Format != "json") || options.MaxSizeMB < 1 || options.MaxSizeMB > 1024 || options.MaxBackups < 1 || options.MaxBackups > 100 || strings.ContainsAny(options.File, "\x00\r\n") {
		return nil, nil, errors.New("invalid logging configuration")
	}
	settings := &slog.HandlerOptions{Level: level}
	var console slog.Handler = slog.NewTextHandler(options.Console, settings)
	if options.Format == "json" {
		console = slog.NewJSONHandler(options.Console, settings)
	}
	outputs := []namedHandler{{"console", console}}
	var rotating *lumberjack.Logger
	if options.File != "" {
		before, err := os.Lstat(options.File)
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, errors.New("cannot open configured log file")
		}
		if before != nil && !before.Mode().IsRegular() {
			return nil, nil, errors.New("configured log file is not a writable regular file")
		}
		file, err := os.OpenFile(options.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, errors.New("cannot open configured log file")
		}
		info, err := file.Stat()
		after, pathErr := os.Lstat(options.File)
		// Validate the opened identity before chmod, including paths first created here.
		if err != nil || !info.Mode().IsRegular() || pathErr != nil || !after.Mode().IsRegular() ||
			!os.SameFile(info, after) || (before != nil && !os.SameFile(before, info)) {
			_ = file.Close()
			return nil, nil, errors.New("configured log file is not a writable regular file")
		}
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			return nil, nil, errors.New("cannot restrict configured log file permissions")
		}
		if err := file.Close(); err != nil {
			return nil, nil, errors.New("cannot close configured log file")
		}
		rotating = &lumberjack.Logger{Filename: options.File, MaxSize: options.MaxSizeMB, MaxBackups: options.MaxBackups, Compress: false}
		outputs = append(outputs, namedHandler{"file", slog.NewJSONHandler(rotating, settings)})
	}
	report := &diagnosticWriter{output: options.Diagnostic}
	handler := &outputHandler{outputs: outputs, diagnostic: report}
	logger := slog.New(&redactingHandler{next: handler, redactor: newRedactor(options.Secrets)})
	var once sync.Once
	var closeErr error
	closeLog := func() error {
		once.Do(func() {
			if rotating != nil {
				closeErr = rotating.Close()
				if closeErr != nil {
					report.failure("file close")
				}
			}
		})
		return closeErr
	}
	return logger, closeLog, nil
}

type namedHandler struct {
	name    string
	handler slog.Handler
}
type outputHandler struct {
	outputs    []namedHandler
	diagnostic *diagnosticWriter
}
type diagnosticWriter struct {
	mu     sync.Mutex
	output io.Writer
}

// failure deliberately avoids writer error text, which can include private destinations.
func (writer *diagnosticWriter) failure(output string) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	message := "logging " + output + " output failed\n"
	if _, err := io.WriteString(writer.output, message); err != nil {
		_, _ = io.WriteString(os.Stderr, "logging diagnostic output failed\n")
	}
}

// Enabled preserves the configured severity threshold across both destinations.
func (handler *outputHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, output := range handler.outputs {
		if output.handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle attempts every destination even when a previous destination failed.
func (handler *outputHandler) Handle(ctx context.Context, record slog.Record) error {
	var failures []error
	for _, output := range handler.outputs {
		if output.handler.Enabled(ctx, record.Level) {
			if err := output.handler.Handle(ctx, record); err != nil {
				handler.diagnostic.failure(output.name)
				failures = append(failures, fmt.Errorf("logging %s failed", output.name))
			}
		}
	}
	return errors.Join(failures...)
}

// WithAttrs copies immutable output state so concurrent derived loggers remain independent.
func (handler *outputHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	outputs := make([]namedHandler, len(handler.outputs))
	for i, output := range handler.outputs {
		outputs[i] = namedHandler{output.name, output.handler.WithAttrs(attrs)}
	}
	return &outputHandler{outputs: outputs, diagnostic: handler.diagnostic}
}

// WithGroup forwards grouping consistently to both independent formats.
func (handler *outputHandler) WithGroup(name string) slog.Handler {
	outputs := make([]namedHandler, len(handler.outputs))
	for i, output := range handler.outputs {
		outputs[i] = namedHandler{output.name, output.handler.WithGroup(name)}
	}
	return &outputHandler{outputs: outputs, diagnostic: handler.diagnostic}
}
