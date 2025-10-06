package slacker

import (
	"fmt"
	"go-smtp-slacker/internal/email"
	"go-smtp-slacker/internal/logger"
	"go-smtp-slacker/internal/utils"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/slack-go/slack"
	util "github.com/takara2314/slack-go-util"
	"golang.org/x/net/html"
)

type ErrUserNotFound struct {
	User string
	Err  error
}

func (e *ErrUserNotFound) Error() string {
	return fmt.Sprintf("error finding user by email '%s': %v", e.User, e.Err)
}

type ErrUserDM struct {
	User string
	Err  error
}

func (e *ErrUserDM) Error() string {
	return fmt.Sprintf("error opening DM with user '%s': %v", e.User, e.Err)
}

type ErrSendMessage struct {
	User string
	Err  error
}

func (e *ErrSendMessage) Error() string {
	return fmt.Sprintf("error sending message to user '%s': %v", e.User, e.Err)
}

// htmlToMarkdown returns an html message in markdown
func htmlToMarkdown(message string) (string, error) {

	c := converter.NewConverter(
		// converter.WithEscapeMode("disabled"),
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(
				commonmark.WithStrongDelimiter("**"), // bold
				commonmark.WithEmDelimiter("_"),      // italic
				commonmark.WithBulletListMarker("*"), // bullet list
				commonmark.WithListEndComment(false), // do not mark the end of a list
			),
			table.NewTablePlugin(
				// table.WithHeaderPromotion(true),
				// table.WithSkipEmptyRows(true),
				// table.WithSkipEmptyHeader(true),
				// table.WithNewlineBehavior("delete"),
				table.WithNewlineBehavior("preserve"),
			),
			strikethrough.NewStrikethroughPlugin(
				strikethrough.WithDelimiter("~~"), // strikethrough
			),
		),
	)

	// Override <br> — return two newlines (paragraph break).
	c.Register.RendererFor(
		"br",
		converter.TagTypeInline,
		func(ctx converter.Context, w converter.Writer, node *html.Node) converter.RenderStatus {
			w.WriteString("\n\n")
			return converter.RenderSuccess
		},
		converter.PriorityEarly,
	)

	// Add a renderer for <input type="checkbox">
	c.Register.RendererFor(
		"input",
		converter.TagTypeInline,
		func(ctx converter.Context, w converter.Writer, node *html.Node) converter.RenderStatus {
			isCheckbox := false
			isChecked := false
			for _, attr := range node.Attr {
				if attr.Key == "type" && attr.Val == "checkbox" {
					isCheckbox = true
				}
				if attr.Key == "checked" {
					isChecked = true
				}
			}

			if isCheckbox {
				w.WriteString(map[bool]string{true: "[x]", false: "[ ]"}[isChecked])
				return converter.RenderSuccess
			}
			return converter.RenderTryNext
		},
		converter.PriorityEarly,
	)

	logger.Tracef("Slack: Converting HTML message to markdown")
	return c.ConvertString(message)
}

// htmlToSlack returns an html message in a Slack format.
func htmlToSlack(message string) []slack.Block {

	// convert html to markdown
	markdown, err := htmlToMarkdown(message)
	if err != nil {
		// fallback to the original message within a block if conversion fails
		return []slack.Block{
			&slack.SectionBlock{
				Type: slack.MBTSection,
				Text: &slack.TextBlockObject{
					Type: slack.MarkdownType,
					Text: message,
				},
			},
		}
	}

	// convert markdown entities to Slack blocks
	logger.Tracef("Slack: Converting markdown message to Slack format")
	blocks, err := util.ConvertMarkdownTextToBlocks(markdown)

	if err != nil {
		// fallback to the original message within a block if conversion fails
		return []slack.Block{
			&slack.SectionBlock{
				Type: slack.MBTSection,
				Text: &slack.TextBlockObject{
					Type: slack.MarkdownType,
					Text: message,
				},
			},
		}
	}
	// Convert Markdown links [text](url) to Slack format <url|text> before returning
	// re := regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
	// markdown = re.ReplaceAllString(markdown, "<$2|$1>")

	return blocks
}

