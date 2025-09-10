// Package configapp provides configuration management for awslogcheck.
package configapp

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// RULESDIR is the default directory name for rule files.
const RULESDIR = "rules"

// AppConfig represents the application configuration.
type AppConfig struct {
	RulesDir              string            `yaml:"rulesdir"`
	ImagesToIgnore        []string          `yaml:"imagesToIgnore"`
	ContainerNameToIgnore []string          `yaml:"containerNameToIgnore"`
	SMTPConfig            smtpConfig        `yaml:"smtp"`
	MailgunConfig         MailGunConfig     `yaml:"mailgun"`
	MailConfig            MailConfiguration `yaml:"mailconfiguration"`
	AwsRegion             string            `yaml:"aws_region"`
	LogGroup              string            `yaml:"loggroup"`
	DebugLevel            string            `yaml:"debuglevel"`
}

// MailConfiguration contains email configuration settings.
type MailConfiguration struct {
	FromEmail string `yaml:"from_email"`
	// realname: Production
	Sendto  string `yaml:"sendto"`
	Subject string `yaml:"subject"`
}

// MailGunConfig contains Mailgun-specific configuration.
type MailGunConfig struct {
	Domain string `yaml:"domain"`
	APIKey string `yaml:"apikey"`
}

type smtpConfig struct {
	Server        string `yaml:"server"`
	Port          int    `yaml:"port"`
	Login         string `yaml:"login"`
	Password      string `yaml:"password"`
	TLS           bool   `yaml:"tls"`
	MaxReportSize int    `yaml:"maxreportsize"`
}

// ReadYamlCnxFile reads and parses a YAML configuration file.
func ReadYamlCnxFile(filename string) (AppConfig, error) {
	var config AppConfig

	// #nosec G304 - filename is validated config file path from command line
	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading YAML file: %s\n", err)
		return config, fmt.Errorf("failed to read YAML file: %w", err)
	}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing YAML file: %s\n", err)
		return config, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}
	return config, nil
}

// IsMailGunConfigured checks if Mailgun is properly configured.
func (a *AppConfig) IsMailGunConfigured() bool {
	return a.MailgunConfig.APIKey != "" && a.MailgunConfig.Domain != ""
}

// IsSMTPConfigured checks if SMTP is properly configured.
func (a *AppConfig) IsSMTPConfigured() bool {
	return a.SMTPConfig.Login != "" && a.SMTPConfig.Port != 0 && a.SMTPConfig.Password != "" && a.SMTPConfig.Server != ""
}

// GetRulesDir returns path of rules directory.
// If empty, return the path of the binary/rules.
func (a *AppConfig) GetRulesDir() (string, error) {
	if a.RulesDir != "" {
		return a.RulesDir, nil
	}

	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Consider that rulesdir is in the same directory of thebinary
	dirToCheck := dir + string(os.PathSeparator) + RULESDIR
	if _, err := os.Stat(dirToCheck); os.IsNotExist(err) {
		return "", fmt.Errorf("%w", ErrRulesDirNotFound)
	}
	return dirToCheck, nil
}
