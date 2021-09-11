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
)

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

	if vOption {
		printVersion()
		os.Exit(0)
	}

	if configFilename != "" {
		configApp, err = app.ReadYamlCnxFile(configFilename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err.Error())
			os.Exit(1)
		}
	}
	app := app.New(configApp, lastPeriodToWatch)

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
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "Cannot load rules...")
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

	fmt.Printf("cptLinePrinted=%d\n", cptLinePrinted)

}
