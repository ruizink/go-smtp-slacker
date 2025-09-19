package email

import (
	"bytes"
	"errors"
	"fmt"
	"go-smtp-slacker/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
	"golang.org/x/crypto/bcrypt"
)

func createTempUserDB(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "users.db")
	err := os.WriteFile(filePath, []byte(content), 0600)
	if err != nil {
		t.Fatalf("failed to write temp user db file: %v", err)
	}
	return filePath
}

func TestLoadUserDatabase(t *testing.T) {
	password := "password123"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to generate hash: %v", err)
	}
	hashStr := string(hash)

	validContent := fmt.Sprintf(`
# This is a comment
user1:%s
user2:anotherhash

# another comment
malformedline
user3:andanotherhash
`, hashStr)

	testCases := []struct {
		name          string
		content       string
		filePath      string
		expectError   bool
		errorContains string
		expectedUsers int
		checkUser     func(*testing.T, map[string]user)
	}{
		{
			name:          "valid user database",
			content:       validContent,
			expectError:   false,
			expectedUsers: 3,
			checkUser: func(t *testing.T, users map[string]user) {
				user1, ok := users["user1"]
				if !ok {
					t.Fatal("user1 should exist")
				}
				if user1.username != "user1" {
					t.Errorf("expected username 'user1', got '%s'", user1.username)
				}
				err := bcrypt.CompareHashAndPassword([]byte(user1.passwordHash), []byte(password))
				if err != nil {
					t.Errorf("password for user1 should match, but got error: %v", err)
				}

				if _, ok := users["user2"]; !ok {
					t.Error("user2 should exist")
				}
				if _, ok := users["user3"]; !ok {
					t.Error("user3 should exist")
				}
			},
		},
		{
			name:          "file not found",
			filePath:      "nonexistent/users.db",
			expectError:   true,
			errorContains: "failed to open user database file",
		},
		{
			name:          "empty file",
			content:       "",
			expectError:   false,
			expectedUsers: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filePath := tc.filePath
			if filePath == "" {
				filePath = createTempUserDB(t, tc.content)
			}

			users, err := loadUserDatabase(filePath)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected an error but got none")
				}
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error to contain '%s', but it was: %v", tc.errorContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("did not expect an error but got: %v", err)
				}
				if len(users) != tc.expectedUsers {
					t.Errorf("expected %d users, but got %d", tc.expectedUsers, len(users))
				}
				if tc.checkUser != nil {
					tc.checkUser(t, users)
				}
			}
		})
	}
}

func TestIsAddressAllowed(t *testing.T) {
	testCases := []struct {
		name          string
		address       string
		allowList     []string
		denyList      []string
		defaultPolicy string
		expected      bool
	}{
		{
			name:          "default deny, not in lists",
			address:       "test@example.com",
			defaultPolicy: PolicyDeny,
			expected:      false,
		},
		{
			name:          "default deny, in allow list",
			address:       "test@example.com",
			allowList:     []string{"test@example.com"},
			defaultPolicy: PolicyDeny,
			expected:      true,
		},
		{
			name:          "default deny, in deny list",
			address:       "test@example.com",
			denyList:      []string{"test@example.com"},
			defaultPolicy: PolicyDeny,
			expected:      false,
		},
		{
			name:          "default deny, in both lists (deny takes precedence)",
			address:       "test@example.com",
			allowList:     []string{"test@example.com"},
			denyList:      []string{"test@example.com"},
			defaultPolicy: PolicyDeny,
			expected:      false,
		},
		{
			name:          "default allow, not in lists",
			address:       "test@example.com",
			defaultPolicy: PolicyAllow,
			expected:      true,
		},
		{
			name:          "default allow, in deny list",
			address:       "test@example.com",
			denyList:      []string{"test@example.com"},
			defaultPolicy: PolicyAllow,
			expected:      false,
		},
		{
			name:          "glob allow",
			address:       "user@domain.com",
			allowList:     []string{"*@domain.com"},
			defaultPolicy: PolicyDeny,
			expected:      true,
		},
		{
			name:          "glob deny",
			address:       "user@domain.com",
			denyList:      []string{"*@domain.com"},
			defaultPolicy: PolicyAllow,
			expected:      false,
		},
		{
			name:          "glob deny takes precedence over exact allow",
			address:       "user@domain.com",
			allowList:     []string{"user@domain.com"},
			denyList:      []string{"*@domain.com"},
			defaultPolicy: PolicyAllow,
			expected:      false,
		},
		{
			name:          "invalid default policy",
			address:       "test@example.com",
			defaultPolicy: "something-else",
			expected:      false,
		},
		{
			name:          "invalid glob pattern in deny list is ignored",
			address:       "test@example.com",
			denyList:      []string{"["}, // invalid glob
			defaultPolicy: PolicyAllow,
			expected:      true,
		},
		{
			name:          "invalid glob pattern in allow list is ignored",
			address:       "test@example.com",
			allowList:     []string{"["}, // invalid glob
			defaultPolicy: PolicyDeny,
			expected:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Note: This test doesn't check for logger output, but verifies the logic.
			allowed := isAddressAllowed(tc.address, tc.allowList, tc.denyList, tc.defaultPolicy)
			if allowed != tc.expected {
				t.Errorf("expected %v, but got %v", tc.expected, allowed)
			}
		})
	}
}

