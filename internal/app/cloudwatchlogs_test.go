package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/sgaunet/awslogcheck/internal/configapp"
	"io"
	"log/slog"
	"golang.org/x/time/rate"
)

// mockCloudWatchClient is a mock implementation of the CloudWatch Logs client
type mockCloudWatchClient struct {
	events []types.FilteredLogEvent
	pageSize int
	callCount int
}

func (m *mockCloudWatchClient) FilterLogEvents(ctx context.Context, params *cloudwatchlogs.FilterLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	// Simple pagination simulation
	start := m.callCount * m.pageSize
	end := start + m.pageSize
	
	if start >= len(m.events) {
		return &cloudwatchlogs.FilterLogEventsOutput{
			Events: []types.FilteredLogEvent{},
		}, nil
	}
	
	if end > len(m.events) {
		end = len(m.events)
	}
	
	pageEvents := m.events[start:end]
	
	var nextToken *string
	if end < len(m.events) {
		token := fmt.Sprintf("token-%d", m.callCount+1)
		nextToken = &token
	}
	
	m.callCount++
	
	return &cloudwatchlogs.FilterLogEventsOutput{
		Events:    pageEvents,
		NextToken: nextToken,
	}, nil
}

// Helper function to create a CloudWatch log event
func createLogEvent(timestamp int64, streamName, podName, containerImage, containerName, logMessage string) types.FilteredLogEvent {
	message := fluentDockerLog{
		Log: logMessage,
		Kubernetes: kubernetesInfos{
			PodName:        podName,
			ContainerImage: containerImage,
			ContainerName:  containerName,
			NamespaceName:  "default",
		},
	}
	
	messageJSON, _ := json.Marshal(message)
	messageStr := string(messageJSON)
	
	return types.FilteredLogEvent{
		Timestamp:     &timestamp,
		Message:       &messageStr,
		LogStreamName: &streamName,
	}
}

