package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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
