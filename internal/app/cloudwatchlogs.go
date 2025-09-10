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
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// Recursive function that will return if the groupName parameter has been found or not.
func (a *App) findLogGroup(ctx context.Context, clientCloudwatchlogs *cloudwatchlogs.Client,
	groupName string, nextToken string) bool {
	var params cloudwatchlogs.DescribeLogGroupsInput
	if len(nextToken) != 0 {
		params.NextToken = &nextToken
	}
	if err := a.logGroupRateLimit.Wait(ctx); err != nil {
		a.appLog.Error("Rate limit error", slog.String("error", err.Error()))
		return false
	}
	res, err := clientCloudwatchlogs.DescribeLogGroups(ctx, &params)
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
	return a.findLogGroup(ctx, clientCloudwatchlogs, groupName, *res.NextToken)
}

const millisecondsMultiplier = 1000

// streamEvents holds events grouped by log stream (like original behavior).
type streamEvents struct {
	streamName          string
	firstContainerInfo  containerInfo // Info from first non-ignored container encountered
	events              []logEvent
	hasIgnoredContainer bool // If true, skip this entire stream
}

// containerInfo holds container metadata.
type containerInfo struct {
	podName        string
	containerImage string
	containerName  string
}

// logEvent represents a single log event with its timestamp.
type logEvent struct {
	timestamp int64
	message   string
}

// CloudWatchLogsFilterClient interface for testing.
type CloudWatchLogsFilterClient interface {
	FilterLogEvents(ctx context.Context,
		params *cloudwatchlogs.FilterLogEventsInput,
		optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error)
}

// parseAllEventsWithFilter uses FilterLogEvents API for improved performance.
func (a *App) parseAllEventsWithFilter(ctx context.Context,
	clientCloudwatchlogs *cloudwatchlogs.Client, groupName string,
	minTimeStamp int64, maxTimeStamp int64, chLogLines chan<- string) (int, error) {
	return a.parseAllEventsWithFilterClient(ctx, clientCloudwatchlogs, groupName, minTimeStamp, maxTimeStamp, chLogLines)
}

// parseAllEventsWithFilterClient is the testable version that takes an interface.
func (a *App) parseAllEventsWithFilterClient(ctx context.Context, client CloudWatchLogsFilterClient,
	groupName string, minTimeStamp int64, maxTimeStamp int64, chLogLines chan<- string) (int, error) {
	input := a.buildFilterLogEventsInput(groupName, minTimeStamp, maxTimeStamp)
	streamGroups, eventCount, err := a.fetchAndProcessAllEvents(ctx, client, input)
	if err != nil {
		return 0, err
	}
	return a.outputStreamEvents(streamGroups, chLogLines, eventCount)
}

func (a *App) buildFilterLogEventsInput(groupName string, minTimeStamp,
	maxTimeStamp int64) *cloudwatchlogs.FilterLogEventsInput {
	a.appLog.Debug("Starting FilterLogEvents",
		slog.String("groupName", groupName),
		slog.Int64("minTimeStamp", minTimeStamp),
		slog.Int64("maxTimeStamp", maxTimeStamp))

	return &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: &groupName,
		StartTime:    &minTimeStamp,
		EndTime:      &maxTimeStamp,
		Interleaved:  &[]bool{true}[0], // Sort events from multiple streams by timestamp
	}
}

func (a *App) fetchAndProcessAllEvents(ctx context.Context, client CloudWatchLogsFilterClient,
	input *cloudwatchlogs.FilterLogEventsInput) (map[string]*streamEvents, int, error) {
	streamGroups := make(map[string]*streamEvents)
	eventCount := 0
	pageCount := 0
	var nextToken *string

	for {
		if err := a.checkContextAndRateLimit(ctx); err != nil {
			return nil, eventCount, err
		}

		if nextToken != nil {
			input.NextToken = nextToken
		}

		output, err := client.FilterLogEvents(ctx, input)
		if err != nil {
			return nil, eventCount, fmt.Errorf("failed to filter log events: %w", err)
		}

		pageCount++
		a.appLog.Debug("Processing page", slog.Int("pageCount", pageCount), slog.Int("eventsCount", len(output.Events)))

		if len(output.Events) == 0 && output.NextToken == nil {
			break
		}

		a.processEventsInPage(output.Events, streamGroups, &eventCount)

		if eventCount%1000 == 0 {
			a.appLog.Debug("Processed events", slog.Int("eventCount", eventCount))
		}

		nextToken = output.NextToken
		if nextToken == nil {
			break
		}
	}

	a.appLog.Debug("Completed FilterLogEvents processing",
		slog.Int("totalEvents", eventCount),
		slog.Int("pages", pageCount),
		slog.Int("streams", len(streamGroups)))
	return streamGroups, eventCount, nil
}

