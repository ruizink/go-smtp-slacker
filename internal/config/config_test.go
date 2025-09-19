package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setenv(t *testing.T, key, value string) {
	t.Helper()
	originalValue, isSet := os.LookupEnv(key)
	err := os.Setenv(key, value)
	require.NoError(t, err)
	t.Cleanup(func() {
		if isSet {
			os.Setenv(key, originalValue)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestLoadConfig(t *testing.T) {
	// Base valid config content. Tests can override parts of this.
	baseValidConfig := `
log-level: "info"
slack:
  token: "xoxb-slack-token"
  message-template: "Email from {{.From}} to {{.To}}"
smtp:
  listen-addr: "0.0.0.0:2525"
  auth:
    enabled: true
    user-database: "/etc/users.db"
  policies:
    from:
      default-action: "allow"
    to:
      default-action: "deny"
      allow:
        - "allowed@example.com"
`

	testCases := []struct {
		name          string
		args          []string
		env           map[string]string
		configContent string
		noConfigFile  bool
		tokenContent  string
		expectError   bool
		errorContains string
		check         func(*testing.T, *Config)
	}{
		{
			name:          "valid config with auth enabled",
			configContent: baseValidConfig,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "info", cfg.LogLevel)
				assert.Equal(t, "xoxb-slack-token", cfg.Slack.Token)
				assert.Equal(t, "Email from {{.From}} to {{.To}}", cfg.Slack.MessageTemplate)
				assert.Equal(t, "0.0.0.0:2525", cfg.SMTP.ListenAddr)
				assert.True(t, *cfg.SMTP.Auth.Enabled)
				assert.Equal(t, "/etc/users.db", cfg.SMTP.Auth.UserDatabase)
				assert.Equal(t, "deny", cfg.SMTP.Policies.To.DefaultAction)
				assert.Contains(t, cfg.SMTP.Policies.To.Allow, "allowed@example.com")
			},
		},
		{
			name: "valid config with auth disabled",
			configContent: `
slack:
  token: "xoxb-slack-token"
  message-template: "template"
smtp:
  listen-addr: "0.0.0.0:2525"
  auth:
    enabled: false
  policies:
    from: { default-action: "allow" }
    to: { default-action: "deny" }
`,
			check: func(t *testing.T, cfg *Config) {
				assert.False(t, *cfg.SMTP.Auth.Enabled)
				assert.Empty(t, cfg.SMTP.Auth.UserDatabase)
			},
		},
		{
			name:          "no config file throws an error",
			noConfigFile:  true,
			expectError:   true,
			errorContains: "failed to read config file",
		},
		{
			name:          "invalid yaml",
			configContent: "slack: { token: 'abc'",
			expectError:   true,
			errorContains: "failed to read config file",
		},
		{
			name: "auth enabled but no user_database",
			configContent: `
slack:
  token: "xoxb-slack-token"
  message-template: "template"
smtp:
  listen-addr: "0.0.0.0:2525"
  auth:
    enabled: true
  policies:
    from: { default-action: "allow" }
    to: { default-action: "deny" }
`,
			expectError:   true,
			errorContains: "config validation error",
		},
		{
			name: "missing slack token",
			configContent: `
slack:
  message-template: "template"
smtp:
  listen-addr: "0.0.0.0:2525"
  auth:
    enabled: false
  policies:
    from: { default-action: "allow" }
    to: { default-action: "deny" }
`,
			expectError:   true,
			errorContains: "config validation error",
		},
		{
			name: "invalid policy default-action",
			configContent: `
slack:
  token: "xoxb-slack-token"
  message-template: "template"
smtp:
  listen-addr: "0.0.0.0:2525"
  auth:
    enabled: false
  policies:
    from: { default-action: "something" }
    to: { default-action: "deny" }
`,
			expectError:   true,
			errorContains: "config validation error",
		},
		{
			name:          "flag overrides config file",
			args:          []string{"--smtp.listen-addr", "1.2.3.4:5678", "--log-level", "warn"},
			configContent: baseValidConfig,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "1.2.3.4:5678", cfg.SMTP.ListenAddr)
				assert.Equal(t, "warn", cfg.LogLevel)
			},
		},
		{
			name: "env var overrides default",
			env:  map[string]string{"LOG_LEVEL": "debug"},
			configContent: `
slack:
  token: t
  message-template: m
smtp:
  policies:
    from: { default-action: "allow" }
    to: { default-action: "deny" }
`, // minimal valid
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "debug", cfg.LogLevel)
				assert.Equal(t, "localhost:25", cfg.SMTP.ListenAddr) // from default
			},
		},
		{
			name: "flag overrides env var and file",
			args: []string{"--log-level", "error"},
			env:  map[string]string{"LOG_LEVEL": "warn"},
			configContent: `
log-level: "info"
slack:
  token: t
  message-template: m
smtp:
  policies:
    from: { default-action: "allow" }
    to: { default-action: "deny" }
`,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "error", cfg.LogLevel)
			},
		},
		{
			name: "slack token from file",
			args: []string{"--slack.token-file", "token.txt"},
			configContent: `
slack:
  message-template: "template"
smtp:
  listen-addr: "0.0.0.0:2525"
  auth: { enabled: false }
  policies:
    from: { default-action: "allow" }
    to: { default-action: "allow" }
`,
			tokenContent: "token-from-file-123",
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "token-from-file-123", cfg.Slack.Token)
			},
		},
		{
			name: "slack token from env var",
			env:  map[string]string{"SLACK_TOKEN": "token-from-env-456"},
			configContent: `
slack:
  message-template: m
smtp:
  policies:
    from: { default-action: "allow" }
    to: { default-action: "deny" }
`,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "token-from-env-456", cfg.Slack.Token)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset global state for pflag and viper
			pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
			viper.Reset()

			// Set up environment variables for this test case
			for k, v := range tc.env {
				setenv(t, k, v)
			}

			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.yaml")
			tokenPath := filepath.Join(dir, "token.txt")

			if !tc.noConfigFile {
				err := os.WriteFile(configPath, []byte(tc.configContent), 0600)
				require.NoError(t, err, "failed to write temp config file")
			}

			if tc.tokenContent != "" {
				err := os.WriteFile(tokenPath, []byte(tc.tokenContent), 0600)
				require.NoError(t, err)
			}

			// Prepare command line arguments, adding the dynamic config path
			// and making file paths in arguments absolute for robustness.
			finalArgs := []string{"--config-file", configPath}
			for _, arg := range tc.args {
				if arg == "token.txt" {
					finalArgs = append(finalArgs, tokenPath)
				} else {
					finalArgs = append(finalArgs, arg)
				}
			}
			originalArgs := os.Args
			os.Args = append([]string{originalArgs[0]}, finalArgs...)
			t.Cleanup(func() { os.Args = originalArgs })

			// Run the function under test
			cfg, err := LoadConfig()

			if tc.expectError {
				require.Error(t, err, "expected an error but got none")
				assert.Contains(t, err.Error(), tc.errorContains, fmt.Sprintf("expected error to contain '%s'", tc.errorContains))
			} else {
				require.NoError(t, err, "did not expect an error")
				require.NotNil(t, cfg, "expected config to be non-nil")
				if tc.check != nil {
					tc.check(t, cfg)
				}
			}
		})
	}
}