func TestParseAllEventsWithFilter_MultiplePodsSameContainer(t *testing.T) {
	// Create test app
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	
	app := &App{
		cfg:               configapp.AppConfig{},
		awscfg:            aws.Config{},
		rules:             []string{}, // No rules, so all logs will be included
		lastPeriodToWatch: 3600,
		appLog:            logger,
		eventsRateLimit:   rate.NewLimiter(rate.Limit(25), 25),
		logGroupRateLimit: rate.NewLimiter(rate.Limit(10), 10),
	}
	
	// Create mock events from two different pods with the same container
	now := time.Now().Unix() * 1000
	events := []types.FilteredLogEvent{
		// Pod A events
		createLogEvent(now-300000, "stream-pod-a", "pod-a", "nginx:latest", "nginx", "Log message 1 from pod A"),
		createLogEvent(now-200000, "stream-pod-a", "pod-a", "nginx:latest", "nginx", "Log message 2 from pod A"),
		// Pod B events (interleaved)
		createLogEvent(now-250000, "stream-pod-b", "pod-b", "nginx:latest", "nginx", "Log message 1 from pod B"),
		createLogEvent(now-150000, "stream-pod-b", "pod-b", "nginx:latest", "nginx", "Log message 2 from pod B"),
		// More Pod A events
		createLogEvent(now-100000, "stream-pod-a", "pod-a", "nginx:latest", "nginx", "Log message 3 from pod A"),
	}
	
	// Mock client that returns our test events
	mockClient := &mockCloudWatchClient{
		events: events,
		pageSize: 10, // Return all events in one page for this test
	}
	
	// Create channel to collect output
	chLogLines := make(chan string, 1000)
	
	// Run the function
	go func() {
		_, err := app.parseAllEventsWithFilterClient(context.Background(), mockClient, "test-group", now-3600000, now, chLogLines)
		if err != nil {
			t.Errorf("parseAllEventsWithFilterClient returned error: %v", err)
		}
		close(chLogLines)
	}()
	
	// Collect all output
	var output []string
	for line := range chLogLines {
		output = append(output, line)
	}
	
	// Verify output
	outputStr := strings.Join(output, "\n")
	
	// Check that stream headers are present (new behavior groups by stream)
	streamAStart := strings.Index(outputStr, "<b>Parse stream</b> :stream-pod-a")
	if streamAStart == -1 {
		t.Fatal("Stream pod-a header not found")
	}
	
	streamBStart := strings.Index(outputStr, "<b>Parse stream</b> :stream-pod-b")
	if streamBStart == -1 {
		t.Fatal("Stream pod-b header not found")
	}
	
	// Find all occurrences of pod A logs
	podALog1 := strings.Index(outputStr, "Log message 1 from pod A")
	podALog2 := strings.Index(outputStr, "Log message 2 from pod A")
	podALog3 := strings.Index(outputStr, "Log message 3 from pod A")
	
	// Find all occurrences of pod B logs
	podBLog1 := strings.Index(outputStr, "Log message 1 from pod B")
	podBLog2 := strings.Index(outputStr, "Log message 2 from pod B")
	
	// Verify that all stream A logs appear after stream A header and before stream B header (if stream A comes first)
	// or after stream B header (if stream B comes first)
	if streamAStart < streamBStart {
		// Stream A comes first
		if podALog1 < streamAStart || podALog1 > streamBStart {
			t.Error("Stream A log 1 not in correct section")
		}
		if podALog2 < streamAStart || podALog2 > streamBStart {
			t.Error("Stream A log 2 not in correct section")
		}
		if podALog3 < streamAStart || podALog3 > streamBStart {
			t.Error("Stream A log 3 not in correct section")
		}
		// Stream B logs should be after stream B header
		if podBLog1 < streamBStart {
			t.Error("Stream B log 1 not in correct section")
		}
		if podBLog2 < streamBStart {
			t.Error("Stream B log 2 not in correct section")
		}
	} else {
		// Stream B comes first
		if podBLog1 < streamBStart || podBLog1 > streamAStart {
			t.Error("Stream B log 1 not in correct section")
		}
		if podBLog2 < streamBStart || podBLog2 > streamAStart {
			t.Error("Stream B log 2 not in correct section")
		}
		// Stream A logs should be after stream A header
		if podALog1 < streamAStart {
			t.Error("Stream A log 1 not in correct section")
		}
		if podALog2 < streamAStart {
			t.Error("Stream A log 2 not in correct section")
		}
		if podALog3 < streamAStart {
			t.Error("Stream A log 3 not in correct section")
		}
	}
	
	// Verify logs are ordered by timestamp within each pod
	// This is a bit complex to verify from the string output, but we can at least
	// check that all logs from each pod are present
	if !strings.Contains(outputStr, "Log message 1 from pod A") {
		t.Error("Missing log message 1 from pod A")
	}
	if !strings.Contains(outputStr, "Log message 2 from pod A") {
		t.Error("Missing log message 2 from pod A")
	}
	if !strings.Contains(outputStr, "Log message 3 from pod A") {
		t.Error("Missing log message 3 from pod A")
	}
	if !strings.Contains(outputStr, "Log message 1 from pod B") {
		t.Error("Missing log message 1 from pod B")
	}
	if !strings.Contains(outputStr, "Log message 2 from pod B") {
		t.Error("Missing log message 2 from pod B")
	}
}

