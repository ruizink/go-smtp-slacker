package email

import (
	"bufio"
	"bytes"
	"fmt"
	"go-smtp-slacker/internal/config"
	"go-smtp-slacker/internal/logger"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DusanKasan/parsemail"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"golang.org/x/crypto/bcrypt"
)

const (
	PolicyAllow = "allow"
	PolicyDeny  = "deny"
)

// backend implements SMTP server methods
type backend struct {
	emailChan chan *email
	cfg       *config.SMTPConfig
	userDb    map[string]user
}

// session implements SMTP session methods
type session struct {
	authenticated bool
	cfg           *config.SMTPConfig
	emailChan     chan *email
	requireAuth   bool
	userDb        map[string]user
	remoteAddr    string
}

// email represents a parsed email.
type email struct {
	Body    EmailBody
	From    string
	Subject string
	To      []string
}

// EmailBody represents the types of email bodies
type EmailBody struct {
	HTML string
	Text string
}

// user represents an authenticated user with a bcrypt hashed password.
type user struct {
	username     string
	passwordHash string
}

// loadUserDatabase reads an user database file and returns a map of users.
// It expects bcrypt hashes (e.g., $2y$10$...).
func loadUserDatabase(filePath string) (map[string]user, error) {
	users := make(map[string]user)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open user database file '%s': %w", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			logger.Warnf("Skipping malformed line %d in user database file '%s': '%s'", lineNum, filePath, line)
			continue
		}

		username := parts[0]
		passwordHash := parts[1]

		users[username] = user{username: username, passwordHash: passwordHash}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading user database file %s: %w", filePath, err)
	}
	return users, nil
}

// Check if address is allowed/denied (deny list takes precedence)
func isAddressAllowed(address string, allowList, denyList []string, defaultPolicy string) bool {
	logger.Debugf("Checking address '%s' against allow list %v and deny list %v with default policy '%s'", address, allowList, denyList, defaultPolicy)

	for _, pattern := range denyList {
		if matched, err := filepath.Match(pattern, address); err != nil {
			logger.Errorf("Invalid glob pattern '%s' in deny list: %v", pattern, err)
		} else if matched {
			logger.Debugf("Address '%s' matched deny pattern '%s', rejecting", address, pattern)
			return false
		}
	}

	for _, pattern := range allowList {
		if matched, err := filepath.Match(pattern, address); err != nil {
			logger.Errorf("Invalid glob pattern '%s' in allow list: %v", pattern, err)
		} else if matched {
			logger.Debugf("Address '%s' matched allow pattern '%s', accepting", address, pattern)
			return true
		}
	}

	switch defaultPolicy {
	case PolicyAllow:
		logger.Debugf("Default policy is 'allow', accepting address '%s'", address)
		return true
	case PolicyDeny:
		logger.Debugf("Default policy is 'deny', rejecting address '%s'", address)
		return false
	default:
		logger.Debugf("Unrecognized default policy '%s', rejecting address '%s'", defaultPolicy, address)
		return false
	}
}

// NewSession is called after client greeting (EHLO, HELO).
func (bkd *backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &session{
		authenticated: false,
		cfg:           bkd.cfg,
		emailChan:     bkd.emailChan,
		requireAuth:   *bkd.cfg.Auth.Enabled,
		userDb:        bkd.userDb,
		remoteAddr:    c.Conn().RemoteAddr().String(),
	}, nil
}

// AuthMechanisms returns available auth mechanisms; only PLAIN is supported.
func (s *session) AuthMechanisms() []string {
	if s.requireAuth {
		return []string{sasl.Plain}
	}
	return nil
}

