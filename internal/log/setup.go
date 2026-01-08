package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// Format represents the log output format
type Format string

const (
	FormatText   Format = "text"
	FormatJSON   Format = "json"
	FormatLogfmt Format = "logfmt"
)

// Config holds logging configuration
type Config struct {
	// Level is the minimum log level (debug, info, warn, error)
	Level string
	// Format is the output format (text, json, logfmt)
	Format Format
	// Output is where logs are written (defaults to os.Stdout)
	Output io.Writer
}

// Setup configures the global logger based on the provided configuration.
// This should be called once at application startup.
func Setup(cfg Config) {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	level := ParseLevel(cfg.Level)
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch cfg.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(cfg.Output, opts)
	case FormatLogfmt:
		handler = NewLogfmtHandler(cfg.Output, opts)
	case FormatText, "":
		handler = NewTextHandler(cfg.Output, opts)
	default:
		fmt.Fprintf(os.Stderr, "Warning: unknown log format %q, using text\n", cfg.Format)
		handler = NewTextHandler(cfg.Output, opts)
	}

	logger := slog.New(handler)
	SetLogger(logger)
	SetLevel(level)
	slog.SetDefault(logger)
}

// ParseLevel converts a string level to slog.Level
func ParseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
