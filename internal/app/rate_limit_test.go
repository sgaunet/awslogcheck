package app

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/sgaunet/awslogcheck/internal/configapp"
	"io"
	"log/slog"
)

// TestRateLimiting tests that rate limiting is properly configured
func TestRateLimitingConfiguration(t *testing.T) {
	cfg := configapp.AppConfig{}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	
	app := New(context.Background(), cfg, aws.Config{}, 3600, logger)

	// Test that rate limiters are initialized
	if app.eventsRateLimit == nil {
		t.Error("Events rate limiter is not initialized")
	}

	if app.logGroupRateLimit == nil {
		t.Error("Log group rate limiter is not initialized")
	}

	// Test event rate limiter
	start := time.Now()
	// Try to make 30 requests quickly (should be rate limited at 25/sec)
	for i := 0; i < 30; i++ {
		if err := app.eventsRateLimit.Wait(context.Background()); err != nil {
			t.Fatalf("Rate limit wait failed: %v", err)
		}
	}
	elapsed := time.Since(start)

	// Should take more than 200ms to process 30 requests at 25/sec
	// The rate limiter uses a token bucket, so initial burst may be allowed
	if elapsed < 200*time.Millisecond {
		t.Errorf("Event rate limiting not working properly, took only %v", elapsed)
	}
	if elapsed > 3*time.Second {
		t.Errorf("Event rate limiting too slow, took %v", elapsed)
	}
}

// TestLogGroupRateLimiting tests log group rate limiting
func TestLogGroupRateLimiting(t *testing.T) {
	cfg := configapp.AppConfig{}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	
	app := New(context.Background(), cfg, aws.Config{}, 3600, logger)

	// Test log group rate limiter
	start := time.Now()
	// Try to make 15 requests quickly (should be rate limited at 10/sec)
	for i := 0; i < 15; i++ {
		if err := app.logGroupRateLimit.Wait(context.Background()); err != nil {
			t.Fatalf("Rate limit wait failed: %v", err)
		}
	}
	elapsed := time.Since(start)

	// Should take more than 500ms to process 15 requests at 10/sec
	// The rate limiter uses a token bucket, so initial burst may be allowed
	if elapsed < 500*time.Millisecond {
		t.Errorf("Log group rate limiting not working properly, took only %v", elapsed)
	}
	if elapsed > 3*time.Second {
		t.Errorf("Log group rate limiting too slow, took %v", elapsed)
	}
}

// TestContextCancellationInRateLimiter tests that rate limiter respects context cancellation
func TestContextCancellationInRateLimiter(t *testing.T) {
	cfg := configapp.AppConfig{}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	
	app := New(context.Background(), cfg, aws.Config{}, 3600, logger)

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start many requests that would take a long time
	errChan := make(chan error, 1)
	go func() {
		for i := 0; i < 100; i++ {
			if err := app.eventsRateLimit.Wait(ctx); err != nil {
				errChan <- err
				return
			}
		}
		errChan <- nil
	}()

	// Cancel the context after a short delay
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Check that we got a context cancellation error
	select {
	case err := <-errChan:
		if err == nil {
			t.Error("Expected context cancellation error but got none")
		}
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled but got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for rate limiter to respect context cancellation")
	}
}

// BenchmarkRateLimiter benchmarks the overhead of rate limiting
func BenchmarkRateLimiter(b *testing.B) {
	cfg := configapp.AppConfig{}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	
	app := New(context.Background(), cfg, aws.Config{}, 3600, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = app.eventsRateLimit.Allow()
	}
}