func TestParseAllEventsWithFilter_IgnoredContainers(t *testing.T) {
	// Create test app with container to ignore
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	
	app := &App{
		cfg: configapp.AppConfig{
			ContainerNameToIgnore: []string{"sidecar"},
		},
		awscfg:            aws.Config{},
		rules:             []string{}, // No rules, so all logs will be included
		lastPeriodToWatch: 3600,
		appLog:            logger,
		eventsRateLimit:   rate.NewLimiter(rate.Limit(25), 25),
		logGroupRateLimit: rate.NewLimiter(rate.Limit(10), 10),
	}
	
	// Create mock events with one container to be ignored
	now := time.Now().Unix() * 1000
	events := []types.FilteredLogEvent{
		createLogEvent(now-300000, "stream-1", "pod-1", "app:latest", "app", "This should be included"),
		createLogEvent(now-200000, "stream-2", "pod-2", "sidecar:latest", "sidecar", "This should be ignored"),
		createLogEvent(now-100000, "stream-1", "pod-1", "app:latest", "app", "This should also be included"),
	}
	
	// Mock client
	mockClient := &mockCloudWatchClient{
		events: events,
		pageSize: 10,
	}
	
	// Create channel to collect output
	chLogLines := make(chan string, 1000)
	
	// Run the function
	go func() {
		_, err := app.parseAllEventsWithFilterClient(context.Background(), mockClient, "test-group", now-3600000, now, chLogLines)
		if err != nil {
			t.Errorf("parseAllEventsWithFilterClient returned error: %v", err)
		}
		close(chLogLines)
	}()
	
	// Collect all output
	var output []string
	for line := range chLogLines {
		output = append(output, line)
	}
	
	outputStr := strings.Join(output, "\n")
	
	// Verify that sidecar container is not in output
	if strings.Contains(outputStr, "sidecar") {
		t.Error("Ignored container 'sidecar' found in output")
	}
	if strings.Contains(outputStr, "This should be ignored") {
		t.Error("Ignored container log message found in output")
	}
	
	// Verify that app container is in output
	if !strings.Contains(outputStr, "app:latest") {
		t.Error("App container not found in output")
	}
	if !strings.Contains(outputStr, "This should be included") {
		t.Error("App container log message not found in output")
	}
	if !strings.Contains(outputStr, "This should also be included") {
		t.Error("Second app container log message not found in output")
	}
}

func TestParseAllEventsWithFilter_LogsMatchingRules(t *testing.T) {
	// Create test app with rules
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	
	app := &App{
		cfg:               configapp.AppConfig{},
		awscfg:            aws.Config{},
		rules:             []string{"^DEBUG:", "^INFO:"}, // Ignore DEBUG and INFO logs
		lastPeriodToWatch: 3600,
		appLog:            logger,
		eventsRateLimit:   rate.NewLimiter(rate.Limit(25), 25),
		logGroupRateLimit: rate.NewLimiter(rate.Limit(10), 10),
	}
	
	// Create mock events with different log levels
	now := time.Now().Unix() * 1000
	events := []types.FilteredLogEvent{
		createLogEvent(now-300000, "stream-1", "pod-1", "app:latest", "app", "ERROR: This is an error"),
		createLogEvent(now-200000, "stream-1", "pod-1", "app:latest", "app", "DEBUG: This is debug info"),
		createLogEvent(now-100000, "stream-1", "pod-1", "app:latest", "app", "WARN: This is a warning"),
		createLogEvent(now-50000, "stream-1", "pod-1", "app:latest", "app", "INFO: This is info"),
	}
	
	// Mock client
	mockClient := &mockCloudWatchClient{
		events: events,
		pageSize: 10,
	}
	
	// Create channel to collect output
	chLogLines := make(chan string, 1000)
	
	// Run the function
	go func() {
		_, err := app.parseAllEventsWithFilterClient(context.Background(), mockClient, "test-group", now-3600000, now, chLogLines)
		if err != nil {
			t.Errorf("parseAllEventsWithFilterClient returned error: %v", err)
		}
		close(chLogLines)
	}()
	
	// Collect all output
	var output []string
	for line := range chLogLines {
		output = append(output, line)
	}
	
	outputStr := strings.Join(output, "\n")
	
	// Verify that DEBUG and INFO logs are not in output (they match rules)
	if strings.Contains(outputStr, "DEBUG: This is debug info") {
		t.Error("DEBUG log found in output but should be filtered by rule")
	}
	if strings.Contains(outputStr, "INFO: This is info") {
		t.Error("INFO log found in output but should be filtered by rule")
	}
	
	// Verify that ERROR and WARN logs are in output
	if !strings.Contains(outputStr, "ERROR: This is an error") {
		t.Error("ERROR log not found in output")
	}
	if !strings.Contains(outputStr, "WARN: This is a warning") {
		t.Error("WARN log not found in output")
	}
}

