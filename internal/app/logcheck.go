package app

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/sgaunet/calcdate/calcdate"
)

// Main function that parses every streams of loggroup a.cfg.LogGroup
func (a *App) LogCheck(ctx context.Context) error {
	var wg sync.WaitGroup
	chLogLines := make(chan string, 1000)
	clientCloudwatchlogs := cloudwatchlogs.NewFromConfig(a.awscfg)
	LogGroupExists := a.findLogGroup(clientCloudwatchlogs, a.cfg.LogGroup, "")
	if !LogGroupExists {
		err := fmt.Errorf("a.cfg.LogGroup %s not found", a.cfg.LogGroup)
		a.appLog.Errorln(err.Error())
		return err
	}

	minTimeStampInMs, maxTimeStampInMs, err := a.GetTimeStampMsRangeofLastHour()
	if err != nil {
		return err
	}
	a.appLog.Debugln("minTimeStampsInMs=", minTimeStampInMs)
	a.appLog.Debugln("maxTimeStampsInMs=", maxTimeStampInMs)

	wg.Add(1)
	go a.collectLinesOfReportAndSendReport(ctx, &wg, chLogLines)
	
	// Use the new FilterLogEvents API for better performance
	_, err = a.parseAllEventsWithFilter(ctx, clientCloudwatchlogs, a.cfg.LogGroup, minTimeStampInMs, maxTimeStampInMs, chLogLines)
	close(chLogLines)
	
	wg.Wait()
	return err
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

// collectLinesOfReportAndSendReport collect lines of report and send report if file is overs 2MB
func (a *App) collectLinesOfReportAndSendReport(ctx context.Context, wg *sync.WaitGroup, chLines <-chan string) {
	defer wg.Done()
	emptyReport := true // used to know if report has to be sent or not
	reportFilename := "/tmp/report.html"
	a.appLog.Debugln("create report")
	f, err := os.Create(reportFilename)
	if err != nil {
		a.appLog.Fatalln(err)
	}
	sizeFile := 0

loop:
	for {
		line, ok := <-chLines
		if !ok {
			// channel closed
			a.appLog.Debugln("channel closed")
			break loop
		}
		emptyReport = false
		f.WriteString(line)
		sizeFile = sizeFile + len(line)

		if sizeFile > a.cfg.SmtpConfig.MaxReportSize {
			a.appLog.Debugln("size > MaxReportSize")
			emptyReport = true
			f.Close()
			a.appLog.Debugln("send report *")
			err = a.SendReport(reportFilename)
			if err != nil {
				a.appLog.Errorln(err)
			}
			a.appLog.Debugln("remove report")
			err = os.Remove(reportFilename)
			if err != nil {
				a.appLog.Errorln(err)
			}
			a.appLog.Debugln("create report")
			f, err = os.Create(reportFilename)
			if err != nil {
				a.appLog.Fatalln(err)
			}
			sizeFile = 0
		}
	}

	f.Close()

	if !emptyReport {
		a.appLog.Debugln("send report")
		err = a.SendReport(reportFilename)
		if err != nil {
			a.appLog.Errorln(err)
		}
	}
	a.appLog.Debugln("remove report")
	err = os.Remove(reportFilename)
	if err != nil {
		a.appLog.Errorln(err)
	}
	a.appLog.Debugln("end")
}
