package app

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

// Recursive function that will return if the groupName parameter has been found or not
func (a *App) findLogGroup(clientCloudwatchlogs *cloudwatchlogs.Client, groupName string, NextToken string) bool {
	var params cloudwatchlogs.DescribeLogGroupsInput

	if len(NextToken) != 0 {
		params.NextToken = &NextToken
	}

	res, err := clientCloudwatchlogs.DescribeLogGroups(context.TODO(), &params)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	for _, i := range res.LogGroups {
		fmt.Println("Parse Log Group Name :", *i.LogGroupName)
		if *i.LogGroupName == groupName {
			return true
		}
	}

	if len(*res.NextToken) == 0 {
		// No token given, end of potential recursive call to parse the list of loggroups
		return false
	}

	return a.findLogGroup(clientCloudwatchlogs, groupName, *res.NextToken)
}

// Parse every events of every streams of a group
// Recursive function
func (a *App) parseAllStreamsOfGroup(clientCloudwatchlogs *cloudwatchlogs.Client, groupName string, nextToken string, minTimeStamp int64) error {
	var paramsLogStream cloudwatchlogs.DescribeLogStreamsInput
	var stopToParseLogStream bool

	// Search logstreams of groupName
	// Ordered by last event time
	// descending
	paramsLogStream.LogGroupName = &groupName
	paramsLogStream.OrderBy = "LastEventTime"
	descending := true
	paramsLogStream.Descending = &descending

	if len(nextToken) != 0 {
		paramsLogStream.NextToken = &nextToken
	}

	// https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs#Client.DescribeLogStreams
	res2, err := clientCloudwatchlogs.DescribeLogStreams(context.TODO(), &paramsLogStream)
	if err != nil {
		return err
	}

	// Loop over streams
	for _, j := range res2.LogStreams {
		// fmt.Println(*j.LogStreamName)
		// fmt.Println(*j.LastEventTimestamp)
		// tm := time.Unix(*j.LastEventTimestamp/1000, 0) // aws timestamp are in ms
		// fmt.Printf("Parse stream : %s (Last event %v)\n", *j.LogStreamName, tm)

		// No need to parse old logstream older than minTimeStamp
		if *j.LastEventTimestamp < minTimeStamp {
			stopToParseLogStream = true
			// fmt.Printf("%v < %v\n", *j.LastEventTimestamp, minTimeStamp)
			break
		}

		a.getEvents(context.TODO(), groupName, *j.LogStreamName, clientCloudwatchlogs)
	}

	if res2.NextToken != nil && !stopToParseLogStream {
		a.parseAllStreamsOfGroup(clientCloudwatchlogs, groupName, *res2.NextToken, minTimeStamp)
	}
	return nil
}
