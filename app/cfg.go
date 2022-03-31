package app

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const RULESDIR = "rules"

type AppConfig struct {
	RulesDir              string        `yaml:"rulesdir"`
	ImagesToIgnore        []string      `yaml:"imagesToIgnore"`
	ContainerNameToIgnore []string      `yaml:"containerNameToIgnore"`
	SmtpConfig            smtpConfig    `yaml:"smtp"`
	MailgunConfig         MailGunConfig `yaml:"mailgun"`
}

type MailGunConfig struct {
	Domain string `yaml:"domain"`
	ApiKey string `yaml:"apikey"`
}

type smtpConfig struct {
	Server   string `yaml:"server"`
	Port     int    `yaml:"port"`
	Login    string `yaml:"login"`
	Password string `yaml:"password"`
	Tls      bool   `yaml:"tls"`
}

func ReadYamlCnxFile(filename string) (AppConfig, error) {
	var config AppConfig

	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		logrus.Errorf("Error reading YAML file: %s\n", err)
		return config, err
	}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		logrus.Errorf("Error parsing YAML file: %s\n", err)
		return config, err
	}
	return config, err
}

// Return path of rules
// If empty, return the path of the binary/rules
func (cfg *AppConfig) GetRulesDir() (string, error) {
	var err error
	if cfg.RulesDir != "" {
		return cfg.RulesDir, err
	}

	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return "", err
	}

	// Consider that rulesdir is in the same directory of thebinary
	dirToCheck := dir + string(os.PathSeparator) + RULESDIR
	if _, err := os.Stat(dirToCheck); os.IsNotExist(err) {
		return "", errors.New("rules directory not found")
	}
	return dirToCheck, err
}
