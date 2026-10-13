package telegram

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SendTelegramMessage sends a message to Telegram using the HTTP API
func SendTelegramMessage(botToken, chatID, message string) error {
	return sendMessage(botToken, chatID, message, "Markdown")
}

// SendHTMLMessage supports Telegram's native spoiler tags.
func SendHTMLMessage(botToken, chatID, message string) error {
	return sendMessage(botToken, chatID, message, "HTML")
}

func sendMessage(botToken, chatID, message, parseMode string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	payload := url.Values{"chat_id": {chatID}, "text": {message}, "parse_mode": {parseMode}}.Encode()
	resp, err := httpClient.Post(apiURL, "application/x-www-form-urlencoded", strings.NewReader(payload))
	if err != nil {
		return redactToken(err, botToken)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram HTTP error: %d", resp.StatusCode)
	}
	return nil
}

// Sender is an interface used to send Telegram messages. It allows injecting
// a mock implementation during tests.
type Sender interface {
	Send(botToken, chatID, message string) error
}

type httpSender struct{}

func (h httpSender) Send(botToken, chatID, message string) error {
	return SendTelegramMessage(botToken, chatID, message)
}

// DefaultSender is the default implementation of Sender which uses HTTP.
var DefaultSender Sender = httpSender{}

// httpClient is used for making HTTP requests; it can be swapped in tests.
// Unlike http.DefaultClient it has a timeout, so a stalled connection cannot
// block a monitor forever.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// redactToken removes the bot token from err. net/http errors embed the full
// request URL, which for the Telegram API contains the token.
func redactToken(err error, token string) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	if token != "" {
		if msg := err.Error(); strings.Contains(msg, token) {
			return errors.New(strings.ReplaceAll(msg, token, "***"))
		}
	}
	return err
}

// urlEncode encodes the message for use in a URL
func urlEncode(s string) string {
	r := strings.NewReplacer(
		"%", "%25",
		"\n", "%0A",
		" ", "%20",
		"&", "%26",
	)
	return r.Replace(s)
}
