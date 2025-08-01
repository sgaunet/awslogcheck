package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

// Recursive function that will return if the groupName parameter has been found or not
func (a *App) findLogGroup(clientCloudwatchlogs *cloudwatchlogs.Client, groupName string, NextToken string) bool {
	var params cloudwatchlogs.DescribeLogGroupsInput
	if len(NextToken) != 0 {
		params.NextToken = &NextToken
	}
	if err := a.logGroupRateLimit.Wait(context.TODO()); err != nil {
		a.appLog.Error("Rate limit error", slog.String("error", err.Error()))
		return false
	}
	res, err := clientCloudwatchlogs.DescribeLogGroups(context.TODO(), &params)
	if err != nil {
		a.appLog.Error(err.Error())
		os.Exit(1)
	}
	for _, i := range res.LogGroups {
		fmt.Printf("## Parse Log Group Name : %s\n", *i.LogGroupName)
		if *i.LogGroupName == groupName {
			return true
		}
	}
	if res.NextToken == nil {
		// No token given, end of potential recursive call to parse the list of loggroups
		return false
	}
	return a.findLogGroup(clientCloudwatchlogs, groupName, *res.NextToken)
}

// streamEvents holds events grouped by log stream (like original behavior)
type streamEvents struct {
	streamName          string
	firstContainerInfo  containerInfo // Info from first non-ignored container encountered
	events              []logEvent
	hasIgnoredContainer bool // If true, skip this entire stream
}

// containerInfo holds container metadata
type containerInfo struct {
	podName        string
	containerImage string
	containerName  string
}

// logEvent represents a single log event with its timestamp
type logEvent struct {
	timestamp int64
	message   string
}

// CloudWatchLogsFilterClient interface for testing
type CloudWatchLogsFilterClient interface {
	FilterLogEvents(ctx context.Context, params *cloudwatchlogs.FilterLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error)
}

// parseAllEventsWithFilter uses FilterLogEvents API for improved performance
func (a *App) parseAllEventsWithFilter(ctx context.Context, clientCloudwatchlogs *cloudwatchlogs.Client, groupName string, minTimeStamp int64, maxTimeStamp int64, chLogLines chan<- string) (int, error) {
	return a.parseAllEventsWithFilterClient(ctx, clientCloudwatchlogs, groupName, minTimeStamp, maxTimeStamp, chLogLines)
}

