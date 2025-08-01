package app

import (
	"context"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/sgaunet/awslogcheck/internal/configapp"
	"io"
	"log/slog"
)

// TestIsLineMatchWithOneRule tests the rule matching functionality
func TestIsLineMatchWithOneRule(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		rules       []string
		expectMatch bool
	}{
		{
			name:        "Match DEBUG log",
			line:        "DEBUG: This is a debug message",
			rules:       []string{"^DEBUG:", "^INFO:"},
			expectMatch: true,
		},
		{
			name:        "Match INFO log",
			line:        "INFO: Application started",
			rules:       []string{"^DEBUG:", "^INFO:"},
			expectMatch: true,
		},
		{
			name:        "No match",
			line:        "ERROR: Something went wrong",
			rules:       []string{"^DEBUG:", "^INFO:"},
			expectMatch: false,
		},
		{
			name:        "Complex regex match",
			line:        "2024-01-01 10:00:00 [TRACE] Request processed",
			rules:       []string{".*\\[TRACE\\].*"},
			expectMatch: true,
		},
		{
			name:        "Empty rules",
			line:        "Any log line",
			rules:       []string{},
			expectMatch: false,
		},
		{
			name:        "Invalid regex",
			line:        "Test log",
			rules:       []string{"[invalid(regex"},
			expectMatch: false, // Should not panic, just not match
		},
	}

	// Create a test app instance
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	app := New(context.Background(), configapp.AppConfig{}, aws.Config{}, 3600, logger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := app.isLineMatchWithOneRule(tt.line, tt.rules)
			if result != tt.expectMatch {
				t.Errorf("Expected %v but got %v", tt.expectMatch, result)
			}
		})
	}
}

// BenchmarkIsLineMatchWithOneRule benchmarks rule matching
func BenchmarkIsLineMatchWithOneRule(b *testing.B) {
	line := "2024-01-01 10:00:00 ERROR: Database connection failed with timeout after 30 seconds"
	rules := []string{
		"^DEBUG:",
		"^INFO:",
		"^TRACE:",
		".*connection failed.*",
		".*timeout.*",
		"\\d{4}-\\d{2}-\\d{2}",
		"ERROR:.*database.*",
	}

	// Create a test app instance
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	app := New(context.Background(), configapp.AppConfig{}, aws.Config{}, 3600, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = app.isLineMatchWithOneRule(line, rules)
	}
}

// BenchmarkRegexpCompile benchmarks the cost of compiling regexes
func BenchmarkRegexpCompile(b *testing.B) {
	patterns := []string{
		"^DEBUG:",
		"^INFO:",
		".*\\[TRACE\\].*",
		"\\d{4}-\\d{2}-\\d{2} \\d{2}:\\d{2}:\\d{2}",
		"ERROR:.*database.*",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, pattern := range patterns {
			_, _ = regexp.Compile(pattern)
		}
	}
}

// Additional test cases can be added here for other helper functions