package logging

import (
	"context"
	"log/slog"
)

type ctxKey string

const loggerKey ctxKey = "slog_logger"

// WithLogger puts a customized logger into context
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext pulls the logger out of the context or gives slog.Default() if no logger exists.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
