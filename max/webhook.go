package max

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Webhook queues validated events; a single consumer preserves dialog order.
func Webhook(secret string, updates chan<- Update) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		reason := "accepted"
		defer func() {
			level := slog.LevelInfo
			if status >= 400 {
				level = slog.LevelWarn
			}
			slog.Log(r.Context(), level, "webhook.request", "method", r.Method, "path", r.URL.Path, "status", status, "reason", reason, "duration_ms", time.Since(start).Milliseconds(), "queue_size", len(updates))
		}()
		if r.Method != http.MethodPost {
			status, reason = http.StatusMethodNotAllowed, "invalid_method"
			w.WriteHeader(status)
			return
		}
		if secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Max-Bot-Api-Secret")), []byte(secret)) != 1 {
			status, reason = http.StatusForbidden, "invalid_secret"
			w.WriteHeader(status)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(r.Body)
		var update Update
		if err := decoder.Decode(&update); err != nil {
			status, reason = http.StatusBadRequest, "invalid_json"
			http.Error(w, "Invalid JSON", status)
			return
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			status, reason = http.StatusBadRequest, "invalid_json"
			http.Error(w, "Invalid JSON", status)
			return
		}
		select {
		case updates <- update:
			w.WriteHeader(http.StatusOK)
		default:
			status, reason = http.StatusServiceUnavailable, "queue_full"
			http.Error(w, "Queue full; retry later", status)
		}
	})
}
