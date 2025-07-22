package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
		a.appLog.Errorln("Rate limit error:", err.Error())
		return false
	}
	res, err := clientCloudwatchlogs.DescribeLogGroups(context.TODO(), &params)
	if err != nil {
		a.appLog.Errorln(err.Error())
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

// parseAllEventsWithFilter uses FilterLogEvents API for improved performance
func (a *App) parseAllEventsWithFilter(ctx context.Context, clientCloudwatchlogs *cloudwatchlogs.Client, groupName string, minTimeStamp int64, maxTimeStamp int64, chLogLines chan<- string) (int, error) {
	var cptLinePrinted int

	// Set up FilterLogEvents input parameters
	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: &groupName,
		StartTime:    &minTimeStamp,
		EndTime:      &maxTimeStamp,
		Interleaved:  &[]bool{true}[0], // Sort events from multiple streams by timestamp
	}

	a.appLog.Debugf("Starting FilterLogEvents for group %s with time range %d-%d", groupName, minTimeStamp, maxTimeStamp)

	// Create paginator for handling large result sets
	paginator := cloudwatchlogs.NewFilterLogEventsPaginator(clientCloudwatchlogs, input)

	eventCount := 0
	pageCount := 0
	
	// Keep track of containers we've already printed headers for
	containersPrinted := make(map[string]bool)

	// Process all pages of results
	for paginator.HasMorePages() {
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

		// Get next page of events
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return cptLinePrinted, fmt.Errorf("failed to filter log events: %w", err)
		}

		pageCount++
		a.appLog.Debugf("Processing page %d with %d events", pageCount, len(output.Events))

		// Process events in this page
		for _, event := range output.Events {
			eventCount++
			
			var lineOfLog fluentDockerLog
			err := json.Unmarshal([]byte(*event.Message), &lineOfLog)
			if err != nil {
				a.appLog.Errorln("Failed to parse log event:", err.Error())
				continue
			}

			// Check if this line matches any rules
			if !isLineMatchWithOneRule(lineOfLog.Log, a.rules) {
				// Check if this container/image should be ignored
				imageIgnored := a.isImageIgnored(lineOfLog.Kubernetes.ContainerImage)
				containerIgnored := a.isContainerIgnored(lineOfLog.Kubernetes.ContainerName)

				if !imageIgnored && !containerIgnored {
					// Create a unique key for this container
					containerKey := fmt.Sprintf("%s|%s|%s", 
						lineOfLog.Kubernetes.ContainerImage,
						lineOfLog.Kubernetes.ContainerName,
						*event.LogStreamName)

					// Print container info if we haven't already
					if !containersPrinted[containerKey] {
						a.appLog.Debugf("Parse stream=%v containerImage=%v containerName=%v\n", 
							*event.LogStreamName, lineOfLog.Kubernetes.ContainerImage, lineOfLog.Kubernetes.ContainerName)
						chLogLines <- "<b>Parse stream</b> :" + *event.LogStreamName + "<br>"
						chLogLines <- "<b>Container Image</b> :" + lineOfLog.Kubernetes.ContainerImage + "<br>"
						chLogLines <- "<b>Container Name</b> :" + lineOfLog.Kubernetes.ContainerName + "<br>"
						containersPrinted[containerKey] = true
					}

					// Print the log line
					timeT := time.Unix(*event.Timestamp/1000, 0).UTC()
					chLogLines <- fmt.Sprintf("%s UTC: %s<br>\n", timeT.Format("2006-01-02 15:04:05"), lineOfLog.Log)
					cptLinePrinted++
				}
			}
		}

		// Update progress periodically
		if eventCount%1000 == 0 {
			a.appLog.Debugf("Processed %d events so far...", eventCount)
		}
	}

	// Add line breaks between different containers
	for range containersPrinted {
		chLogLines <- "<br>\n"
	}

	a.appLog.Debugf("Completed FilterLogEvents processing: %d total events, %d lines printed, from %d pages", 
		eventCount, cptLinePrinted, pageCount)
	return cptLinePrinted, nil
}
