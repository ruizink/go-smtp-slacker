package config

import (
	"fmt"
	"go-smtp-slacker/internal/logger"
	"go-smtp-slacker/internal/version"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// SMTPConfig holds the SMTP server's settings.
type SMTPConfig struct {
	ListenAddr string     `mapstructure:"listen-addr" validate:"required"`
	Auth       AuthConfig `mapstructure:"auth" validate:"required"`
	Policies   struct {
		From Policy `mapstructure:"from" validate:"required"`
		To   Policy `mapstructure:"to" validate:"required"`
	} `mapstructure:"policies" validate:"required"`
}

// PoliciesConfig holds the policy settings.
type Policy struct {
	Allow         []string `mapstructure:"allow"`
	Deny          []string `mapstructure:"deny"`
	DefaultAction string   `mapstructure:"default-action" validate:"oneof=allow deny"`
}

// AuthConfig holds the authentication settings.
type AuthConfig struct {
	UserDatabase string `mapstructure:"user-database" validate:"required_if=Enabled true"`
	Enabled      *bool  `mapstructure:"enabled" validate:"required"`
}

// SlackConfig holds the Slack settings.
type SlackConfig struct {
	Token           string `mapstructure:"token" validate:"required"`
	MessageTemplate string `mapstructure:"message-template" validate:"required"`
}

// Config holds the application's settings.
type Config struct {
	LogLevel string       `mapstructure:"log-level"`
	Slack    *SlackConfig `mapstructure:"slack" validate:"required"`
	SMTP     *SMTPConfig  `mapstructure:"smtp" validate:"required"`
}

// Helper to read a string flag from the console
func regFlagString(flag string, value string, usage string) {
	if pflag.Lookup(flag) == nil {
		pflag.String(flag, value, usage)
	}
}

// Helper to read a boolean flag from the console
func regFlagBoolP(flag, shorthand string, value bool, usage string) {
	if pflag.Lookup(flag) == nil {
		pflag.BoolP(flag, shorthand, value, usage)
	}
}

// Function to validate the config
func validateConfig(cfg interface{}) error {
	validate := validator.New()
	return validate.Struct(cfg)
}

// LoadConfig reads the configuration from the specified YAML file.
func LoadConfig() (*Config, error) {

	// Set defaults
	viper.SetDefault("config-file", "./config.yaml")
	viper.SetDefault("log-level", "INFO")
	viper.SetDefault("smtp.listen-addr", "localhost:25")

	// Register command flags
	regFlagString("config-file", viper.GetString("config-file"), "The path to the configuration file (YAML)")
	regFlagString("log-level", viper.GetString("log-level"), "The log level to use")
	regFlagString("smtp.listen-addr", viper.GetString("smtp.listen-addr"), "Listen address for the SMTP server (e.g., ':25').")
	regFlagBoolP("smtp.auth.enabled", "a", viper.GetBool("smtp.auth.enabled"), "Enable SMTP authentication")
	regFlagString("smtp.auth.user-database", viper.GetString("smtp.auth.user-database"), "Path to the user database file")
	regFlagString("slack.token-file", viper.GetString("slack.token-file"), "The path to a file containing Slack's token")
	regFlagBoolP("help", "h", false, "Prints this help message")
	regFlagBoolP("version", "V", false, "Prints the version")

	pflag.Parse()

	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		return nil, err
	}

	// Print usage if --help or -h
	if viper.GetBool("help") {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
		os.Exit(0)
	}

	// Print version if --version or -V
	if viper.GetBool("version") {
		fmt.Fprintf(os.Stderr, "Version: %s\n", version.Version)
		fmt.Fprintf(os.Stderr, "(Build date: %s, Git commit: %s)\n", version.BuildDate, version.GitCommit)
		os.Exit(0)
	}

	// Bind env vars to config directives
	viper.BindEnv("log-level", "LOG_LEVEL")
	viper.BindEnv("slack.token", "SLACK_TOKEN")

	// Load the config from file if it exists.
	viper.SetConfigFile(viper.GetString("config-file"))
	viper.SetConfigType("yaml")
	logger.Infof("Attempting to load config from '%s'", viper.GetString("config-file"))

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Config file was found but another error was produced
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		logger.Warnf("Config file not found at '%s', using defaults.", viper.GetString("config-file"))
	}

	// If access token file defined, attempt to load it
	pathToSlackTokenFile := viper.GetString("slack.token-file")
	if pathToSlackTokenFile != "" {
		logger.Debugf("Loading Slack Token from file: %s", pathToSlackTokenFile)
		// Read the entire content of the file
		contentBytes, err := os.ReadFile(pathToSlackTokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file '%s': %w", pathToSlackTokenFile, err)
		}
		// Convert byte slice to string
		contentString := string(contentBytes)
		// Set access-token configuration
		// accessTokenFromFile = strings.TrimSpace(contentString)
		viper.Set("slack.token", strings.TrimSpace(contentString))
	}

	cfg := &Config{}

	// Unmarshal the config
	if err := viper.Unmarshal(&cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	logger.Infof("Loaded config: %#v", cfg)

	// Validate the config
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation error: %w", err)
	}

	return cfg, nil
}