func TestParseAllEventsWithFilter_MixedContainersInStream(t *testing.T) {
	// This test demonstrates the key fix: multiple containers in the same stream stay together
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	
	app := &App{
		cfg:               configapp.AppConfig{},
		awscfg:            aws.Config{},
		rules:             []string{}, // No rules, so all logs will be included
		lastPeriodToWatch: 3600,
		appLog:            logger,
		eventsRateLimit:   rate.NewLimiter(rate.Limit(25), 25),
		logGroupRateLimit: rate.NewLimiter(rate.Limit(10), 10),
	}
	
	// Create events from same stream but different containers (simulating pod with multiple containers)
	now := time.Now().Unix() * 1000
	events := []types.FilteredLogEvent{
		// All from same stream but different containers, interleaved by timestamp
		createLogEvent(now-500000, "pod-abc-xyz-stream", "pod-abc", "nginx:latest", "nginx", "nginx: starting up"),
		createLogEvent(now-400000, "pod-abc-xyz-stream", "pod-abc", "sidecar:latest", "sidecar", "sidecar: initializing"),
		createLogEvent(now-300000, "pod-abc-xyz-stream", "pod-abc", "nginx:latest", "nginx", "nginx: ready to serve"),
		createLogEvent(now-200000, "pod-abc-xyz-stream", "pod-abc", "sidecar:latest", "sidecar", "sidecar: ready"),
		createLogEvent(now-100000, "pod-abc-xyz-stream", "pod-abc", "nginx:latest", "nginx", "nginx: first request"),
	}
	
	mockClient := &mockCloudWatchClient{
		events: events,
		pageSize: 10,
	}
	
	chLogLines := make(chan string, 1000)
	
	go func() {
		_, err := app.parseAllEventsWithFilterClient(context.Background(), mockClient, "test-group", now-3600000, now, chLogLines)
		if err != nil {
			t.Errorf("parseAllEventsWithFilterClient returned error: %v", err)
		}
		close(chLogLines)
	}()
	
	var output []string
	for line := range chLogLines {
		output = append(output, line)
	}
	
	outputStr := strings.Join(output, "\n")
	
	// Verify there's only ONE stream section (not separate sections per container)
	streamHeaderCount := strings.Count(outputStr, "<b>Parse stream</b> :pod-abc-xyz-stream")
	if streamHeaderCount != 1 {
		t.Errorf("Expected 1 stream header, got %d", streamHeaderCount)
	}
	
	// Verify all logs are present
	if !strings.Contains(outputStr, "nginx: starting up") {
		t.Error("Missing nginx startup log")
	}
	if !strings.Contains(outputStr, "sidecar: initializing") {
		t.Error("Missing sidecar init log")
	}
	if !strings.Contains(outputStr, "nginx: ready to serve") {
		t.Error("Missing nginx ready log")
	}
	if !strings.Contains(outputStr, "sidecar: ready") {
		t.Error("Missing sidecar ready log")
	}
	if !strings.Contains(outputStr, "nginx: first request") {
		t.Error("Missing nginx request log")
	}
	
	// Verify logs are in chronological order (this was the main issue!)
	startupPos := strings.Index(outputStr, "nginx: starting up")
	initPos := strings.Index(outputStr, "sidecar: initializing") 
	readyPos := strings.Index(outputStr, "nginx: ready to serve")
	sidecarReadyPos := strings.Index(outputStr, "sidecar: ready")
	requestPos := strings.Index(outputStr, "nginx: first request")
	
	if !(startupPos < initPos && initPos < readyPos && readyPos < sidecarReadyPos && sidecarReadyPos < requestPos) {
		t.Error("Logs are not in chronological order within the stream")
	}
	
	// The key test: verify that the header shows info from the first container encountered
	if !strings.Contains(outputStr, "<b>Container Image</b> :nginx:latest") {
		t.Error("Stream header should show first container (nginx)")
	}
	if !strings.Contains(outputStr, "<b>Container Name</b> :nginx") {
		t.Error("Stream header should show first container name (nginx)")
	}
}