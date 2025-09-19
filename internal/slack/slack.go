package slack

import (
	"fmt"
	"go-smtp-slacker/internal/logger"
	"strings"

	"regexp"

	html2markdown "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/slack-go/slack"
)

// htmlToSlack returns the body an html message in a Slack format.
func htmlToSlack(message string) string {

	converter := html2markdown.NewConverter("", true, &html2markdown.Options{})
	markdown, err := converter.ConvertString(message)
	if err != nil {
		return message // fallback to the original message if conversion fails
	}

	// Convert Markdown links [text](url) to Slack format <url|text> before returning
	re := regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
	return re.ReplaceAllString(markdown, `<$2|$1>`)
}

type Service struct {
	client          *slack.Client
	messageTemplate string
}

// NewService creates a new Slack client and sets the message template.
func NewService(token, messageTemplate string) (*Service, error) {
	client := slack.New(token)

	resp, err := client.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("slack: authentication failed: %w", err)
	}
	if resp.User == "" {
		return nil, fmt.Errorf("slack: authentication failed: user is empty")
	}

	logger.Debugf("Slack: Token verified. Connected as user '%s'", resp.User)

	return &Service{
		client:          client,
		messageTemplate: messageTemplate,
	}, nil
}

func (s *Service) Client() *slack.Client {
	return s.client
}

// SendMessage sends a Slack message
func (s *Service) SendMessage(userEmail, sender string, to []string, subject, body string) {
	user, err := s.client.GetUserByEmail(userEmail)
	if err != nil {
		logger.Warnf("Slack: Error finding user by email '%s': %v", userEmail, err)
		return
	}
	logger.Debugf("Slack: Found matching user for email '%s': '%s'", userEmail, user.Name)

	channel, _, _, err := s.client.OpenConversation(&slack.OpenConversationParameters{
		Users: []string{user.ID},
	})
	if err != nil {
		logger.Errorf("Slack: Error opening DM with user '%s': %v", user.ID, err)
		return
	}
	logger.Debugf("Slack: Opened DM channel '%s' with user '%s'", channel.ID, user.Name)

	body = htmlToSlack(body)                             // Convert message format
	body = "> " + strings.ReplaceAll(body, "\n", "\n> ") // Quote body by adding '> ' prefix

	// Replace placeholders in template
	msg := s.messageTemplate
	replacements := map[string]string{
		"{from}":    sender,
		"{to}":      strings.Join(to, ", "),
		"{subject}": subject,
		"{body}":    body,
	}

	for placeholder, val := range replacements {
		msg = strings.ReplaceAll(msg, placeholder, val)
	}

	_, _, err = s.client.PostMessage(channel.ID, slack.MsgOptionText(msg, false))
	if err != nil {
		logger.Errorf("Slack: Error sending message to user '%s': %v", user.ID, err)
	} else {
		logger.Infof("Slack: Successfully forwarded email from '%s' to Slack user '%s' ('%s')", sender, user.Name, userEmail)
	}
}
