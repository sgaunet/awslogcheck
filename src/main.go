package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/sgaunet/awslogcheck/app"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/sirupsen/logrus"
)

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
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err.Error())
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
	var lastPeriodToWatch int
	var configFilename string
	var configApp app.AppConfig

	// Treat args
	flag.BoolVar(&vOption, "v", false, "Get version")
	flag.StringVar(&groupName, "g", "", "LogGroup to parse")
	flag.StringVar(&ssoProfile, "p", "", "Auth by SSO")
	flag.IntVar(&lastPeriodToWatch, "t", 600, "Time in s")
	flag.StringVar(&configFilename, "c", "", "Directory containing patterns to ignore")
	flag.Parse()

	appLog := initTrace(os.Getenv("DEBUGLEVEL"))
	fmt.Println("log level=", appLog.Level)

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
	app := app.New(configApp, lastPeriodToWatch, appLog)

	// No profile selected
	if len(ssoProfile) == 0 {
		cfg, err = config.LoadDefaultConfig(context.TODO())
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

	err, cptLinePrinted = app.LogCheck(cfg, groupName)
	if err != nil {
		fmt.Printf("<span style=\"color:red\">GroupName %s not found </span>", groupName)
	}
	if cptLinePrinted == 0 {
		fmt.Println("<span style=\"color:red\">Every logs have been filtered </span>", groupName)
		os.Exit(200)
	}

	logrus.Infoln("cptLinePrinted=%d\n", cptLinePrinted)

}
