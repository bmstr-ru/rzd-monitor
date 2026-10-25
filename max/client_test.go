package max

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/messages" || r.URL.Query().Get("chat_id") != "-123" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "test-token" || r.URL.Query().Has("access_token") {
			t.Error("token must only be sent in Authorization header")
		}
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text != "Нашли билеты" {
			t.Errorf("unexpected body: %+v, %v", body, err)
		}
		w.Write([]byte(`{"message":{}}`))
	}))
	defer server.Close()
	client, _ := New("test-token", server.URL)
	if err := client.Send(context.Background(), -123, "Нашли билеты"); err != nil {
		t.Fatal(err)
	}
}

func TestPollMarkerAndInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/subscriptions" {
			w.Write([]byte(`{"subscriptions":[]}`))
			return
		}
		if r.URL.Query().Get("types") != "message_created,bot_started" {
			t.Error("wrong update types")
		}
		calls++
		switch calls {
		case 1:
			if r.URL.Query().Has("marker") {
				t.Error("first marker must be absent")
			}
			w.Write([]byte(`{"updates":[{"update_type":"bot_started","chat_id":21,"user":{"user_id":42}}],"marker":9007199254740993}`))
		case 2:
			if r.URL.Query().Get("marker") != "9007199254740993" {
				t.Error("marker lost precision or was not advanced")
			}
			w.Write([]byte(`{"updates":[{"update_type":"message_created","message":{"sender":{"user_id":42},"recipient":{"chat_id":21},"body":{"mid":"msg1","text":"/monitors"}}}],"marker":9007199254740994}`))
		default:
			t.Error("unexpected extra poll")
			cancel()
		}
	}))
	defer server.Close()
	client, _ := New("token", server.URL)
	inputs := 0
	err := client.Poll(ctx, func(u Update) {
		chat, user, text, ok := u.Input()
		if !ok || chat != 21 || user != 42 {
			t.Errorf("unexpected input: %+v", u)
		}
		inputs++
		if inputs == 1 && text != "/start" {
			t.Error("bot_started must start dialog")
		}
		if inputs == 2 {
			if text != "/monitors" {
				t.Error("wrong text")
			}
			cancel()
		}
	})
	if err != nil || inputs != 2 {
		t.Fatalf("poll: %v, inputs: %d", err, inputs)
	}
}

func TestPollRejectsWebhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"subscriptions":[{"url":"https://example.com/webhook"}]}`))
	}))
	defer server.Close()
	client, _ := New("token", server.URL)
	if err := client.Poll(context.Background(), func(Update) {}); err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestWebhook(t *testing.T) {
	for _, tc := range []struct {
		name, method, secret, body string
		want                       int
	}{
		{"valid", "POST", "secret", `{"update_type":"bot_started","chat_id":1,"user":{"user_id":2}}`, 200},
		{"bad secret", "POST", "wrong", `{}`, 403},
		{"method", "GET", "secret", `{}`, 405},
		{"invalid json", "POST", "secret", `{`, 400},
		{"trailing json", "POST", "secret", `{} {}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queue := make(chan Update, 1)
			req := httptest.NewRequest(tc.method, "/webhook", strings.NewReader(tc.body))
			req.Header.Set("X-Max-Bot-Api-Secret", tc.secret)
			recorder := httptest.NewRecorder()
			Webhook("secret", queue).ServeHTTP(recorder, req)
			if recorder.Code != tc.want {
				t.Fatalf("status %d; want %d", recorder.Code, tc.want)
			}
			if tc.want == 200 {
				if u := <-queue; u.Type != "bot_started" {
					t.Error("wrong queued update")
				}
			} else if len(queue) != 0 {
				t.Error("invalid update was queued")
			}
		})
	}
	queue := make(chan Update)
	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(`{}`))
	req.Header.Set("X-Max-Bot-Api-Secret", "secret")
	recorder := httptest.NewRecorder()
	Webhook("secret", queue).ServeHTTP(recorder, req)
	if recorder.Code != 503 {
		t.Error("full queue must trigger delivery retry")
	}
}

func TestAPIErrors(t *testing.T) {
	for _, code := range []int{401, 429, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		client, _ := New("token", server.URL)
		if err := client.Verify(context.Background()); err == nil {
			t.Errorf("HTTP %d must be an error", code)
		}
		server.Close()
	}
	if _, err := New("", ""); err == nil {
		t.Error("empty token must be rejected")
	}
}
