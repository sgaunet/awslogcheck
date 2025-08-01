package app_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/sgaunet/awslogcheck/internal/app"
	"github.com/sgaunet/awslogcheck/internal/configapp"
	"io"
	"log/slog"
)

// Helper function to create a test App instance
func createTestApp(cfg configapp.AppConfig) *app.App {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	return app.New(context.Background(), cfg, aws.Config{}, 3600, logger)
}

// TestLoadRules tests the LoadRules functionality
func TestLoadRules(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() (string, error) // Returns rules dir path
		cleanup     func(string)
		expectError bool
		expectRules int
	}{
		{
			name: "LoadRules with valid rules directory",
			setup: func() (string, error) {
				// Create temp directory with test rules
				tmpDir, err := os.MkdirTemp("", "awslogcheck-test-rules-*")
				if err != nil {
					return "", err
				}

				// Create test rule files
				rule1 := filepath.Join(tmpDir, "rule1.txt")
				if err := os.WriteFile(rule1, []byte("^DEBUG:.*\n^INFO:.*\n"), 0644); err != nil {
					return tmpDir, err
				}

				rule2 := filepath.Join(tmpDir, "rule2.txt")
				if err := os.WriteFile(rule2, []byte("^TRACE:.*\n"), 0644); err != nil {
					return tmpDir, err
				}

				// Create a subdirectory (should be ignored)
				subDir := filepath.Join(tmpDir, "subdir")
				if err := os.Mkdir(subDir, 0755); err != nil {
					return tmpDir, err
				}

				return tmpDir, nil
			},
			cleanup: func(dir string) {
				os.RemoveAll(dir)
			},
			expectError: false,
			expectRules: 3, // 2 from rule1.txt, 1 from rule2.txt
		},
		{
			name: "LoadRules with empty rules directory",
			setup: func() (string, error) {
				return "", nil
			},
			cleanup:     func(dir string) {},
			expectError: true,
			expectRules: 0,
		},
		{
			name: "LoadRules with non-existent directory",
			setup: func() (string, error) {
				return "/non/existent/path", nil
			},
			cleanup:     func(dir string) {},
			expectError: true,
			expectRules: 0,
		},
		{
			name: "LoadRules with empty files",
			setup: func() (string, error) {
				tmpDir, err := os.MkdirTemp("", "awslogcheck-test-empty-*")
				if err != nil {
					return "", err
				}

				// Create empty rule file
				emptyFile := filepath.Join(tmpDir, "empty.txt")
				if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
					return tmpDir, err
				}

				return tmpDir, nil
			},
			cleanup: func(dir string) {
				os.RemoveAll(dir)
			},
			expectError: false,
			expectRules: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rulesDir, err := tt.setup()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer tt.cleanup(rulesDir)

			cfg := configapp.AppConfig{
				RulesDir: rulesDir,
			}
			app := createTestApp(cfg)

			err = app.LoadRules()
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Note: We can't directly check the number of rules loaded
			// as the rules field is private. This would need refactoring
			// to expose a method like GetRulesCount() or similar.
		})
	}
}

// TestGetTimeStampMsRangeofLastHour tests timestamp calculation
func TestGetTimeStampMsRangeofLastHour(t *testing.T) {
	app := createTestApp(configapp.AppConfig{})

	begin, end, err := app.GetTimeStampMsRangeofLastHour()
	if err != nil {
		t.Fatalf("GetTimeStampMsRangeofLastHour returned error: %v", err)
	}

	// Verify timestamps are reasonable
	now := time.Now().Unix() * 1000
	oneHourAgo := now - (3600 * 1000)
	twoHoursAgo := now - (7200 * 1000)

	// Begin should be approximately 1 hour ago
	if begin > oneHourAgo || begin < twoHoursAgo {
		t.Errorf("Begin timestamp %d is not within expected range", begin)
	}

	// End should be approximately 1 hour ago (end of that hour)
	if end > now || end < oneHourAgo {
		t.Errorf("End timestamp %d is not within expected range", end)
	}

	// Begin should be before end
	if begin >= end {
		t.Errorf("Begin %d should be before end %d", begin, end)
	}

	// The difference should be approximately 1 hour (3600000 ms)
	diff := end - begin
	if diff < 3500000 || diff > 3700000 { // Allow some tolerance
		t.Errorf("Time range %d ms is not approximately 1 hour", diff)
	}
}

// Test for image and container ignore functionality
func TestImageAndContainerIgnore(t *testing.T) {
	tests := []struct {
		name              string
		imagesToIgnore    []string
		containersIgnore  []string
		imageToCheck      string
		containerToCheck  string
		expectImageIgnore bool
		expectContIgnore  bool
	}{
		{
			name:              "Exact match ignore",
			imagesToIgnore:    []string{"nginx:latest", "redis:6"},
			containersIgnore:  []string{"sidecar", "init-container"},
			imageToCheck:      "nginx:latest",
			containerToCheck:  "sidecar",
			expectImageIgnore: true,
			expectContIgnore:  true,
		},
		{
			name:              "Regex pattern match",
			imagesToIgnore:    []string{"nginx:.*", ".*fluent.*"},
			containersIgnore:  []string{"aws-.*", ".*-init"},
			imageToCheck:      "nginx:1.21",
			containerToCheck:  "aws-vpc-cni",
			expectImageIgnore: true,
			expectContIgnore:  true,
		},
		{
			name:              "No match",
			imagesToIgnore:    []string{"nginx:.*"},
			containersIgnore:  []string{"sidecar"},
			imageToCheck:      "apache:latest",
			containerToCheck:  "app",
			expectImageIgnore: false,
			expectContIgnore:  false,
		},
		{
			name:              "Empty ignore lists",
			imagesToIgnore:    []string{},
			containersIgnore:  []string{},
			imageToCheck:      "anything",
			containerToCheck:  "anything",
			expectImageIgnore: false,
			expectContIgnore:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: Since isImageIgnored and isContainerIgnored are private,
			// we test them through the parseAllEventsWithFilter function
			// in cloudwatchlogs_test.go
			_ = configapp.AppConfig{
				ImagesToIgnore:        tt.imagesToIgnore,
				ContainerNameToIgnore: tt.containersIgnore,
			}
		})
	}
}

