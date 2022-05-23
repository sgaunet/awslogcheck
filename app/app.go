package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/sgaunet/ratelimit"
	"github.com/sirupsen/logrus"
)

type App struct {
	cfg               AppConfig
	rules             []string
	lastPeriodToWatch int
	appLog            *logrus.Logger
	rateLimit         *ratelimit.RateLimit
}

const awsCloudWatchRateLimit = 20

func New(cfg AppConfig, lastPeriodToWatch int, log *logrus.Logger) *App {
	app := App{cfg: cfg,
		lastPeriodToWatch: lastPeriodToWatch,
		appLog:            log,
		rateLimit:         ratelimit.New(1*time.Second, awsCloudWatchRateLimit),
	}
	return &app
}

// Try to load regexp rules that will be used to ignore events (log)
func (a *App) LoadRules() error {
	rulesDir, err := a.cfg.GetRulesDir()
	if err != nil {
		return err
	}
	if rulesDir == "" {
		return errors.New("No rules folder found")
	}

	_, err = os.Stat(rulesDir)
	if err != nil {
		return err
	}
	err = filepath.Walk(rulesDir,
		func(pathitem string, info os.FileInfo, err error) error {

			if !info.IsDir() {
				ruleFile, _ := os.Open(pathitem)
				defer ruleFile.Close()
				scanner := bufio.NewScanner(ruleFile)
				scanner.Split(bufio.ScanLines)

				for scanner.Scan() {
					//fmt.Println("====", scanner.Text())
					a.rules = append(a.rules, scanner.Text())
				}

			}
			return err
		})

	return err
}

// getEvents parse events of a stream and return results that do not match with any rules on stdout
func (a *App) getEvents(context context.Context, groupName string, streamName string, client *cloudwatchlogs.Client, f *os.File, nextToken string) (cptLinePrinted int) {
	now := time.Now().Unix() * 1000
	start := now - int64((a.lastPeriodToWatch * 1000))
	input := cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  &groupName,
		LogStreamName: &streamName,
		EndTime:       &now,
		StartTime:     &start,
	}

	if nextToken == "" {
		input.NextToken = nil
		a.appLog.Debugln("getEvents", groupName, streamName)
	} else {
		input.NextToken = &nextToken
		a.appLog.Debugln("getEvents", groupName, streamName, nextToken)
	}

	a.rateLimit.WaitIfLimitReached()
	res, err := client.GetLogEvents(context, &input)
	if err != nil {
		a.appLog.Errorln("Error", err.Error())
		os.Exit(1)
	}

	containerNamePrinted := false
	for _, k := range res.Events {
		var lineOfLog fluentDockerLog
		err := json.Unmarshal([]byte(*k.Message), &lineOfLog)
		if err != nil {
			a.appLog.Errorln(err.Error())
			f.WriteString(err.Error())
		}
		var hasBeenChecked, imageIgnored, containerToIgnore bool
		if !isLineMatchWithOneRule(lineOfLog.Log, a.rules) {
			if !hasBeenChecked {
				imageIgnored = a.isImageIgnored(lineOfLog.Kubernetes.ContainerImage)
				containerToIgnore = a.isContainerIgnored(lineOfLog.Kubernetes.ContainerName)
				hasBeenChecked = true
			}
			if !imageIgnored && !containerToIgnore {
				if !containerNamePrinted {
					// fmt.Printf("**Parse stream** : %s\n", streamName)
					// fmt.Printf("**container image** : %s\n", lineOfLog.Kubernetes.ContainerImage)
					// fmt.Printf("**container name** : %s\n", lineOfLog.Kubernetes.ContainerName)
					f.WriteString("<b>Parse stream</b> :" + streamName + "<br>")
					f.WriteString("<b>Container Image</b> :" + lineOfLog.Kubernetes.ContainerImage + "<br>")
					f.WriteString("<b>Container Name</b> :" + lineOfLog.Kubernetes.ContainerName + "<br>")
					containerNamePrinted = true
				}
				timeT := time.Unix(*k.Timestamp/1000, 0)
				// fmt.Printf("%s: %s\n", timeT, lineOfLog.Log)
				f.WriteString(fmt.Sprintf("%s: %s<br>\n", timeT, lineOfLog.Log))
				cptLinePrinted++
			} else {
				// Log of this image can be ignored so stop the loop over events
				break
			}
		}
	}
	if containerNamePrinted {
		// fmt.Println("")
		f.WriteString("<br>\n")
	}
	a.appLog.Debugln("nextToken             =", nextToken)
	// a.appLog.Debugln("*res.NextForwardToken =", *res.NextForwardToken)
	a.appLog.Debugln("*res.NextBackwardToken=", *res.NextBackwardToken)

	if *res.NextBackwardToken != nextToken {
		time.Sleep(100 * time.Millisecond)
		return cptLinePrinted + a.getEvents(context, groupName, streamName, client, f, *res.NextBackwardToken)
	}
	return cptLinePrinted
}

func (a *App) isImageIgnored(imageToCheck string) bool {
	for _, imgToIgnore := range a.cfg.ImagesToIgnore {
		r := regexp.MustCompile(imgToIgnore)
		if r.MatchString(imageToCheck) {
			// fmt.Printf("%s compared to %s : MATCH\n", imageToCheck, imgToIgnore)
			return true
			// } else {
			// 	fmt.Printf("%s compared to %s : DO NOT MATCH\n", imageToCheck, imgToIgnore)
		}
	}
	return false
}

func (a *App) isContainerIgnored(containerToCheck string) bool {
	for _, containerToIgnore := range a.cfg.ContainerNameToIgnore {
		r := regexp.MustCompile(containerToIgnore)
		if r.MatchString(containerToCheck) {
			// fmt.Printf("%s compared to %s : MATCH\n", containerToCheck, containerToIgnore)
			return true
			// } else {
			// 	fmt.Printf("%s compared to %s : DO NOT MATCH\n", containerToCheck, containerToIgnore)
		}
	}
	return false
}

func (a *App) PrintMemoryStats(stop <-chan interface{}) {
	for {
		select {
		case <-stop:
			return
		default:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			a.appLog.Debugf("\nAlloc = %v\nTotalAlloc = %v\nSys = %v\nNumGC = %v\n\n", m.Alloc/1024, m.TotalAlloc/1024, m.Sys/1024, m.NumGC)
			time.Sleep(5 * time.Second)
		}
	}
}

func isLineMatchWithOneRule(line string, rules []string) bool {
	for _, rule := range rules {
		//fmt.Printf("rule=%s\n", rule)
		r := regexp.MustCompile(rule)

		if r.MatchString(line) {
			logrus.Debugf("MATCH rule %s // line %s\n", rule, line)
			return true
		}
	}
	logrus.Debugf("line %s MATCH NO RULES\n", line)
	return false
}
