package app

import (
	"io"
	"log/slog"

	"golang.org/x/time/rate"
)

// Test utilities for internal testing

func createTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

func createTestRateLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Limit(100), 100) // High limit for tests
}