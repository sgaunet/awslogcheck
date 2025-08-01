package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sgaunet/awslogcheck/internal/app"
	"github.com/sgaunet/awslogcheck/internal/configapp"
	"github.com/sgaunet/awslogcheck/internal/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/robfig/cron"
)

const ingestionTimeS int = 120

func initTrace(debugLevel string) *slog.Logger {
	appLog := logger.NewLogger(debugLevel)
	return appLog
}

func checkErrorAndExitIfErr(err error, logger *slog.Logger) {
	if err != nil {
		logger.Error("error occured", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// print AWS identity
func printID(cfg aws.Config, logger *slog.Logger) {
	client := sts.NewFromConfig(cfg)
	identity, err := client.GetCallerIdentity(
		context.TODO(),
		&sts.GetCallerIdentityInput{},
	)
	checkErrorAndExitIfErr(err, logger)
	logger.Info("", slog.String("account", aws.ToString(identity.Account)))
	logger.Info("", slog.String("userID", aws.ToString(identity.UserId)))
	logger.Info("", slog.String("arn", aws.ToString(identity.Arn)))
}

var version string = "development"
var application *app.App
var awsCfg aws.Config // Configuration to connect to AWS API
var appCtx context.Context

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
	appLog := initTrace("")

	// Treat args
	flag.BoolVar(&vOption, "v", false, "Get version")
	flag.StringVar(&ssoProfile, "p", "", "Auth by SSO")
	flag.StringVar(&configFilename, "c", "", "Directory containing patterns to ignore")
	flag.Parse()

	if vOption {
		printVersion()
		os.Exit(0)
	}

	if configFilename == "" {
		fmt.Fprintf(os.Stderr, "ERROR: configuration file is mandatory\n")
		os.Exit(1)
	}
	if configFilename != "" {
		configApp, err = configapp.ReadYamlCnxFile(configFilename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err.Error())
			appLog.Error("Cannot read configuration file", slog.String("filename", configFilename))
			appLog.Error("Please check the file and try again")
			os.Exit(1)
		}
	}

	appLog = initTrace(configApp.DebugLevel)
	appLog.Info("Log level set", slog.String("level", configApp.DebugLevel))
	appLog.Debug("Log group configured", slog.String("loggroup", configApp.LogGroup))

	appCtx = context.Background()
	appCtx, cancel := context.WithCancel(appCtx)

	// No profile selected
	if len(ssoProfile) == 0 {
		awsCfg, err = config.LoadDefaultConfig(context.TODO(), config.WithRegion("eu-west-3"))
		checkErrorAndExitIfErr(err, appLog)
	} else {
		// Try to connect with the SSO profile put in parameter
		awsCfg, err = config.LoadDefaultConfig(
			context.TODO(),
			config.WithSharedConfigProfile(ssoProfile),
		)
		checkErrorAndExitIfErr(err, appLog)
	}
	printID(awsCfg, appLog)
	application = app.New(appCtx, configApp, awsCfg, 3600, appLog) // 3600 is the number of second since now to parse logs

	err = application.LoadRules()
	if err != nil {
		appLog.Error("error occured", slog.String("error", err.Error()))
		appLog.Error("Cannot load rules...")
		os.Exit(1)
	}

	if ssoProfile != "" {
		appLog.Info("Using SSO profile", slog.String("profile", ssoProfile))
		mainRoutine()
		os.Exit(0)
	}

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
	application.GetLogger().Debug("Start Logcheck")
	err := application.LogCheck(appCtx)
	if err != nil {
		application.GetLogger().Error(err.Error())
		os.Exit(1)
	}
	application.GetLogger().Debug("End Logcheck")
}
