package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const sendMessageURL = "https://api.telegram.org/bot%s/sendMessage"

// Client sends messages via Telegram Bot API.
type Client struct {
	token  string
	client *http.Client
}

// NewClient returns a Client with the given bot token.
func NewClient(token string) *Client {
	return &Client{token: token, client: &http.Client{}}
}

// SendMessageRequest is the request body for sendMessage.
type SendMessageRequest struct {
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// SendMessage sends text to the given chat. Returns error on HTTP or API failure.
func (c *Client) SendMessage(chatID int64, text string) error {
	url := fmt.Sprintf(sendMessageURL, c.token)
	body := SendMessageRequest{ChatID: chatID, Text: text}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sendMessage: status %d", resp.StatusCode)
	}
	return nil
}
