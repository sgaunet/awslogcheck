package app

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

// Main function that parses every streams of loggroup groupName
func (a *App) LogCheck(cfg aws.Config, groupName string) (error, int) {
	clientCloudwatchlogs := cloudwatchlogs.NewFromConfig(cfg)

	doesGroupNameExists := a.findLogGroup(clientCloudwatchlogs, groupName, "")
	if !doesGroupNameExists {
		err := fmt.Errorf("GroupName %s not found", groupName)
		a.appLog.Errorln(err.Error())
		return err, 0
	}

	minTimeStampInMs := (time.Now().Unix() - int64(a.lastPeriodToWatch)) * 1000
	err, cpt := a.parseAllStreamsOfGroup(clientCloudwatchlogs, groupName, "", minTimeStampInMs)
	return err, cpt
}
