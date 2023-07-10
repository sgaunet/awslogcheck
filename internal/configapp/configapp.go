package configapp

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const RULESDIR = "rules"

type AppConfig struct {
	RulesDir              string            `yaml:"rulesdir"`
	ImagesToIgnore        []string          `yaml:"imagesToIgnore"`
	ContainerNameToIgnore []string          `yaml:"containerNameToIgnore"`
	SmtpConfig            smtpConfig        `yaml:"smtp"`
	MailgunConfig         MailGunConfig     `yaml:"mailgun"`
	MailConfig            MailConfiguration `yaml:"mailconfiguration"`
	AwsRegion             string            `yaml:"aws_region"`
	LogGroup              string            `yaml:"loggroup"`
	DebugLevel            string            `yaml:"debuglevel"`
}

type MailConfiguration struct {
	FromEmail string `yaml:"from_email"`
	// realname: Production
	Sendto  string `yaml:"sendto"`
	Subject string `yaml:"subject"`
}

type MailGunConfig struct {
	Domain string `yaml:"domain"`
	ApiKey string `yaml:"apikey"`
}

type smtpConfig struct {
	Server        string `yaml:"server"`
	Port          int    `yaml:"port"`
	Login         string `yaml:"login"`
	Password      string `yaml:"password"`
	Tls           bool   `yaml:"tls"`
	MaxReportSize int    `yaml:"maxreportsize"`
}

func ReadYamlCnxFile(filename string) (AppConfig, error) {
	var config AppConfig

	yamlFile, err := os.ReadFile(filename)
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

func (a *AppConfig) IsMailGunConfigured() bool {
	return a.MailgunConfig.ApiKey != "" && a.MailgunConfig.Domain != ""
}

func (a *AppConfig) IsSmtpConfigured() bool {
	return a.SmtpConfig.Login != "" && a.SmtpConfig.Port != 0 && a.SmtpConfig.Password != "" && a.SmtpConfig.Server != ""
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
