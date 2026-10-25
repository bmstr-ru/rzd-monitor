package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bmstr.ru/rzd-monitor/max"
	"bmstr.ru/rzd-monitor/monitor"
)

func TestDialogsAreIsolatedAndInvalidDateDoesNotAdvance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer server.Close()
	client, _ := max.New("token", server.URL)
	oldClient := max.Default
	max.Default = client
	defer func() { max.Default = oldClient }()
	oldDialogs := dialogs
	dialogs = make(map[dialogKey]*dialog)
	defer func() { dialogs = oldDialogs }()
	start := func(chatID, userID int64) {
		handleUpdate(max.Update{Type: "bot_started", ChatID: chatID, User: &max.User{ID: userID}})
		key := dialogKey{chatID, userID}
		ctx := context.WithValue(context.Background(), "ChatId", chatID)
		ctx = context.WithValue(ctx, dialogContextKey{}, dialogs[key])
		startNewMonitorInput(ctx)
	}
	start(10, 1)
	start(10, 2)
	start(20, 1)
	first, second, third := dialogs[dialogKey{10, 1}], dialogs[dialogKey{10, 2}], dialogs[dialogKey{20, 1}]
	if first == second || first == third || first.mon == second.mon {
		t.Fatal("dialogs share state")
	}
	ctx := context.WithValue(context.Background(), "ChatId", int64(10))
	ctx = context.WithValue(ctx, dialogContextKey{}, first)
	first.machine.Event(ctx, "input-start", &monitor.Station{Name: "Москва"})
	first.machine.Event(ctx, "input-finish", &monitor.Station{Name: "Сочи"})
	first.machine.Event(ctx, "input-train", &monitor.Train{Number: "123"})
	processInputDate(ctx, "bad-date")
	if first.machine.Current() != "get-date" || first.mon.Date.Year != 0 {
		t.Error("invalid date advanced dialog")
	}
	if second.machine.Current() != "get-start" || third.machine.Current() != "get-start" {
		t.Error("another dialog changed")
	}
	commands(ctx, "cancel")
	if first.machine != nil || first.mon != nil {
		t.Error("cancel did not reset dialog")
	}
	if second.machine == nil {
		t.Error("cancel reset another dialog")
	}
}

type retryRepository struct {
	err   error
	calls int
}

func (r *retryRepository) FindMonitors(context.Context, int64) ([]*monitor.Monitor, error) {
	return nil, r.err
}
func (r *retryRepository) DeleteMonitors(context.Context, int64) error { return r.err }
func (r *retryRepository) StoreMonitor(_ context.Context, mon *monitor.Monitor) error {
	r.calls++
	if r.err != nil {
		return r.err
	}
	mon.Id = 123
	mon.Scheduled = true
	return nil
}

func TestDatabaseFailureKeepsDialogForRetry(t *testing.T) {
	messages := make(chan string, 20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		messages <- body.Text
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, _ := max.New("token", server.URL)
	oldClient := max.Default
	max.Default = client
	defer func() { max.Default = oldClient }()
	repository := &retryRepository{err: errors.New("database unavailable")}
	oldStore := monitorStore
	monitorStore = repository
	defer func() { monitorStore = oldStore }()
	d := &dialog{}
	ctx := context.WithValue(context.Background(), "ChatId", int64(1))
	ctx = context.WithValue(ctx, dialogContextKey{}, d)
	startNewMonitorInput(ctx)
	d.machine.Event(ctx, "input-start", &monitor.Station{Id: "1", Name: "Кемь"})
	d.machine.Event(ctx, "input-finish", &monitor.Station{Id: "2", Name: "Санкт-Петербург"})
	d.machine.Event(ctx, "input-train", &monitor.Train{Number: "015А"})
	processInputDate(ctx, "2027/01/20")
	if d.machine == nil || d.machine.Current() != "get-date" || d.mon == nil || repository.calls != 1 {
		t.Fatal("failed save must preserve dialog for retry")
	}
	for len(messages) > 0 {
		if strings.Contains(<-messages, "Спасибо! Ваше отслеживание") {
			t.Fatal("failed save reported as successful")
		}
	}
	repository.err = nil
	processInputDate(ctx, "2027/01/20")
	if d.machine != nil || d.mon != nil || repository.calls != 2 {
		t.Fatal("successful retry must finish dialog")
	}
	if text := <-messages; !strings.Contains(text, "Номер отслеживания: 123") {
		t.Fatalf("missing DB-assigned ID: %s", text)
	}
}
