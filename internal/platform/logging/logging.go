// Package logging builds the application's structured logger.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// New creates a logger for the configured format and level.
func New(level, format string) *slog.Logger {
	return NewWithWriter(level, format, os.Stdout)
}

// NewWithWriter creates a logger that writes to w.
func NewWithWriter(level, format string, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	var handler slog.Handler
	if strings.EqualFold(format, "text") {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}
	return slog.New(handler)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