// parseAllEventsWithFilterClient is the testable version that takes an interface
func (a *App) parseAllEventsWithFilterClient(ctx context.Context, client CloudWatchLogsFilterClient, groupName string, minTimeStamp int64, maxTimeStamp int64, chLogLines chan<- string) (int, error) {
	var cptLinePrinted int

	// Set up FilterLogEvents input parameters
	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: &groupName,
		StartTime:    &minTimeStamp,
		EndTime:      &maxTimeStamp,
		Interleaved:  &[]bool{true}[0], // Sort events from multiple streams by timestamp
	}

	a.appLog.Debug("Starting FilterLogEvents", slog.String("groupName", groupName), slog.Int64("minTimeStamp", minTimeStamp), slog.Int64("maxTimeStamp", maxTimeStamp))

	eventCount := 0
	pageCount := 0

	// Group events by stream to maintain original behavior
	streamGroups := make(map[string]*streamEvents)

	// Process all pages of results manually
	var nextToken *string
	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return cptLinePrinted, ctx.Err()
		default:
		}

		// Rate limit the API call
		if err := a.eventsRateLimit.Wait(ctx); err != nil {
			return cptLinePrinted, fmt.Errorf("rate limit wait error: %w", err)
		}

		// Set next token if we have one
		if nextToken != nil {
			input.NextToken = nextToken
		}

		// Get next page of events
		output, err := client.FilterLogEvents(ctx, input)
		if err != nil {
			return cptLinePrinted, fmt.Errorf("failed to filter log events: %w", err)
		}

		pageCount++
		a.appLog.Debug("Processing page", slog.Int("pageCount", pageCount), slog.Int("eventsCount", len(output.Events)))

		// If no events and no next token, we're done
		if len(output.Events) == 0 && output.NextToken == nil {
			break
		}

		// Process events in this page
		for _, event := range output.Events {
			eventCount++

			var lineOfLog fluentDockerLog
			err := json.Unmarshal([]byte(*event.Message), &lineOfLog)
			if err != nil {
				a.appLog.Error("Failed to parse log event", slog.String("error", err.Error()))
				continue
			}

			// Get or create stream group
			streamName := *event.LogStreamName
			stream, exists := streamGroups[streamName]
			if !exists {
				stream = &streamEvents{
					streamName:          streamName,
					events:              make([]logEvent, 0),
					hasIgnoredContainer: false,
				}
				streamGroups[streamName] = stream
				a.appLog.Debug("New stream group", slog.String("streamName", streamName))
			}

			// Check if this line matches any rules
			if !a.isLineMatchWithOneRule(lineOfLog.Log, a.rules) {
				// Check if this container/image should be ignored
				imageIgnored := a.isImageIgnored(lineOfLog.Kubernetes.ContainerImage)
				containerIgnored := a.isContainerIgnored(lineOfLog.Kubernetes.ContainerName)

				if imageIgnored || containerIgnored {
					// Mark this stream as having ignored containers (like old break behavior)
					stream.hasIgnoredContainer = true
					a.appLog.Debug("Stream marked as ignored", slog.String("streamName", streamName), slog.String("containerImage", lineOfLog.Kubernetes.ContainerImage), slog.String("containerName", lineOfLog.Kubernetes.ContainerName))
				} else {
					// Set first container info if not set yet (for headers)
					if stream.firstContainerInfo.containerImage == "" {
						stream.firstContainerInfo = containerInfo{
							podName:        lineOfLog.Kubernetes.PodName,
							containerImage: lineOfLog.Kubernetes.ContainerImage,
							containerName:  lineOfLog.Kubernetes.ContainerName,
						}
					}

					// Add event to the stream
					stream.events = append(stream.events, logEvent{
						timestamp: *event.Timestamp,
						message:   lineOfLog.Log,
					})
				}
			}
		}

		// Update progress periodically
		if eventCount%1000 == 0 {
			a.appLog.Debug("Processed events", slog.Int("eventCount", eventCount))
		}

		// Set next token for next iteration, break if no more pages
		nextToken = output.NextToken
		if nextToken == nil {
			break
		}
	}

	// Sort stream keys for consistent output
	streamKeys := make([]string, 0, len(streamGroups))
	for key := range streamGroups {
		streamKeys = append(streamKeys, key)
	}
	sort.Strings(streamKeys)

	// Output events grouped by stream (like original behavior)
	for _, streamKey := range streamKeys {
		stream := streamGroups[streamKey]

		// Skip streams that have ignored containers (like original break behavior)
		if stream.hasIgnoredContainer {
			a.appLog.Debug("Skipping stream due to ignored containers", slog.String("streamKey", streamKey))
			continue
		}

		// Skip streams with no events
		if len(stream.events) == 0 {
			continue
		}

		// Print stream header (like original code - once per stream)
		chLogLines <- "<b>Parse stream</b> :" + stream.streamName + "<br>"
		chLogLines <- "<b>Container Image</b> :" + stream.firstContainerInfo.containerImage + "<br>"
		chLogLines <- "<b>Container Name</b> :" + stream.firstContainerInfo.containerName + "<br>"

		// Sort events within the stream by timestamp
		sort.Slice(stream.events, func(i, j int) bool {
			return stream.events[i].timestamp < stream.events[j].timestamp
		})

		// Print all events for this stream (all containers mixed chronologically)
		for _, event := range stream.events {
			timeT := time.Unix(event.timestamp/1000, 0).UTC()
			chLogLines <- fmt.Sprintf("%s UTC: %s<br>\n", timeT.Format("2006-01-02 15:04:05"), event.message)
			cptLinePrinted++
		}

		// Add separator between streams (like original code)
		chLogLines <- "<br>\n"
	}

	a.appLog.Debug("Completed FilterLogEvents processing", slog.Int("totalEvents", eventCount), slog.Int("linesPrinted", cptLinePrinted), slog.Int("pages", pageCount), slog.Int("streams", len(streamGroups)))
	return cptLinePrinted, nil
}
