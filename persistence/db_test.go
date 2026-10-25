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

// Each test gets a disposable database; no objects in TEST_DATABASE_URL are changed.
func testDatabase(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL with CREATEDB permission to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	name := fmt.Sprintf("rzd_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "postgres" && u.Scheme != "postgresql" {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	u.Path = "/" + name
	return u.String()
}

func TestPostgresPersistence(t *testing.T) {
	scoped := testDatabase(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var schemaExists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname='rzd_monitor')`).Scan(&schemaExists); err != nil {
		t.Fatal(err)
	}
	if schemaExists {
		t.Fatal("test must start without the application schema")
	}

	if store, err := Open(ctx, scoped); err == nil {
		store.Close()
		t.Fatal("unmigrated database must not start")
	}
	if err := Migrate(ctx, scoped); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, scoped); err != nil {
		t.Fatalf("repeated migrations must be harmless: %v", err)
	}
	var tableCount int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='rzd_monitor' AND table_name IN ('monitors','schema_migrations')`).Scan(&tableCount); err != nil || tableCount != 2 {
		t.Fatalf("migration did not create schema and tables: count=%d err=%v", tableCount, err)
	}
	store, err := Open(ctx, scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if store != nil {
			store.Close()
		}
	}()
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
	// Reopen: rows and IDs must survive; application startup performs no DDL.
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

func TestDirtyMigrationPreventsStartup(t *testing.T) {
	dsn := testDatabase(t)
	ctx := context.Background()
	if err := Migrate(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `UPDATE rzd_monitor.schema_migrations SET dirty=TRUE`); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(ctx, dsn); err == nil {
		store.Close()
		t.Fatal("dirty database must not start")
	}
}
