package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestTransportLogsJSONAndPreservesResponse(t *testing.T) {
	t.Setenv("MAX_BOT_TOKEN", "private-token")
	t.Setenv("MAX_WEBHOOK_SECRET", "private-secret")
	var output bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(old)
	body := `{"result":"ok","nested":{"access_token":"response-token"},"note":"private-token"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429); io.WriteString(w, body) }))
	defer server.Close()
	client := &http.Client{Transport: &Transport{Service: "max"}}
	req, _ := http.NewRequest("POST", server.URL+"/test?access_token=private-token", strings.NewReader(`{"secret":"private-secret","text":"hello"}`))
	req.Header.Set("Authorization", "private-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(data) != body {
		t.Fatal("logging changed response")
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected request and response logs: %s", output.String())
	}
	var request, response map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &request); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &response); err != nil {
		t.Fatal(err)
	}
	if request["request_id"] != response["request_id"] || response["status"] != float64(429) || response["level"] != "ERROR" || response["duration_ms"] == nil {
		t.Fatalf("missing HTTP fields: %v", response)
	}
	for _, secret := range []string{"private-token", "private-secret", "response-token"} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("secret leaked: %s", secret)
		}
	}
	if !strings.Contains(response["response_body"].(string), "ok") {
		t.Error("response preview missing")
	}
}

func TestFullRequestAndResponseBodies(t *testing.T) {
	// Both the former preview limit and capture limit must be gone.
	payload := `{"text":"` + strings.Repeat("я", 600000) + `"}`
	for _, replayable := range []bool{true, false} {
		t.Run(strconv.FormatBool(replayable), func(t *testing.T) {
			var output bytes.Buffer
			old := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
			defer slog.SetDefault(old)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received, err := io.ReadAll(r.Body)
				if err != nil || string(received) != payload {
					t.Error("request body changed")
				}
				io.WriteString(w, payload)
			}))
			defer server.Close()
			client := &http.Client{Transport: &Transport{Service: "test"}}
			req, _ := http.NewRequest("POST", server.URL, strings.NewReader(payload))
			if !replayable {
				req.GetBody = nil
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			// Close after a partial read: the log must still contain the full response.
			prefix := make([]byte, 10)
			if _, err := io.ReadFull(resp.Body, prefix); err != nil {
				t.Fatal(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(&output)
			for _, field := range []string{"request_body", "response_body"} {
				var record map[string]any
				if err := decoder.Decode(&record); err != nil {
					t.Fatal(err)
				}
				if record[field] != payload {
					t.Fatalf("%s missing or truncated", field)
				}
			}
		})
	}
}

func TestBodyText(t *testing.T) {
	t.Setenv("MAX_BOT_TOKEN", "private-token")
	for _, body := range []string{"", "plain response", "<html>upstream error</html>", `{"broken":`} {
		if got := bodyText([]byte(body)); got != body {
			t.Fatalf("body omitted: %q", got)
		}
	}
	if got := bodyText([]byte("error private-token")); got != "error [REDACTED]" {
		t.Fatalf("secret leaked: %s", got)
	}
}