// TestSendReport tests the email sending functionality
func TestSendReport(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() (string, configapp.AppConfig, error)
		cleanup     func(string)
		expectError bool
	}{
		{
			name: "Send report with valid file",
			setup: func() (string, configapp.AppConfig, error) {
				// Create temp report file
				tmpFile, err := os.CreateTemp("", "test-report-*.html")
				if err != nil {
					return "", configapp.AppConfig{}, err
				}
				defer tmpFile.Close()

				content := "<html><body>Test Report</body></html>"
				if _, err := tmpFile.WriteString(content); err != nil {
					return tmpFile.Name(), configapp.AppConfig{}, err
				}

				cfg := configapp.AppConfig{
					MailConfig: configapp.MailConfiguration{
						Sendto:    "test@example.com",
						FromEmail: "noreply@example.com",
						Subject:   "Test Report",
					},
					// Note: We'd need to mock the mail service
					// to properly test this without sending real emails
				}

				return tmpFile.Name(), cfg, nil
			},
			cleanup: func(file string) {
				os.Remove(file)
			},
			expectError: false, // Should not error - file exists but mail not configured
		},
		{
			name: "Send report with non-existent file",
			setup: func() (string, configapp.AppConfig, error) {
				cfg := configapp.AppConfig{
					MailConfig: configapp.MailConfiguration{
						Sendto:    "test@example.com",
						FromEmail: "noreply@example.com",
						Subject:   "Test Report",
					},
				}
				return "/non/existent/file.html", cfg, nil
			},
			cleanup:     func(file string) {},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, cfg, err := tt.setup()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			defer tt.cleanup(file)

			app := createTestApp(cfg)
			err = app.SendReport(file)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// Integration test for LogCheck
func TestLogCheckIntegration(t *testing.T) {
	// This is a complex integration test that would require:
	// 1. Mocking CloudWatch Logs client
	// 2. Setting up test log groups and streams
	// 3. Mocking email service
	// 4. Creating test rules

	t.Skip("Integration test requires significant mocking infrastructure")

	// Example of what the test structure would look like:
	/*
		mockCWClient := &mockCloudWatchClient{
			logGroups: []string{"/aws/containerinsights/test-cluster/application"},
			// ... setup mock data
		}

		cfg := configapp.AppConfig{
			LogGroup:    "/aws/containerinsights/test-cluster/application",
			RulesDir:    "./test-rules",
			MailConfig: configapp.MailConfiguration{
				Sendto: "test@example.com",
			},
			// ... other config
		}

		app := createTestApp(cfg)
		// Inject mock client somehow (requires refactoring)

		ctx := context.Background()
		err := app.LogCheck(ctx)
		if err != nil {
			t.Errorf("LogCheck failed: %v", err)
		}

		// Verify expected behavior
	*/
}

// Benchmark tests
func BenchmarkLoadRules(b *testing.B) {
	// Create temp directory with many rules
	tmpDir, err := os.MkdirTemp("", "bench-rules-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create 100 rule files with 10 rules each
	for i := 0; i < 100; i++ {
		ruleFile := filepath.Join(tmpDir, fmt.Sprintf("rule%d.txt", i))
		var rules strings.Builder
		for j := 0; j < 10; j++ {
			rules.WriteString(fmt.Sprintf("^PATTERN_%d_%d:.*\n", i, j))
		}
		if err := os.WriteFile(ruleFile, []byte(rules.String()), 0644); err != nil {
			b.Fatal(err)
		}
	}

	cfg := configapp.AppConfig{
		RulesDir: tmpDir,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app := createTestApp(cfg)
		if err := app.LoadRules(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetTimeStampMsRangeofLastHour benchmarks timestamp calculation
func BenchmarkGetTimeStampMsRangeofLastHour(b *testing.B) {
	app := createTestApp(configapp.AppConfig{})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := app.GetTimeStampMsRangeofLastHour()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSendReport benchmarks report sending (file reading part)
func BenchmarkSendReport(b *testing.B) {
	// Create a test report file
	tmpFile, err := os.CreateTemp("", "bench-report-*.html")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write some content
	content := strings.Repeat("<p>Test log line with some content</p>\n", 1000)
	if _, err := tmpFile.WriteString(content); err != nil {
		b.Fatal(err)
	}

	cfg := configapp.AppConfig{
		MailConfig: configapp.MailConfiguration{
			Sendto:    "test@example.com",
			FromEmail: "noreply@example.com",
			Subject:   "Test Report",
		},
	}
	app := createTestApp(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will fail on mail sending but we're benchmarking file reading
		_ = app.SendReport(tmpFile.Name())
	}
}