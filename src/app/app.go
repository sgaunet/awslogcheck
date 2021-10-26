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
func (a *App) getEvents(context context.Context, groupName string, streamName string, client *cloudwatchlogs.Client) int {
	var cptLinePrinted int
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
		var hasBeenChecked, imageIgnored, containerToIgnore bool
		if !isLineMatchWithOneRule(lineOfLog.Log, a.rules) {
			if !hasBeenChecked {
				imageIgnored = a.isImageIgnored(lineOfLog.Kubernetes.ContainerImage)
				containerToIgnore = a.isContainerIgnored(lineOfLog.Kubernetes.ContainerName)
				hasBeenChecked = true
			}
			if !imageIgnored && !containerToIgnore {
				if !containerNamePrinted {
					fmt.Printf("**Parse stream** : %s\n", streamName)
					fmt.Printf("**container image** : %s\n", lineOfLog.Kubernetes.ContainerImage)
					fmt.Printf("**container name** : %s\n", lineOfLog.Kubernetes.ContainerName)
					// fmt.Println("*******************************************************")
					// fmt.Println(lineOfLog.Kubernetes)
					// fmt.Println("*******************************************************")
					// Tester ici si container fait parti de la liste a ignorer
					containerNamePrinted = true
				}
				timeT := time.Unix(*k.Timestamp/1000, 0)
				fmt.Printf("%s: %s\n", timeT, lineOfLog.Log)
				cptLinePrinted++
			} else {
				// Log of this image can be ignored so stop the loop over events
				break
			}
		}
	}
	if containerNamePrinted {
		fmt.Println("")
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
