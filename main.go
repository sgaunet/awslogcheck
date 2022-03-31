package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sgaunet/awslogcheck/app"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/robfig/cron"
	"github.com/sirupsen/logrus"
)

const ingestionTimeS int = 120

func initTrace(debugLevel string) *logrus.Logger {
	appLog := logrus.New()
	// Log as JSON instead of the default ASCII formatter.
	//log.SetFormatter(&log.JSONFormatter{})
	appLog.SetFormatter(&logrus.TextFormatter{
		DisableColors:    false,
		FullTimestamp:    false,
		DisableTimestamp: true,
	})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	appLog.SetOutput(os.Stdout)

	switch debugLevel {
	case "info":
		appLog.SetLevel(logrus.InfoLevel)
	case "warn":
		appLog.SetLevel(logrus.WarnLevel)
	case "error":
		appLog.SetLevel(logrus.ErrorLevel)
	default:
		appLog.SetLevel(logrus.DebugLevel)
	}
	return appLog
}

func checkErrorAndExitIfErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR-: %s\n", err.Error())
		os.Exit(1)
	}
}

// print AWS identity
func printID(cfg aws.Config) {
	client := sts.NewFromConfig(cfg)
	identity, err := client.GetCallerIdentity(
		context.TODO(),
		&sts.GetCallerIdentityInput{},
	)
	checkErrorAndExitIfErr(err)
	fmt.Printf(
		"Account: %s\nUserID: %s\nARN: %s\n\n",
		aws.ToString(identity.Account),
		aws.ToString(identity.UserId),
		aws.ToString(identity.Arn),
	)
}

var version string = "development"

func printVersion() {
	fmt.Println(version)
}

func main() {
	var cptLinePrinted int
	var cfg aws.Config // Configuration to connect to AWS API
	var vOption bool
	var groupName, ssoProfile string
	var err error
	var configFilename string
	var configApp app.AppConfig
	sigs := make(chan os.Signal, 1)

	// Treat args
	flag.BoolVar(&vOption, "v", false, "Get version")
	flag.StringVar(&groupName, "g", "", "LogGroup to parse")
	flag.StringVar(&ssoProfile, "p", "", "Auth by SSO")
	flag.StringVar(&configFilename, "c", "", "Directory containing patterns to ignore")
	flag.Parse()

	appLog := initTrace(os.Getenv("DEBUGLEVEL"))
	appLog.Infoln("appLog.Level=", appLog.Level)

	if vOption {
		printVersion()
		os.Exit(0)
	}

	if configFilename != "" {
		configApp, err = app.ReadYamlCnxFile(configFilename)
		if err != nil {
			logrus.Errorf("ERROR: %v\n", err.Error())
			os.Exit(1)
		}
	}
	app := app.New(configApp, 3600, appLog) // 3600 is the number of second since now to parse logs

	// No profile selected
	if len(ssoProfile) == 0 {
		cfg, err = config.LoadDefaultConfig(context.TODO(), config.WithRegion("eu-west-3"))
		checkErrorAndExitIfErr(err)
	} else {
		// Try to connect with the SSO profile put in parameter
		cfg, err = config.LoadDefaultConfig(
			context.TODO(),
			config.WithSharedConfigProfile(ssoProfile),
		)
		checkErrorAndExitIfErr(err)
	}
	printID(cfg)

	err = app.LoadRules()
	if err != nil {
		logrus.Errorln(err)
		logrus.Errorln("Cannot load rules...")
		os.Exit(1)
	}

	if groupName == "" {
		// groupname not specified in command line, try to get it with env var
		groupName = os.Getenv("LOGGROUP")
		if groupName == "" {
			logrus.Errorln("No LOGGROUP specified. Set LOGGROUP env var or use the option -g")
			os.Exit(1)
		}
	}

	c := cron.New()
	// second minute hour day month
	c.AddFunc("0 0 * * *", func() {
		freport := "/tmp/report.html"
		time.Sleep(time.Duration(ingestionTimeS) * time.Second) // Wait the time for the ingestion time of logs

		cptLinePrinted, err = app.LogCheck(cfg, groupName, freport)
		if err != nil {
			logrus.Errorln(err.Error())
			os.Exit(1)
		}
		if cptLinePrinted == 0 {
			logrus.Infof("Every logs have been filtered (%s)\n", groupName)
		} else {
			body, err := ioutil.ReadFile(freport)
			if err != nil {
				logrus.Errorln(err.Error())
			}
			if isMailGunConfigured(os.Getenv("MAILGUN_DOMAIN"), os.Getenv("MAILGUN_APIKEY")) {
				err = sendMailWithMailgun(os.Getenv("MAILGUN_DOMAIN"), os.Getenv("MAILGUN_APIKEY"), os.Getenv("FROM_EMAIL"), os.Getenv("SUBJECT"), string(body), os.Getenv("MAILTO"))
				if err != nil {
					logrus.Errorln(err.Error())
				}
			}
			if isSmtpConfigured(os.Getenv("SMTP_LOGIN"), os.Getenv("SMTP_PASSWORD"), os.Getenv("SMTP_SERVER")+":"+os.Getenv("SMTP_PORT")) {
				err = sendSmtpMail(os.Getenv("FROM_EMAIL"), os.Getenv("MAILTO"), os.Getenv("SUBJECT"), string(body), os.Getenv("SMTP_LOGIN"), os.Getenv("SMTP_PASSWORD"), os.Getenv("SMTP_SERVER")+":"+os.Getenv("SMTP_PORT"), true)
				if err != nil {
					logrus.Errorln(err.Error())
				}
			}
		}
		err = os.Remove(freport)
		if err != nil {
			logrus.Errorln(err.Error())
		}
	})
	c.Start()
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	c.Stop()
}
