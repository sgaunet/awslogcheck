package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
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
		"Account: %s\nUserID: %s\nARN: %s\n",
		aws.ToString(identity.Account),
		aws.ToString(identity.UserId),
		aws.ToString(identity.Arn),
	)
}

func main() {
	var cfg aws.Config // Configuration to connect to AWS API
	var groupName, ssoProfile string
	var err error
	var lastPeriodToWatch int

	// Treat args
	flag.StringVar(&groupName, "g", "", "LogGroup to parse")
	flag.StringVar(&ssoProfile, "p", "", "Auth by SSO")
	flag.IntVar(&lastPeriodToWatch, "t", 600, "Time in s")
	flag.Parse()

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

	clientCloudwatchlogs := cloudwatchlogs.NewFromConfig(cfg)

	doesGroupNameExists := findLogGroup(clientCloudwatchlogs, groupName, "")
	if !doesGroupNameExists {
		fmt.Printf("GroupName %s not found\n.", groupName)
		os.Exit(1)
	}

	minTimeStamp := (time.Now().Unix() - int64(lastPeriodToWatch)) * 1000
	parseAllStreamsOfGroup(clientCloudwatchlogs, groupName, "", minTimeStamp)
}

func getEvents(groupName string, streamName string, client *cloudwatchlogs.Client, context context.Context) {
	now := time.Now().Unix() * 1000
	start := now - 60000000
	input := cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  &groupName,
		LogStreamName: &streamName,
		EndTime:       &now,
		StartTime:     &start,
	}

	res, err := client.GetLogEvents(context, &input)
	if err != nil {
		fmt.Println("Error", err.Error())
		os.Exit(1)
	}

	containerNamePrinted := false
	for _, k := range res.Events {
		// fmt.Println("##", *k.Message)
		var lineOfLog fluentDockerLog
		err := json.Unmarshal([]byte(*k.Message), &lineOfLog)
		if err != nil {
			fmt.Println("error numarshall")
		}
		// fmt.Println("LOG=>", toto.Log)
		rules, err := loadRules("rules")
		if err != nil {
			panic(err)
			os.Exit(1)
		}
		if !isLineMatchWithOneRule(lineOfLog.Log, rules) {
			if !containerNamePrinted {
				fmt.Printf("Parse stream : %s\n", streamName)
				fmt.Printf("container image ==> %s\n", lineOfLog.Kubernetes.ContainerImage)
				containerNamePrinted = true
			}
			fmt.Printf("LINE: %s\n", lineOfLog.Log)
		}
	}
	if containerNamePrinted {
		fmt.Println("")
	}
}
