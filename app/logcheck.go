package app

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/sgaunet/calcdate/calcdate"
)

// Main function that parses every streams of loggroup groupName
func (a *App) LogCheck(cfg aws.Config, groupName string, reportFilename string) (int, error) {
	f, err := os.Create(reportFilename)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	clientCloudwatchlogs := cloudwatchlogs.NewFromConfig(cfg)
	doesGroupNameExists := a.findLogGroup(clientCloudwatchlogs, groupName, "")
	if !doesGroupNameExists {
		err := fmt.Errorf("GroupName %s not found", groupName)
		a.appLog.Errorln(err.Error())
		return 0, err
	}

	minTimeStampInMs, maxTimeStampInMs, err := a.GetTimeStampMsRangeofLastHour()
	if err != nil {
		return 0, err
	}
	a.appLog.Debugln("minTimeStampsInMs=", minTimeStampInMs)
	a.appLog.Debugln("maxTimeStampsInMs=", maxTimeStampInMs)
	return a.parseAllStreamsOfGroup(clientCloudwatchlogs, groupName, "", minTimeStampInMs, maxTimeStampInMs, f)
}

func (a *App) GetTimeStampMsRangeofLastHour() (begin int64, end int64, err error) {
	beginTime, err := calcdate.CreateDate("// -1::", "yyyy/mm/dd hh:mm:ss", "UTC", true, false)
	if err != nil {
		return
	}
	begin = beginTime.Unix() * 1000
	endTime, err := calcdate.CreateDate("// -1::", "yyyy/mm/dd hh:mm:ss", "UTC", false, true)
	if err != nil {
		return
	}
	end = endTime.Unix() * 1000
	a.appLog.Debugln("beginTime=", beginTime)
	a.appLog.Debugln("endTime=", endTime)
	return
}