// Auth is the handler for supported authenticators.
func (s *session) Auth(mech string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(identity, username, password string) error {
		logger.Debugf("Authenticating user '%s' from %s", username, s.remoteAddr)
		user, ok := s.userDb[username]
		if !ok {
			logger.Warnf("Authentication failed for user '%s' (user not found) from %s", username, s.remoteAddr)
			return smtp.ErrAuthFailed
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.passwordHash), []byte(password)); err != nil {
			logger.Warnf("Authentication failed for user '%s' (password mismatch) from %s", username, s.remoteAddr)
			return smtp.ErrAuthFailed
		}
		logger.Debugf("User '%s' authenticated successfully from %s", username, s.remoteAddr)

		// Mark authentication as successful
		s.authenticated = true
		return nil
	}), nil
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {

	// Check if user is authenticated
	if s.requireAuth && !s.authenticated {
		logger.Warnf("There was an attempt to send an email without authentication from %s, rejecting", s.remoteAddr)
		return smtp.ErrAuthRequired
	}

	// Check against allowed/denied senders
	logger.Debugf("Checking if sender '%s' is allowed or denied", from)
	if !isAddressAllowed(from, s.cfg.Policies.From.Allow, s.cfg.Policies.From.Deny, s.cfg.Policies.From.DefaultAction) {
		logger.Warnf("Sender '%s' rejected by policy", from)
		return &smtp.SMTPError{
			Code:    550,
			Message: "Sender not allowed",
		}
	}

	return nil
}

func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {

	// Check if user is authenticated
	if s.requireAuth && !s.authenticated {
		logger.Warnf("There was an attempt to send an email without authentication from %s, rejecting", s.remoteAddr)
		return smtp.ErrAuthRequired
	}

	// Check against allowed/denied recipients
	logger.Debugf("Checking if recipient '%s' is allowed or denied", to)
	if !isAddressAllowed(to, s.cfg.Policies.To.Allow, s.cfg.Policies.To.Deny, s.cfg.Policies.To.DefaultAction) {
		logger.Warnf("Recipient '%s' rejected by policy", to)
		return &smtp.SMTPError{
			Code:    550,
			Message: "Recipient not allowed",
		}
	}

	return nil
}

// Data reads and parses the email, then sends it to the Emails channel.
func (s *session) Data(r io.Reader) error {

	// Check if user is authenticated
	if s.requireAuth && !s.authenticated {
		logger.Warnf("There was an attempt to send an email without authentication from %s, rejecting", s.remoteAddr)
		return smtp.ErrAuthRequired
	}

	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	// log RAW email
	logger.Tracef("Raw email:\n%s", string(b))

	emailParsed, err := parsemail.Parse(bytes.NewReader(b))
	if err != nil {
		logger.Errorf("Error parsing email: %v", err)
		return nil // skip if parse errors
	}

	var from string
	// skip if no from
	if len(emailParsed.From) == 0 {
		return nil
	} else {
		from = emailParsed.From[0].Address
	}

	var to []string
	for _, recipient := range emailParsed.To {
		to = append(to, recipient.Address)
	}

	// Skip if no recipients
	if len(to) == 0 {
		logger.Warnf("Email from '%s' has no recipient; skipping", from)
		return nil
	}

	email := &email{
		From:    from,
		To:      to,
		Subject: emailParsed.Subject,
		Body: EmailBody{
			HTML: emailParsed.HTMLBody,
			Text: emailParsed.TextBody,
		},
	}

	// Send the parsed email to the channel
	if s.emailChan != nil {
		s.emailChan <- email
	}

	return nil
}

func (s *session) Reset() {}

func (s *session) Logout() error {
	return nil
}

// NewServer creates a new SMTP server that pushes parsed emails to a channel.
func NewServer(cfg config.SMTPConfig) (*smtp.Server, chan *email) {
	emailChan := make(chan *email, 100) // buffered channel

	var users map[string]user
	if *cfg.Auth.Enabled {
		var err error
		users, err = loadUserDatabase(cfg.Auth.UserDatabase)
		if err != nil {
			logger.Fatalf("Failed to load user database file: %v", err)
		}
		logger.Infof("Loaded %d users from user database file '%s'", len(users), cfg.Auth.UserDatabase)
	}

	be := &backend{
		emailChan: emailChan,
		cfg:       &cfg,
		userDb:    users,
	}

	s := smtp.NewServer(be)
	s.ErrorLog = log.New(logger.NewLineWriter(logger.LevelError, "smtp/server:"), "", 0)
	s.Addr = cfg.ListenAddr
	// TODO: Make these configurable via config file
	s.Domain = "localhost"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024 // 1 MB
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true

	return s, emailChan
}
