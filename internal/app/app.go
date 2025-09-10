// Package app provides the core application logic for awslogcheck.
package app

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/sgaunet/awslogcheck/internal/configapp"
	mailgunservice "github.com/sgaunet/awslogcheck/internal/mailservice/mailgunService"
	smtpservice "github.com/sgaunet/awslogcheck/internal/mailservice/smtpService"
	"golang.org/x/time/rate"
)

// App represents the main application structure.
type App struct {
	cfg               configapp.AppConfig
	awscfg            aws.Config
	rules             []string
	lastPeriodToWatch int
	appLog            *slog.Logger
	eventsRateLimit   *rate.Limiter
	logGroupRateLimit *rate.Limiter
}

// AWS CloudWatch Logs API rate limits
// https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/cloudwatch_limits_cwl.html
const maxEventsAPICallPerSecond = 25   // FilterLogEvents, GetLogEvents calls per second
const maxLogGroupAPICallPerSecond = 10 // DescribeLogGroups calls per second

// Other constants.
const memoryStatsSleepSeconds = 5
const bytesToKB = 1024

// New creates a new App instance.
func New(_ context.Context, cfg configapp.AppConfig, awscfg aws.Config, lastPeriodToWatch int, log *slog.Logger) *App {
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

// GetLogger returns the logger instance.
func (a *App) GetLogger() *slog.Logger {
	return a.appLog
}

// LoadRules loads regexp rules that will be used to ignore events (log).
func (a *App) LoadRules() error {
	rulesDir, err := a.cfg.GetRulesDir()
	if err != nil {
		return fmt.Errorf("failed to get rules directory: %w", err)
	}
	if rulesDir == "" {
		return fmt.Errorf("%w", ErrNoRulesFolder)
	}

	_, err = os.Stat(rulesDir)
	if err != nil {
		return fmt.Errorf("failed to stat rules directory: %w", err)
	}
	err = filepath.Walk(rulesDir,
		func(pathitem string, info os.FileInfo, err error) error {
			if !info.IsDir() {
				// #nosec G304 - pathitem is from filepath.Walk of trusted rules directory
				ruleFile, err := os.Open(pathitem)
				if err != nil {
					return fmt.Errorf("failed to open rule file: %w", err)
				}
				defer func() {
					if closeErr := ruleFile.Close(); closeErr != nil {
						a.appLog.Error("Failed to close rule file", slog.String("error", closeErr.Error()))
					}
				}()
				scanner := bufio.NewScanner(ruleFile)
				scanner.Split(bufio.ScanLines)

				for scanner.Scan() {
					// fmt.Println("====", scanner.Text())
					a.rules = append(a.rules, scanner.Text())
				}
			}
			return err
		})

	if err != nil {
		return fmt.Errorf("failed to walk rules directory: %w", err)
	}
	return nil
}


// PrintMemoryStats prints memory statistics periodically until stopped.
func (a *App) PrintMemoryStats(stop <-chan interface{}) {
	for {
		select {
		case <-stop:
			return
		default:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			a.appLog.Debug("Memory stats",
				slog.Uint64("alloc", m.Alloc/bytesToKB),
				slog.Uint64("totalAlloc", m.TotalAlloc/bytesToKB),
				slog.Uint64("sys", m.Sys/bytesToKB),
				slog.Int("numGC", int(m.NumGC)))
			time.Sleep(memoryStatsSleepSeconds * time.Second)
		}
	}
}

// SendReport sends the report file via email using configured mail service.
func (a *App) SendReport(freport string) error {
	// #nosec G304 - freport is a controlled temp file path from internal function
	body, err := os.ReadFile(freport)
	if err != nil {
		return fmt.Errorf("failed to read report file: %w", err)
	}
	if a.cfg.IsMailGunConfigured() {
		a.appLog.Debug("Mail with mailgun")
		mailgunSvc, err := mailgunservice.NewMailgunService(a.cfg.MailgunConfig.Domain, a.cfg.MailgunConfig.APIKey)
		if err != nil {
			return fmt.Errorf("failed to create mailgun service: %w", err)
		}
		err = mailgunSvc.Send(a.cfg.MailConfig.FromEmail,
			a.cfg.MailConfig.FromEmail,
			a.cfg.MailConfig.Subject, string(body),
			a.cfg.MailConfig.Sendto)
		if err != nil {
			return fmt.Errorf("failed to send email via mailgun: %w", err)
		}
	}
	if a.cfg.IsSMTPConfigured() {
		a.appLog.Debug("Mail with smtp")
		smtpsvc, err := smtpservice.NewSMTPService(a.cfg.SMTPConfig.Login, a.cfg.SMTPConfig.Password,
			fmt.Sprintf("%s:%d", a.cfg.SMTPConfig.Server, a.cfg.SMTPConfig.Port), a.cfg.SMTPConfig.TLS)
		if err != nil {
			return fmt.Errorf("failed to create smtp service: %w", err)
		}
		err = smtpsvc.Send(a.cfg.MailConfig.FromEmail,
			a.cfg.MailConfig.FromEmail,
			a.cfg.MailConfig.Subject, string(body),
			a.cfg.MailConfig.Sendto)
		if err != nil {
			return fmt.Errorf("failed to send email via smtp: %w", err)
		}
	}
	return nil
}

func (a *App) isImageIgnored(imageToCheck string) bool {
	for _, imgToIgnore := range a.cfg.ImagesToIgnore {
		r := regexp.MustCompile(imgToIgnore)
		if r.MatchString(imageToCheck) {
			a.appLog.Debug("Image match", slog.String("imageToCheck", imageToCheck), slog.String("imgToIgnore", imgToIgnore))
			return true
		}
		a.appLog.Debug("Image no match", slog.String("imageToCheck", imageToCheck), slog.String("imgToIgnore", imgToIgnore))
	}
	return false
}

func (a *App) isContainerIgnored(containerToCheck string) bool {
	for _, containerToIgnore := range a.cfg.ContainerNameToIgnore {
		r := regexp.MustCompile(containerToIgnore)
		if r.MatchString(containerToCheck) {
			// fmt.Printf("%s compared to %s : MATCH\n", containerToCheck, containerToIgnore)
			a.appLog.Debug("Container match",
				slog.String("containerToCheck", containerToCheck),
				slog.String("containerToIgnore", containerToIgnore))
			return true
		}
		// 	fmt.Printf("%s compared to %s : DO NOT MATCH\n", containerToCheck, containerToIgnore)
		a.appLog.Debug("Container no match",
			slog.String("containerToCheck", containerToCheck),
			slog.String("containerToIgnore", containerToIgnore))
	}
	return false
}

func (a *App) isLineMatchWithOneRule(line string, rules []string) bool {
	for _, rule := range rules {
		r, err := regexp.Compile(rule)
		if err != nil {
			a.appLog.Error("rule is incorrect", slog.String("rule", rule))
		} else if r.MatchString(line) {
			a.appLog.Debug("Rule match", slog.String("rule", rule), slog.String("line", line))
			return true
		}
	}
	a.appLog.Debug("Line matches no rules", slog.String("line", line))
	return false
}
