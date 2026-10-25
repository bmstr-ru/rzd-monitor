// Package max implements the subset of MAX Bot API used by the monitor.
package max

import (
	"bmstr.ru/rzd-monitor/logging"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultBaseURL = "https://platform-api2.max.ru"

type User struct {
	ID    int64 `json:"user_id"`
	IsBot bool  `json:"is_bot"`
}
type Message struct {
	Sender    *User `json:"sender"`
	Recipient struct {
		ChatID int64 `json:"chat_id"`
	} `json:"recipient"`
	Body struct {
		Text string `json:"text"`
		MID  string `json:"mid"`
	} `json:"body"`
}
type Update struct {
	Type    string   `json:"update_type"`
	Message *Message `json:"message"`
	ChatID  int64    `json:"chat_id"`
	User    *User    `json:"user"`
}

// Input normalizes text messages and the first press of the Start button.
func (u Update) Input() (chatID, userID int64, text string, ok bool) {
	switch u.Type {
	case "bot_started":
		if u.User != nil {
			return u.ChatID, u.User.ID, "/start", true
		}
	case "message_created":
		if u.Message != nil && u.Message.Sender != nil && !u.Message.Sender.IsBot && strings.TrimSpace(u.Message.Body.Text) != "" {
			return u.Message.Recipient.ChatID, u.Message.Sender.ID, strings.TrimSpace(u.Message.Body.Text), true
		}
	}
	return
}

type Client struct {
	token    string
	baseURL  string
	http     *http.Client
	sendMu   sync.Mutex
	nextSend map[int64]time.Time
}

func New(token, baseURL string) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("MAX_BOT_TOKEN is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{token: token, baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 100 * time.Second, Transport: &logging.Transport{Service: "max"}}, nextSend: make(map[int64]time.Time)}, nil
}
func (c *Client) request(ctx context.Context, method, path string, body, result interface{}) error {
	var data io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		data = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, data)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("MAX request failed: %w", err)
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The HTTP transport logs the complete response with secrets masked.
		return fmt.Errorf("MAX %s: HTTP %d", method, resp.StatusCode)
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
func (c *Client) Verify(ctx context.Context) error {
	return c.request(ctx, http.MethodGet, "/me", nil, &User{})
}
func (c *Client) Send(ctx context.Context, chatID int64, text string) error {
	// MAX allows at most two messages per second in a chat.
	c.sendMu.Lock()
	slot := time.Now()
	if c.nextSend[chatID].After(slot) {
		slot = c.nextSend[chatID]
	}
	c.nextSend[chatID] = slot.Add(550 * time.Millisecond)
	c.sendMu.Unlock()
	timer := time.NewTimer(time.Until(slot))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	return c.request(ctx, http.MethodPost, "/messages?chat_id="+strconv.FormatInt(chatID, 10), map[string]string{"text": text}, nil)
}
func (c *Client) Poll(ctx context.Context, handle func(Update)) error {
	var subscriptions struct {
		Subscriptions []json.RawMessage `json:"subscriptions"`
	}
	if err := c.request(ctx, http.MethodGet, "/subscriptions", nil, &subscriptions); err != nil {
		return err
	}
	if len(subscriptions.Subscriptions) > 0 {
		return fmt.Errorf("MAX has an active webhook subscription; use webhook mode or remove the subscription before polling")
	}
	var marker *int64
	for ctx.Err() == nil {
		q := url.Values{"timeout": {"60"}, "types": {"message_created,bot_started"}}
		if marker != nil {
			q.Set("marker", strconv.FormatInt(*marker, 10))
		}
		var page struct {
			Updates []Update `json:"updates"`
			Marker  *int64   `json:"marker"`
		}
		if err := c.request(ctx, http.MethodGet, "/updates?"+q.Encode(), nil, &page); err != nil {
			if ctx.Err() != nil {
				break
			}
			slog.Error("max.poll_failed", "error", err, "retry_in_ms", 3000)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
			}
			continue
		}
		for _, u := range page.Updates {
			handle(u)
		}
		if page.Marker != nil {
			marker = page.Marker
		}
	}
	return nil
}
func (c *Client) Subscribe(ctx context.Context, endpoint, secret string) error {
	var result struct {
		Success bool `json:"success"`
	}
	err := c.request(ctx, http.MethodPost, "/subscriptions", map[string]interface{}{"url": endpoint, "secret": secret, "update_types": []string{"message_created", "bot_started"}}, &result)
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("MAX rejected webhook subscription")
	}
	return nil
}

// Default is initialized in main before starting the scheduler.
var Default *Client

func Send(chatID int64, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := Default.Send(ctx, chatID, text); err != nil {
		slog.Error("max.send_failed", "chat_id", chatID, "error", err)
	}
}
func Error(text string) { slog.Error("operation.failed", "error", text) }
