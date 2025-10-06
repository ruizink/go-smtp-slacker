package main

import (
	"errors"
	"go-smtp-slacker/internal/config"
	"go-smtp-slacker/internal/email"
	"go-smtp-slacker/internal/logger"
	"go-smtp-slacker/internal/slacker"

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
	slackService, err := slacker.NewService(cfg.Slack.Token)
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
				err := slackService.SendMessage(recipient, e.From, e.To, e.Subject, e.Body, *cfg.SMTP.PreferHTMLBody)

				// if we failed to send the message (not using plain text), retry forcing the usage of plain text
				if err != nil {
					logger.Warnf("Failed to send message to '%s': %v", recipient, err)

					var sendErr *slacker.ErrSendMessage
					if errors.As(err, &sendErr) && *cfg.SMTP.PreferHTMLBody {
						logger.Warnf("Retrying with plain text")
						err := slackService.SendMessage(recipient, e.From, e.To, e.Subject, e.Body, true)
						if err != nil {
							logger.Errorf("Failed to send message to '%s': %v", recipient, err)
						}
					}
				}
			}
		}
	}()

	logger.Infof("Starting SMTP server at %s...", cfg.SMTP.ListenAddr)
	if err := server.ListenAndServe(); err != nil {
		logger.Fatalf("SMTP server error: %v", err)
	}
}
