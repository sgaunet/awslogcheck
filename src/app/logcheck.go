package app

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

// Main function that parses every streams of loggroup groupName
func (a *App) LogCheck(cfg aws.Config, groupName string) error {
	clientCloudwatchlogs := cloudwatchlogs.NewFromConfig(cfg)

	doesGroupNameExists := a.findLogGroup(clientCloudwatchlogs, groupName, "")
	if !doesGroupNameExists {
		return fmt.Errorf("GroupName %s not found\n.", groupName)
	}

	minTimeStampInMs := (time.Now().Unix() - int64(a.lastPeriodToWatch)) * 1000
	a.parseAllStreamsOfGroup(clientCloudwatchlogs, groupName, "", minTimeStampInMs)
	return nil
}