func newTestSession(t *testing.T, cfg *config.SMTPConfig, authenticated bool, emailChan chan *email) *session {
	t.Helper()

	var userDb map[string]user
	if cfg.Auth.Enabled != nil && *cfg.Auth.Enabled {
		password := "password123"
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			t.Fatalf("failed to generate hash: %v", err)
		}
		userDb = map[string]user{
			"testuser": {
				username:     "testuser",
				passwordHash: string(hash),
			},
		}
	}

	return &session{
		authenticated: authenticated,
		cfg:           cfg,
		emailChan:     emailChan,
		requireAuth:   cfg.Auth.Enabled != nil && *cfg.Auth.Enabled,
		userDb:        userDb,
		remoteAddr:    "127.0.0.1:12345",
	}
}

func TestSession_MailAndRcpt(t *testing.T) {
	authEnabled := true
	authDisabled := false

	baseCfg := config.SMTPConfig{
		Policies: struct {
			From config.Policy `mapstructure:"from" validate:"required"`
			To   config.Policy `mapstructure:"to" validate:"required"`
		}{
			From: config.Policy{DefaultAction: PolicyAllow, Deny: []string{"bad-sender@example.com"}},
			To:   config.Policy{DefaultAction: PolicyDeny, Allow: []string{"good-rcpt@example.com"}},
		},
	}

	cfgAuthEnabled := baseCfg
	cfgAuthEnabled.Auth.Enabled = &authEnabled

	cfgAuthDisabled := baseCfg
	cfgAuthDisabled.Auth.Enabled = &authDisabled

	testCases := []struct {
		name          string
		method        string // "Mail" or "Rcpt"
		address       string
		cfg           config.SMTPConfig
		authenticated bool
		expectErr     error
	}{
		// Auth checks
		{
			name:          "Mail - Auth required, not authenticated",
			method:        "Mail",
			address:       "any@a.com",
			cfg:           cfgAuthEnabled,
			authenticated: false,
			expectErr:     smtp.ErrAuthRequired,
		},
		{
			name:          "Rcpt - Auth required, not authenticated",
			method:        "Rcpt",
			address:       "any@a.com",
			cfg:           cfgAuthEnabled,
			authenticated: false,
			expectErr:     smtp.ErrAuthRequired,
		},
		// Policy checks for Mail
		{
			name:          "Mail - Sender denied",
			method:        "Mail",
			address:       "bad-sender@example.com",
			cfg:           cfgAuthDisabled,
			authenticated: false,
			expectErr:     &smtp.SMTPError{Code: 550, Message: "Sender not allowed"},
		},
		{
			name:          "Mail - Sender allowed",
			method:        "Mail",
			address:       "good-sender@example.com",
			cfg:           cfgAuthDisabled,
			authenticated: false,
			expectErr:     nil,
		},
		// Policy checks for Rcpt
		{
			name:          "Rcpt - Recipient denied by default",
			method:        "Rcpt",
			address:       "bad-rcpt@example.com",
			cfg:           cfgAuthDisabled,
			authenticated: false,
			expectErr:     &smtp.SMTPError{Code: 550, Message: "Recipient not allowed"},
		},
		{
			name:          "Rcpt - Recipient allowed",
			method:        "Rcpt",
			address:       "good-rcpt@example.com",
			cfg:           cfgAuthDisabled,
			authenticated: false,
			expectErr:     nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestSession(t, &tc.cfg, tc.authenticated, nil)
			var err error
			if tc.method == "Mail" {
				err = s.Mail(tc.address, nil)
			} else {
				err = s.Rcpt(tc.address, nil)
			}

			if !errors.Is(err, tc.expectErr) {
				// Handle comparison for SMTPError which doesn't work well with errors.Is
				if expected, ok := tc.expectErr.(*smtp.SMTPError); ok {
					if actual, ok := err.(*smtp.SMTPError); ok {
						if expected.Code == actual.Code && expected.Message == actual.Message {
							return // Errors match
						}
					}
				}
				t.Errorf("expected error '%v', but got '%v'", tc.expectErr, err)
			}
		})
	}
}

