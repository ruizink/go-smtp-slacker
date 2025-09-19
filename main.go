package main

import (
	"go-smtp-slacker/internal/config"
	"go-smtp-slacker/internal/email"
	"go-smtp-slacker/internal/logger"
	"go-smtp-slacker/internal/slack"

	"github.com/kr/pretty"
)

func main() {
	// Load configuration from YAML
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}

	// Set log level loaded from the config
	logger.SetLogLevel(logger.ParseLogLevel(cfg.LogLevel))
	logger.Debugf("Loaded configuration: %# v\n", pretty.Formatter(cfg))

	// Initialize Slack service
	slackService, err := slack.NewService(cfg.Slack.Token, cfg.Slack.MessageTemplate)
	if err != nil {
		logger.Fatalf("Failed to initialize Slack service: %v", err)
	}

	// Start the SMTP server
	server, emailChan := email.NewServer(*cfg.SMTP)

	// Forward incoming emails to Slack in a separate goroutine
	go func() {
		for e := range emailChan {
			logger.Debugf("Received email from %s to %v with subject: '%s'", e.From, e.To, e.Subject)

			// Skip if no recipients
			if len(e.To) == 0 {
				logger.Infof("Email from %s has no recipient; skipping", e.From)
				continue
			}

			// Send to each recipient
			for _, recipient := range e.To {
				slackService.SendMessage(recipient, e.From, e.To, e.Subject, e.Body)
			}
		}
	}()

	logger.Infof("Starting SMTP server at %s...", cfg.SMTP.ListenAddr)
	if err := server.ListenAndServe(); err != nil {
		logger.Fatalf("SMTP server error: %v", err)
	}
}
