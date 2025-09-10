package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/sgaunet/calcdate/calcdate"
)

// Main function that parses every streams of loggroup a.cfg.LogGroup.
const logLinesChannelSize = 1000

// LogCheck performs the main log checking process.
func (a *App) LogCheck(ctx context.Context) error {
	var wg sync.WaitGroup
	chLogLines := make(chan string, logLinesChannelSize)
	clientCloudwatchlogs := cloudwatchlogs.NewFromConfig(a.awscfg)
	LogGroupExists := a.findLogGroup(ctx, clientCloudwatchlogs, a.cfg.LogGroup, "")
	if !LogGroupExists {
		err := fmt.Errorf("%w: %s", ErrLogGroupNotFound, a.cfg.LogGroup)
		a.appLog.Error(err.Error())
		return err
	}

	minTimeStampInMs, maxTimeStampInMs, err := a.GetTimeStampMsRangeofLastHour()
	if err != nil {
		return err
	}
	a.appLog.Debug("minTimeStampsInMs", slog.Int64("value", minTimeStampInMs))
	a.appLog.Debug("maxTimeStampsInMs", slog.Int64("value", maxTimeStampInMs))

	wg.Add(1)
	go a.collectLinesOfReportAndSendReport(ctx, &wg, chLogLines)

	// Use the new FilterLogEvents API for better performance
	_, err = a.parseAllEventsWithFilter(ctx, clientCloudwatchlogs,
		a.cfg.LogGroup, minTimeStampInMs, maxTimeStampInMs, chLogLines)
	close(chLogLines)

	wg.Wait()
	return err
}

// GetTimeStampMsRangeofLastHour returns timestamps for the last hour in milliseconds.
func (a *App) GetTimeStampMsRangeofLastHour() (int64, int64, error) {
	beginTime, err := calcdate.CreateDate("// -1::", "yyyy/mm/dd hh:mm:ss", "UTC", true, false)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create begin date: %w", err)
	}
	begin := beginTime.Unix() * millisecondsMultiplier
	endTime, err := calcdate.CreateDate("// -1::", "yyyy/mm/dd hh:mm:ss", "UTC", false, true)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create end date: %w", err)
	}
	end := endTime.Unix() * millisecondsMultiplier
	a.appLog.Debug("beginTime", slog.Time("value", beginTime))
	a.appLog.Debug("endTime", slog.Time("value", endTime))
	return begin, end, nil
}

// collectLinesOfReportAndSendReport collect lines of report and send report if file is overs 2MB.
func (a *App) collectLinesOfReportAndSendReport(_ context.Context, wg *sync.WaitGroup, chLines <-chan string) {
	defer wg.Done()
	emptyReport := true // used to know if report has to be sent or not
	reportFilename := "/tmp/report.html"
	f, err := a.createReportFile(reportFilename)
	if err != nil {
		return
	}
	sizeFile := 0

	for {
		line, ok := <-chLines
		if !ok {
			a.appLog.Debug("channel closed")
			break
		}
		emptyReport = false
		if f, sizeFile = a.writeLineToReport(f, line, sizeFile, reportFilename); f == nil {
			return
		}
	}

	a.closeReportFile(f)
	a.sendReportIfNotEmpty(emptyReport, reportFilename)
}

func (a *App) createReportFile(filename string) (*os.File, error) {
	a.appLog.Debug("create report")
	// #nosec G304 - filename is a controlled temp file path from internal function
	f, err := os.Create(filename)
	if err != nil {
		a.appLog.Error("Failed to create report file", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to create report file: %w", err)
	}
	return f, nil
}

func (a *App) writeLineToReport(f *os.File, line string, sizeFile int, filename string) (*os.File, int) {
	if _, err := f.WriteString(line); err != nil {
		a.appLog.Error("Failed to write to report file", slog.String("error", err.Error()))
		return f, sizeFile
	}
	sizeFile += len(line)

	if sizeFile > a.cfg.SMTPConfig.MaxReportSize {
		a.appLog.Debug("size > MaxReportSize")
		a.closeReportFile(f)
		a.sendAndRemoveReport(filename)
		newFile, err := a.createReportFile(filename)
		if err != nil {
			return nil, 0
		}
		return newFile, 0
	}
	return f, sizeFile
}

func (a *App) closeReportFile(f *os.File) {
	if err := f.Close(); err != nil {
		a.appLog.Error("Failed to close report file", slog.String("error", err.Error()))
	}
}

func (a *App) sendAndRemoveReport(filename string) {
	a.appLog.Debug("send report *")
	if err := a.SendReport(filename); err != nil {
		a.appLog.Error("Error occurred", slog.String("error", err.Error()))
	}
	a.appLog.Debug("remove report")
	if err := os.Remove(filename); err != nil {
		a.appLog.Error("Error occurred", slog.String("error", err.Error()))
	}
}

func (a *App) sendReportIfNotEmpty(emptyReport bool, filename string) {
	if !emptyReport {
		a.appLog.Debug("send report")
		if err := a.SendReport(filename); err != nil {
			a.appLog.Error("Error occurred", slog.String("error", err.Error()))
		}
	}
	a.appLog.Debug("remove report")
	if err := os.Remove(filename); err != nil {
		a.appLog.Error("Error occurred", slog.String("error", err.Error()))
	}
	a.appLog.Debug("report filepath", slog.String("path", filename))
	a.appLog.Debug("end")
}