func TestSession_Data(t *testing.T) {
	authEnabled := true
	authDisabled := false

	validEmail := `From: from@example.com
To: to@example.com
Subject: Test Subject

This is the body.`

	testCases := []struct {
		name          string
		emailContent  string
		cfg           config.SMTPConfig
		authenticated bool
		expectErr     error
		expectOnChan  bool
		checkEmail    func(*testing.T, *email)
	}{
		{
			name:          "Auth required, not authenticated",
			emailContent:  validEmail,
			cfg:           config.SMTPConfig{Auth: config.AuthConfig{Enabled: &authEnabled}},
			authenticated: false,
			expectErr:     smtp.ErrAuthRequired,
			expectOnChan:  false,
		},
		{
			name:          "Valid email is parsed and sent to channel",
			emailContent:  validEmail,
			cfg:           config.SMTPConfig{Auth: config.AuthConfig{Enabled: &authDisabled}},
			authenticated: true,
			expectErr:     nil,
			expectOnChan:  true,
			checkEmail: func(t *testing.T, e *email) {
				if e.From != "from@example.com" {
					t.Errorf("expected From 'from@example.com', got '%s'", e.From)
				}
				if len(e.To) != 1 || e.To[0] != "to@example.com" {
					t.Errorf("expected To 'to@example.com', got '%v'", e.To)
				}
				if e.Subject != "Test Subject" {
					t.Errorf("expected Subject 'Test Subject', got '%s'", e.Subject)
				}
			},
		},
		{
			name:          "Email with no From header is skipped",
			emailContent:  "To: to@example.com\n\nbody",
			cfg:           config.SMTPConfig{Auth: config.AuthConfig{Enabled: &authDisabled}},
			authenticated: true,
			expectErr:     nil,
			expectOnChan:  false,
		},
		{
			name:          "Email with no To header is skipped",
			emailContent:  "From: from@example.com\n\nbody",
			cfg:           config.SMTPConfig{Auth: config.AuthConfig{Enabled: &authDisabled}},
			authenticated: true,
			expectErr:     nil,
			expectOnChan:  false,
		},
		{
			name:          "Unparseable email is skipped",
			emailContent:  "this is not an email",
			cfg:           config.SMTPConfig{Auth: config.AuthConfig{Enabled: &authDisabled}},
			authenticated: true,
			expectErr:     nil, // The function currently returns nil on parse error
			expectOnChan:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			emailChan := make(chan *email, 1)
			s := newTestSession(t, &tc.cfg, tc.authenticated, emailChan)

			reader := bytes.NewReader([]byte(tc.emailContent))
			err := s.Data(reader)

			if !errors.Is(err, tc.expectErr) {
				t.Errorf("expected error '%v', but got '%v'", tc.expectErr, err)
			}

			if tc.expectOnChan {
				select {
				case e := <-emailChan:
					if tc.checkEmail != nil {
						tc.checkEmail(t, e)
					}
				case <-time.After(100 * time.Millisecond):
					t.Error("expected an email on the channel, but got none")
				}
			} else {
				select {
				case <-emailChan:
					t.Error("did not expect an email on the channel, but got one")
				default:
					// Correct, nothing on channel
				}
			}
		})
	}
}