func textToSlack(message string) []slack.Block {
	// replace the markdown italic __ with slack's implementation _
	// message = strings.ReplaceAll(message, "__", "_")
	// // replace single * with _ for italic slack's implementation _
	// re := regexp.MustCompile(`[\*]{1}(.*?)[\*]{1}`)
	// message = re.ReplaceAllString(message, "_$1_")
	// // replace the markdown bold **  with slack's implementation *
	// message = strings.ReplaceAll(message, "**", "*")
	// // replace the markdown strikethrough ~~ with slack's implementation ~
	// message = strings.ReplaceAll(message, "~~", "~")
	logger.Tracef("Slack: Converting text message to Slack format")

	return []slack.Block{
		&slack.SectionBlock{
			Type: slack.MBTSection,
			Text: &slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: message,
			},
		},
	}
}

type Service struct {
	client *slack.Client
}

// NewService creates a new Slack client
func NewService(token utils.Secret) (*Service, error) {
	client := slack.New(token.GetValue())

	resp, err := client.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("slack: authentication failed: %w", err)
	}
	if resp.User == "" {
		return nil, fmt.Errorf("slack: authentication failed: user is empty")
	}

	logger.Debugf("Slack: Token verified. Connected as user '%s'", resp.User)

	return &Service{
		client: client,
	}, nil
}

func (s *Service) Client() *slack.Client {
	return s.client
}

// SendMessage sends a Slack message
func (s *Service) SendMessage(userEmail, sender string, to []string, subject string, body email.EmailBody, preferHTMLBody bool) error {

	// retrieve user by email
	user, err := s.client.GetUserByEmail(userEmail)
	if err != nil {
		logger.Warnf("Slack: Error finding user by email '%s': %v", userEmail, err)
		return &ErrUserNotFound{User: userEmail, Err: err}
	}
	logger.Debugf("Slack: Found matching user for email '%s': '%s'", userEmail, user.Name)

	// generate the message
	var bodyBlocks []slack.Block
	if preferHTMLBody {
		if strings.TrimSpace(body.HTML) == "" {
			return &ErrSendMessage{User: user.ID, Err: fmt.Errorf("empty HTML body")}
		}
		logger.Debugf("Slack: Converting HTML message to Slack format")
		bodyBlocks = htmlToSlack(body.HTML)
	} else {
		if strings.TrimSpace(body.Text) == "" {
			return &ErrSendMessage{User: user.ID, Err: fmt.Errorf("empty plain text body")}
		}
		logger.Debugf("Slack: Using plain text message")
		bodyBlocks = textToSlack(body.Text)
	}

	dividerBlock := &slack.DividerBlock{
		Type: slack.MBTDivider,
	}

	headerBlock := &slack.SectionBlock{
		Type: slack.MBTSection,
		Text: &slack.TextBlockObject{
			Type: slack.MarkdownType,
			Text: fmt.Sprintf("*New notification from:* %s\n*Subject:* %s", sender, strings.Join(to, ", ")),
		},
	}

	// open a DM with the user
	channel, _, _, err := s.client.OpenConversation(&slack.OpenConversationParameters{
		Users: []string{user.ID},
	})
	if err != nil {
		logger.Errorf("Slack: Error opening DM with user '%s': %v", user.ID, err)
		return &ErrUserDM{User: user.ID, Err: err}
	}
	logger.Debugf("Slack: Opened DM channel '%s' with user '%s'", channel.ID, user.Name)

	// compose the Slack message blocks
	msgBlocks := []slack.Block{}
	msgBlocks = append(msgBlocks, dividerBlock)
	msgBlocks = append(msgBlocks, headerBlock)
	msgBlocks = append(msgBlocks, bodyBlocks...)
	msgBlocks = append(msgBlocks, dividerBlock)

	logger.Debugf("Slack: Sending message to user '%s'", user.ID)
	_, _, err = s.client.PostMessage(channel.ID, slack.MsgOptionBlocks(msgBlocks...))
	if err != nil {
		logger.Errorf("Slack: Error sending message to user '%s': %v", user.ID, err)
		return &ErrSendMessage{User: user.ID, Err: err}
	} else {
		logger.Infof("Slack: Successfully sent message from '%s' to Slack user '%s' ('%s')", sender, user.Name, userEmail)
	}

	return nil
}
