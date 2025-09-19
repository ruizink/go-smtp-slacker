package slack

import (
	"testing"
)

func TestHtmlToSlack(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple paragraph",
			input:    "<p>Hello World</p>",
			expected: "Hello World",
		},
		{
			name:     "html link",
			input:    `Some text with a <a href="http://example.com">link</a>.`,
			expected: `Some text with a <http://example.com|link>.`,
		},
		{
			name:     "bold and italic",
			input:    "<b>bold</b> and <i>italic</i>",
			expected: "**bold** and _italic_",
		},
		{
			name:     "plain text",
			input:    "This is just plain text.",
			expected: "This is just plain text.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := htmlToSlack(tc.input)
			if actual != tc.expected {
				t.Errorf("expected '%s', got '%s'", tc.expected, actual)
			}
		})
	}
}
