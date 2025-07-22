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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/sgaunet/awslogcheck/internal/configapp"
	mailgunservice "github.com/sgaunet/awslogcheck/internal/mailservice/mailgunService"
	smtpservice "github.com/sgaunet/awslogcheck/internal/mailservice/smtpService"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

type App struct {
	cfg               configapp.AppConfig
	awscfg            aws.Config
	rules             []string
	lastPeriodToWatch int
	appLog            *logrus.Logger
	eventsRateLimit   *rate.Limiter
	logGroupRateLimit *rate.Limiter
}

// AWS CloudWatch Logs API rate limits
// https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/cloudwatch_limits_cwl.html
const maxEventsAPICallPerSecond = 25    // FilterLogEvents, GetLogEvents calls per second
const maxLogGroupAPICallPerSecond = 10   // DescribeLogGroups calls per second

func New(ctx context.Context, cfg configapp.AppConfig, awscfg aws.Config, lastPeriodToWatch int, log *logrus.Logger) *App {
	app := App{
		cfg:               cfg,
		awscfg:            awscfg,
		lastPeriodToWatch: lastPeriodToWatch,
		appLog:            log,
		eventsRateLimit:   rate.NewLimiter(rate.Limit(maxEventsAPICallPerSecond), maxEventsAPICallPerSecond),
		logGroupRateLimit: rate.NewLimiter(rate.Limit(maxLogGroupAPICallPerSecond), maxLogGroupAPICallPerSecond),
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
		return errors.New("no rules folder found")
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
func (a *App) getEvents(context context.Context, groupName string, streamName string, client *cloudwatchlogs.Client, chLogLines chan<- string, nextToken string) (cptLinePrinted int) {
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

	if err := a.eventsRateLimit.Wait(context); err != nil {
		a.appLog.Errorln("Rate limit error:", err.Error())
		return cptLinePrinted
	}
	res, err := client.GetLogEvents(context, &input)
	if err != nil {
		a.appLog.Errorln("Error", err.Error())
		os.Exit(1)
	}

	containerNamePrinted := false
	for _, k := range res.Events {
		var lineOfLog fluentDockerLog
		// a.appLog.Debugln("*k.message             =", *k.Message)
		err := json.Unmarshal([]byte(*k.Message), &lineOfLog)
		if err != nil {
			a.appLog.Errorln(err.Error())
		}
		// a.appLog.Debugf("ContainerName=%v  ContainerImage=%v\n", lineOfLog.Kubernetes.ContainerName, lineOfLog.Kubernetes.ContainerImage)
		var hasBeenChecked, imageIgnored, containerToIgnore bool
		if !isLineMatchWithOneRule(lineOfLog.Log, a.rules) {
			if !hasBeenChecked {
				imageIgnored = a.isImageIgnored(lineOfLog.Kubernetes.ContainerImage)
				containerToIgnore = a.isContainerIgnored(lineOfLog.Kubernetes.ContainerName)
				hasBeenChecked = true
			}
			// a.appLog.Debugf("imageIgnored=%v containerToIgnore=%v\n", imageIgnored, containerToIgnore)
			if !imageIgnored && !containerToIgnore {
				if !containerNamePrinted {
					// fmt.Printf("**Parse stream** : %s\n", streamName)
					// fmt.Printf("**container image** : %s\n", lineOfLog.Kubernetes.ContainerImage)
					// fmt.Printf("**container name** : %s\n", lineOfLog.Kubernetes.ContainerName)
					a.appLog.Debugf("Parse stream=%v containerImage=%v containerName=%v\n", streamName, lineOfLog.Kubernetes.ContainerImage, lineOfLog.Kubernetes.ContainerName)
					chLogLines <- "<b>Parse stream</b> :" + streamName + "<br>"
					chLogLines <- "<b>Container Image</b> :" + lineOfLog.Kubernetes.ContainerImage + "<br>"
					chLogLines <- "<b>Container Name</b> :" + lineOfLog.Kubernetes.ContainerName + "<br>"
					containerNamePrinted = true
				}
				timeT := time.Unix(*k.Timestamp/1000, 0).UTC()
				chLogLines <- fmt.Sprintf("%s UTC: %s<br>\n", timeT.Format("2006-01-02 15:04:05"), lineOfLog.Log)
				cptLinePrinted++
			} else {
				a.appLog.Debugln("Log of this image can be ignored so stop the loop over events")
				break
			}
		}
	}
	if containerNamePrinted {
		chLogLines <- "<br>\n"
	}
	if *res.NextBackwardToken != nextToken {
		time.Sleep(100 * time.Millisecond)
		return cptLinePrinted + a.getEvents(context, groupName, streamName, client, chLogLines, *res.NextBackwardToken)
	}
	return cptLinePrinted
}

func (a *App) isImageIgnored(imageToCheck string) bool {
	for _, imgToIgnore := range a.cfg.ImagesToIgnore {
		r := regexp.MustCompile(imgToIgnore)
		if r.MatchString(imageToCheck) {
			a.appLog.Debugf("%s compared to %s : MATCH\n", imageToCheck, imgToIgnore)
			return true
		} else {
			a.appLog.Debugf("%s compared to %s : DO NOT MATCH\n", imageToCheck, imgToIgnore)
		}
	}
	return false
}

func (a *App) isContainerIgnored(containerToCheck string) bool {
	for _, containerToIgnore := range a.cfg.ContainerNameToIgnore {
		r := regexp.MustCompile(containerToIgnore)
		if r.MatchString(containerToCheck) {
			// fmt.Printf("%s compared to %s : MATCH\n", containerToCheck, containerToIgnore)
			a.appLog.Debugf("%s compared to %s : MATCH\n", containerToCheck, containerToIgnore)
			return true
		} else {
			// 	fmt.Printf("%s compared to %s : DO NOT MATCH\n", containerToCheck, containerToIgnore)
			a.appLog.Debugf("%s compared to %s : DO NOT MATCH\n", containerToCheck, containerToIgnore)
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
		r, err := regexp.Compile(rule)
		if err != nil {
			logrus.Errorf("rule %s is incorrect", rule)
		} else {
			if r.MatchString(line) {
				logrus.Debugf("MATCH rule %s // line %s\n", rule, line)
				return true
			}
		}
	}
	logrus.Debugf("line %s MATCH NO RULES\n", line)
	return false
}

func (a *App) SendReport(freport string) error {
	body, err := os.ReadFile(freport)
	if err != nil {
		return err
	}
	if a.cfg.IsMailGunConfigured() {
		logrus.Debugln("Mail with mailgun")
		mailgunSvc, err := mailgunservice.NewMailgunService(a.cfg.MailgunConfig.Domain, a.cfg.MailgunConfig.ApiKey)
		if err != nil {
			return err
		}
		err = mailgunSvc.Send(a.cfg.MailConfig.FromEmail,
			a.cfg.MailConfig.FromEmail,
			a.cfg.MailConfig.Subject, string(body),
			a.cfg.MailConfig.Sendto)
		if err != nil {
			return err
		}
	}
	if a.cfg.IsSmtpConfigured() {
		logrus.Debugln("Mail with smtp")
		smtpsvc, err := smtpservice.NewSmtpService(a.cfg.SmtpConfig.Login, a.cfg.SmtpConfig.Password,
			fmt.Sprintf("%s:%d", a.cfg.SmtpConfig.Server, a.cfg.SmtpConfig.Port), a.cfg.SmtpConfig.Tls)
		if err != nil {
			return err
		}
		err = smtpsvc.Send(a.cfg.MailConfig.FromEmail,
			a.cfg.MailConfig.FromEmail,
			a.cfg.MailConfig.Subject, string(body),
			a.cfg.MailConfig.Sendto)
		if err != nil {
			return err
		}
	}
	return nil
}
