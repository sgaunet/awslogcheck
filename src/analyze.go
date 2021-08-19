package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
)

func loadRules(dirRules string) ([]string, error) {
	var rules []string
	err := filepath.Walk(dirRules,
		func(pathitem string, info os.FileInfo, err error) error {

			if !info.IsDir() {
				ruleFile, _ := os.Open(pathitem)
				defer ruleFile.Close()
				scanner := bufio.NewScanner(ruleFile)
				scanner.Split(bufio.ScanLines)

				for scanner.Scan() {
					//fmt.Println(scanner.Text())
					rules = append(rules, scanner.Text())
				}

			}
			return err
		})

	var rules2 []string
	for _, line := range rules {
		r := regexp.MustCompile("^\n$")
		if !r.Match([]byte(line)) {
			rules2 = append(rules2, line)
		}
	}

	return rules2, err
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
