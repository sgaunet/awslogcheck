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
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

type App struct {
	cfg               AppConfig
	rules             []string
	lastPeriodToWatch int
}

func New(cfg AppConfig, lastPeriodToWatch int) *App {
	app := App{cfg: cfg,
		lastPeriodToWatch: lastPeriodToWatch}
	return &app
}

// Try to load regexp rules that will be used to ignore events (log)
func (a *App) LoadRules() error {
	//var rules []string
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

// getEvents parse events of a stream and print results that do not match with any rules on stdout
func (a *App) getEvents(context context.Context, groupName string, streamName string, client *cloudwatchlogs.Client) {
	now := time.Now().Unix() * 1000
	start := now - int64((a.lastPeriodToWatch * 1000))
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
		// rules, err := loadRules("rules")
		// if err != nil {
		// 	panic(err)
		// 	os.Exit(1)
		// }
		if !isLineMatchWithOneRule(lineOfLog.Log, a.rules) {
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

func isLineMatchWithOneRule(line string, rules []string) bool {
	for _, rule := range rules {
		//fmt.Printf("rule=%s\n", rule)
		r := regexp.MustCompile(rule)

		if r.MatchString(line) {
			//fmt.Printf("MATCH rule %s // line %s\n", rule, line)
			return true
		}
	}
	//fmt.Printf("line %s MATHC AUCUNE REGLE\n", line)
	return false
}
