// Package logging configures JSON logs and instruments outgoing HTTP requests.
package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func Setup() error {
	level := slog.LevelInfo
	if value := os.Getenv("LOG_LEVEL"); value != "" {
		if err := level.UnmarshalText([]byte(value)); err != nil {
			return err
		}
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	return nil
}

var sequence atomic.Uint64

type Transport struct {
	Service string
	Base    http.RoundTripper
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	id := sequence.Add(1)
	start := time.Now()
	attrs := []any{"service", t.Service, "request_id", id, "method", req.Method, "url", safeURL(req.URL)}
	if req.Body != nil && req.Body != http.NoBody {
		var body io.ReadCloser
		var err error
		if req.GetBody != nil {
			body, err = req.GetBody()
		} else {
			body = req.Body
		}
		if err != nil {
			slog.Error("http.request_body_failed", append(attrs, "error", redact(err.Error()))...)
			return nil, err
		}
		data, readErr := io.ReadAll(body)
		body.Close()
		if readErr != nil {
			slog.Error("http.request_body_failed", append(attrs, "error", redact(readErr.Error()))...)
			return nil, readErr
		}
		if req.GetBody == nil {
			req.Body = io.NopCloser(bytes.NewReader(data))
		}
		attrs = append(attrs, "request_body", bodyText(data))
	}

	slog.Info("http.request", attrs...)
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		slog.Error("http.request_failed", append(attrs, "duration_ms", time.Since(start).Milliseconds(), "error", redact(err.Error()))...)
		return nil, err
	}
	resp.Body = &loggedBody{ReadCloser: resp.Body, finish: func(data []byte, readErr error) {
		fields := []any{"service", t.Service, "request_id", id, "method", req.Method, "url", safeURL(req.URL), "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds(), "response_body", bodyText(data)}
		level := slog.LevelInfo
		if resp.StatusCode >= 400 || readErr != nil {
			level = slog.LevelError
		}
		if readErr != nil {
			fields = append(fields, "error", redact(readErr.Error()))
		}
		slog.Log(req.Context(), level, "http.response", fields...)
	}}
	return resp, nil
}

type loggedBody struct {
	io.ReadCloser
	data   []byte
	err    error
	once   sync.Once
	finish func([]byte, error)
}

func (b *loggedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.data = append(b.data, p[:n]...)
	if err != nil {
		if err != io.EOF {
			b.err = err
		}
		b.once.Do(func() { b.finish(b.data, b.err) })
	}
	return n, err
}
func (b *loggedBody) Close() error {
	// Capture unread bytes too when a JSON decoder stops before EOF.
	if b.err == nil {
		_, _ = io.Copy(io.Discard, b)
	}
	err := b.ReadCloser.Close()
	if b.err == nil {
		b.err = err
	}
	b.once.Do(func() { b.finish(b.data, b.err) })
	return err
}

func sensitive(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	return strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "password") || key == "authorization" || key == "cookie" || key == "set_cookie"
}
func clean(v any) {
	switch value := v.(type) {
	case map[string]any:
		for k, item := range value {
			if sensitive(k) {
				value[k] = "[REDACTED]"
			} else {
				clean(item)
			}
		}
	case []any:
		for _, item := range value {
			clean(item)
		}
	}
}
func redact(s string) string {
	for _, name := range []string{"MAX_BOT_TOKEN", "MAX_WEBHOOK_SECRET"} {
		if secret := os.Getenv(name); secret != "" {
			s = strings.ReplaceAll(s, secret, "[REDACTED]")
		}
	}
	return s
}

// bodyText logs the complete body, masking structured secrets for JSON payloads.
func bodyText(data []byte) string {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&value) == nil && decoder.Decode(new(any)) == io.EOF {
		clean(value)
		if encoded, err := json.Marshal(value); err == nil {
			data = encoded
		}
	}
	return redact(string(data))
}
func safeURL(u *url.URL) string {
	copy := *u
	copy.User = nil
	copy.Fragment = ""
	q := copy.Query()
	for key := range q {
		if sensitive(key) {
			q.Set(key, "[REDACTED]")
		}
	}
	copy.RawQuery = q.Encode()
	return redact(copy.String())
}