func (a *App) checkContextAndRateLimit(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	default:
	}

	if err := a.eventsRateLimit.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait error: %w", err)
	}
	return nil
}

func (a *App) processEventsInPage(events []types.FilteredLogEvent,
	streamGroups map[string]*streamEvents, eventCount *int) {
	for _, event := range events {
		*eventCount++
		a.processLogEvent(event, streamGroups)
	}
}

func (a *App) processLogEvent(event types.FilteredLogEvent, streamGroups map[string]*streamEvents) {
	var lineOfLog fluentDockerLog
	err := json.Unmarshal([]byte(*event.Message), &lineOfLog)
	if err != nil {
		a.appLog.Error("Failed to parse log event", slog.String("error", err.Error()))
		return
	}

	streamName := *event.LogStreamName
	stream := a.getOrCreateStream(streamName, streamGroups)

	if a.isLineMatchWithOneRule(lineOfLog.Log, a.rules) {
		return
	}

	a.processUnmatchedLogLine(lineOfLog, stream, event, streamName)
}

func (a *App) getOrCreateStream(streamName string, streamGroups map[string]*streamEvents) *streamEvents {
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
	return stream
}

func (a *App) processUnmatchedLogLine(lineOfLog fluentDockerLog, stream *streamEvents,
	event types.FilteredLogEvent, streamName string) {
	imageIgnored := a.isImageIgnored(lineOfLog.Kubernetes.ContainerImage)
	containerIgnored := a.isContainerIgnored(lineOfLog.Kubernetes.ContainerName)

	if imageIgnored || containerIgnored {
		stream.hasIgnoredContainer = true
		a.appLog.Debug("Stream marked as ignored",
			slog.String("streamName", streamName),
			slog.String("containerImage", lineOfLog.Kubernetes.ContainerImage),
			slog.String("containerName", lineOfLog.Kubernetes.ContainerName))
		return
	}

	a.addEventToStream(lineOfLog, stream, event)
}

func (a *App) addEventToStream(lineOfLog fluentDockerLog, stream *streamEvents, event types.FilteredLogEvent) {
	if stream.firstContainerInfo.containerImage == "" {
		stream.firstContainerInfo = containerInfo{
			podName:        lineOfLog.Kubernetes.PodName,
			containerImage: lineOfLog.Kubernetes.ContainerImage,
			containerName:  lineOfLog.Kubernetes.ContainerName,
		}
	}

	stream.events = append(stream.events, logEvent{
		timestamp: *event.Timestamp,
		message:   lineOfLog.Log,
	})
}

func (a *App) outputStreamEvents(streamGroups map[string]*streamEvents, chLogLines chan<- string, _ int) (int, error) {
	streamKeys := a.getSortedStreamKeys(streamGroups)
	cptLinePrinted := 0

	for _, streamKey := range streamKeys {
		stream := streamGroups[streamKey]
		if stream.hasIgnoredContainer || len(stream.events) == 0 {
			if stream.hasIgnoredContainer {
				a.appLog.Debug("Skipping stream due to ignored containers", slog.String("streamKey", streamKey))
			}
			continue
		}
		cptLinePrinted += a.outputSingleStream(stream, chLogLines)
	}

	a.appLog.Debug("Output complete",
		slog.Int("linesPrinted", cptLinePrinted),
		slog.Int("streams", len(streamGroups)))
	return cptLinePrinted, nil
}

func (a *App) getSortedStreamKeys(streamGroups map[string]*streamEvents) []string {
	streamKeys := make([]string, 0, len(streamGroups))
	for key := range streamGroups {
		streamKeys = append(streamKeys, key)
	}
	sort.Strings(streamKeys)
	return streamKeys
}

func (a *App) outputSingleStream(stream *streamEvents, chLogLines chan<- string) int {
	chLogLines <- "<b>Parse stream</b> :" + stream.streamName + "<br>"
	chLogLines <- "<b>Container Image</b> :" + stream.firstContainerInfo.containerImage + "<br>"
	chLogLines <- "<b>Container Name</b> :" + stream.firstContainerInfo.containerName + "<br>"

	sort.Slice(stream.events, func(i, j int) bool {
		return stream.events[i].timestamp < stream.events[j].timestamp
	})

	for _, event := range stream.events {
		timeT := time.Unix(event.timestamp/millisecondsMultiplier, 0).UTC()
		chLogLines <- fmt.Sprintf("%s UTC: %s<br>\n", timeT.Format("2006-01-02 15:04:05"), event.message)
	}

	chLogLines <- "<br>\n"
	return len(stream.events)
}
