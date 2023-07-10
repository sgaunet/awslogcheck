package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sgaunet/awslogcheck/internal/app"
	"github.com/sgaunet/awslogcheck/internal/configapp"

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
	case "debug":
		appLog.SetLevel(logrus.DebugLevel)
	case "warn":
		appLog.SetLevel(logrus.WarnLevel)
	case "error":
		appLog.SetLevel(logrus.ErrorLevel)
	default:
		appLog.SetLevel(logrus.InfoLevel)
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
var application *app.App
var awsCfg aws.Config // Configuration to connect to AWS API

func printVersion() {
	fmt.Println(version)
}

func main() {
	// var cptLinePrinted int
	var vOption bool
	var ssoProfile string
	var err error
	var configFilename string
	var configApp configapp.AppConfig
	sigs := make(chan os.Signal, 5)

	// Treat args
	flag.BoolVar(&vOption, "v", false, "Get version")
	flag.StringVar(&ssoProfile, "p", "", "Auth by SSO")
	flag.StringVar(&configFilename, "c", "", "Directory containing patterns to ignore")
	flag.Parse()

	if vOption {
		printVersion()
		os.Exit(0)
	}

	if configFilename != "" {
		configApp, err = configapp.ReadYamlCnxFile(configFilename)
		if err != nil {
			logrus.Errorf("ERROR: %v\n", err.Error())
			os.Exit(1)
		}
	} else {
		logrus.Errorln("configuration file is mandatory")
	}
	appLog := initTrace(configApp.DebugLevel)
	appLog.Infoln("appLog.Level=", configApp.DebugLevel)
	appLog.Debugln("loggroup=", configApp.LogGroup)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	// No profile selected
	if len(ssoProfile) == 0 {
		awsCfg, err = config.LoadDefaultConfig(context.TODO(), config.WithRegion("eu-west-3"))
		checkErrorAndExitIfErr(err)
	} else {
		// Try to connect with the SSO profile put in parameter
		awsCfg, err = config.LoadDefaultConfig(
			context.TODO(),
			config.WithSharedConfigProfile(ssoProfile),
		)
		checkErrorAndExitIfErr(err)
	}
	printID(awsCfg)
	application = app.New(ctx, configApp, awsCfg, 3600, appLog) // 3600 is the number of second since now to parse logs

	err = application.LoadRules()
	if err != nil {
		appLog.Errorln(err)
		appLog.Errorln("Cannot load rules...")
		os.Exit(1)
	}

	// mainRoutine() // TO DELETE

	c := cron.New()
	// second minute hour day month
	c.AddFunc("0 0 * * *", mainRoutine)
	c.Start()
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	cancel()
	c.Stop()
	time.Sleep(1 * time.Second)
	// buf := make([]byte, 1<<16)
	// runtime.Stack(buf, true)
	// fmt.Printf("%s", buf)
}

func mainRoutine() {
	time.Sleep(time.Duration(ingestionTimeS) * time.Second) // Wait the time for the ingestion time of logs
	// if debug mode, launch goroutine to print memory stats
	// if os.Getenv("DEBUGLEVEL") == "debug" {
	// 	stop = make(chan interface{})
	// 	go app.PrintMemoryStats(stop)
	// }
	logrus.Debugln("Start Logcheck")
	err := application.LogCheck()
	if err != nil {
		logrus.Errorln(err.Error())
		os.Exit(1)
	}
	logrus.Debugln("End Logcheck")
}
