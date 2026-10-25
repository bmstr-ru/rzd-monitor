package max

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
)

// Webhook queues validated events; a single consumer preserves dialog order.
func Webhook(secret string, updates chan<- Update) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Max-Bot-Api-Secret")), []byte(secret)) != 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(r.Body)
		var update Update
		if err := decoder.Decode(&update); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		select {
		case updates <- update:
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "Queue full; retry later", http.StatusServiceUnavailable)
		}
	})
}
