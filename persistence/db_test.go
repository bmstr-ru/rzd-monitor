package persistence

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"bmstr.ru/rzd-monitor/monitor"
	"github.com/jackc/pgx/v5"
)

// Uses an isolated schema, never the production monitors table.
func TestPostgresPersistence(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schemaName := fmt.Sprintf("rzd_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schemaName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE")
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "postgres" && u.Scheme != "postgresql" {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	query := u.Query()
	query.Set("search_path", schemaName)
	u.RawQuery = query.Encode()
	scoped := u.String()
	store, err := Open(ctx, scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	first := &monitor.Monitor{ChatId: 9007199254740993, From: &monitor.Station{Id: "2004720", Name: "Кемь"}, To: &monitor.Station{Id: "2004000", Name: "Санкт-Петербург"}, Train: &monitor.Train{Number: "015А"}, Date: monitor.Date{Year: 2027, Month: 1, Day: 20}}
	if err := store.StoreMonitor(ctx, first); err != nil {
		t.Fatal(err)
	}
	if first.Id <= 0 || !first.Scheduled {
		t.Fatalf("insert did not set identity and scheduled: %+v", first)
	}
	rows, err := store.FindMonitors(ctx, first.ChatId)
	if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0], first) {
		t.Fatalf("round trip mismatch: %+v, %v", rows, err)
	}
	second := *first
	second.Id = 0
	second.ChatId = -9007199254740993
	if err := store.StoreMonitor(ctx, &second); err != nil {
		t.Fatal(err)
	}
	if first.Id == second.Id {
		t.Fatal("IDs must be unique")
	}
	if err := store.Deactivate(ctx, first.Id); err != nil {
		t.Fatal(err)
	}
	active, err := store.GetScheduledMonitors(ctx)
	if err != nil || len(active) != 1 || active[0].Id != second.Id {
		t.Fatalf("scheduled filter: %+v, %v", active, err)
	}
	// Reopen: schema creation must be idempotent and rows and IDs must survive.
	store.Close()
	store, err = Open(ctx, scoped)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = store.FindMonitors(ctx, first.ChatId)
	if err != nil || len(rows) != 1 || rows[0].Scheduled {
		t.Fatalf("restart lost state: %+v, %v", rows, err)
	}
	third := *first
	third.Id = 0
	if err := store.StoreMonitor(ctx, &third); err != nil {
		t.Fatal(err)
	}
	if third.Id <= second.Id {
		t.Fatal("identity reset after restart")
	}
	if err := store.DeleteMonitors(ctx, first.ChatId); err != nil {
		t.Fatal(err)
	}
	rows, err = store.FindMonitors(ctx, first.ChatId)
	if err != nil || len(rows) != 0 {
		t.Fatalf("delete failed: %+v, %v", rows, err)
	}
	rows, err = store.FindMonitors(ctx, second.ChatId)
	if err != nil || len(rows) != 1 {
		t.Fatal("delete affected another chat")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.FindMonitors(canceled, second.ChatId); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
	invalid := *first
	invalid.Date = monitor.Date{Year: 2027, Month: 2, Day: 31}
	if err := store.StoreMonitor(ctx, &invalid); err == nil {
		t.Fatal("invalid date accepted")
	}
}

func TestInvalidConfigurationDoesNotLeakCredentials(t *testing.T) {
	for _, dsn := range []string{"", "://secret-password", "postgres://user:secret-password@localhost/db?pool_max_conns=bad"} {
		_, err := Open(context.Background(), dsn)
		if err == nil || strings.Contains(err.Error(), "secret-password") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}
