package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// slogAdapter adapts slog.Logger to goproxy's Logger interface.
// goproxy.Logger requires only Printf(format string, v ...any).
type slogAdapter struct {
	logger *slog.Logger
}

// Printf implements goproxy.Logger interface.
// It parses goproxy's log format and converts to structured slog messages.
func (a *slogAdapter) Printf(format string, v ...any) {
	// Format the message
	msg := fmt.Sprintf(format, v...)

	// Determine log level from message content
	// goproxy logs are typically formatted as "[%03d] INFO: %s" or "[%03d] WARN: %s"
	ctx := context.Background()
	switch {
	case strings.Contains(msg, "WARN:"):
		// Extract the actual message after "WARN: "
		if idx := strings.Index(msg, "WARN: "); idx >= 0 {
			msg = msg[idx+6:]
		}
		a.logger.WarnContext(ctx, "goproxy: "+msg)
	case strings.Contains(msg, "ERROR:"):
		if idx := strings.Index(msg, "ERROR: "); idx >= 0 {
			msg = msg[idx+7:]
		}
		a.logger.ErrorContext(ctx, "goproxy: "+msg)
	case strings.Contains(msg, "INFO:"):
		if idx := strings.Index(msg, "INFO: "); idx >= 0 {
			msg = msg[idx+6:]
		}
		a.logger.InfoContext(ctx, "goproxy: "+msg)
	default:
		// Default to debug for other messages
		a.logger.DebugContext(ctx, "goproxy: "+msg)
	}
}
