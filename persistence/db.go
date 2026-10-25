package persistence

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"bmstr.ru/rzd-monitor/monitor"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const queryTimeout = 10 * time.Second

// Store reads and writes directly to PostgreSQL. No local cache or CSV is used.
type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	// Parsing errors can contain the DSN and password. Do not expose them.
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL")
	}
	config.ConnConfig.RuntimeParams["application_name"] = "rzd-monitor"
	config.ConnConfig.ConnectTimeout = 10 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("create PostgreSQL pool failed")
	}
	store := &Store{pool: pool}
	setupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := pool.Ping(setupCtx); err != nil {
		pool.Close()
		return nil, dbError("connect", err)
	}
	var version uint
	var dirty bool
	if err := pool.QueryRow(setupCtx, `SELECT version, dirty FROM rzd_monitor.schema_migrations`).Scan(&version, &dirty); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database schema is not ready; run rzd-monitor migrate first: %w", dbError("read migration version", err))
	}
	expected, err := latestMigrationVersion()
	if err != nil {
		pool.Close()
		return nil, err
	}
	if dirty || version < expected {
		pool.Close()
		return nil, fmt.Errorf("database schema is not ready (version=%d, required=%d, dirty=%t); run rzd-monitor migrate", version, expected, dirty)
	}

	slog.Info("database.ready", "storage", "postgres", "schema", DatabaseSchema, "version", version)
	return store, nil
}
func (s *Store) Close() { s.pool.Close(); slog.Info("database.closed") }

// Keep server messages and connection errors (which may include credentials or
// SQL parameters) out of user-visible errors and logs. SQLSTATE remains useful.
func dbError(operation string, err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("database %s: %w", operation, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("database %s: %w", operation, context.DeadlineExceeded)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Errorf("database %s failed (SQLSTATE %s)", operation, pgErr.Code)
	}
	return fmt.Errorf("database %s failed (%T)", operation, err)
}

const columns = `id, chat_id, from_station_id, from_station_name,
 to_station_id, to_station_name, departure_date, train_number, scheduled`

func (s *Store) find(ctx context.Context, operation, query string, args ...any) ([]*monitor.Monitor, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	start := time.Now()
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, dbError(operation, err)
	}
	defer rows.Close()
	monitors := make([]*monitor.Monitor, 0)
	for rows.Next() {
		mon := &monitor.Monitor{From: &monitor.Station{}, To: &monitor.Station{}, Train: &monitor.Train{}}
		var date time.Time
		if err := rows.Scan(&mon.Id, &mon.ChatId, &mon.From.Id, &mon.From.Name, &mon.To.Id, &mon.To.Name, &date, &mon.Train.Number, &mon.Scheduled); err != nil {
			return nil, dbError(operation, err)
		}
		mon.Date = monitor.Date{Year: date.Year(), Month: int(date.Month()), Day: date.Day()}
		monitors = append(monitors, mon)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError(operation, err)
	}
	slog.Info("database.query", "operation", operation, "rows", len(monitors), "duration_ms", time.Since(start).Milliseconds())
	return monitors, nil
}
func (s *Store) FindMonitors(ctx context.Context, chatID int64) ([]*monitor.Monitor, error) {
	return s.find(ctx, "monitors.find", `SELECT `+columns+` FROM rzd_monitor.monitors WHERE chat_id=$1 ORDER BY id`, chatID)
}
func (s *Store) GetScheduledMonitors(ctx context.Context) ([]*monitor.Monitor, error) {
	return s.find(ctx, "monitors.scheduled", `SELECT `+columns+` FROM rzd_monitor.monitors WHERE scheduled ORDER BY id`)
}
func (s *Store) DeleteMonitors(ctx context.Context, chatID int64) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	start := time.Now()
	result, err := s.pool.Exec(ctx, `DELETE FROM rzd_monitor.monitors WHERE chat_id=$1`, chatID)
	if err != nil {
		return dbError("monitors.delete", err)
	}
	slog.Info("database.query", "operation", "monitors.delete", "chat_id", chatID, "rows", result.RowsAffected(), "duration_ms", time.Since(start).Milliseconds())
	return nil
}
func (s *Store) Deactivate(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	result, err := s.pool.Exec(ctx, `UPDATE rzd_monitor.monitors SET scheduled=FALSE WHERE id=$1`, id)
	if err != nil {
		return dbError("monitor.deactivate", err)
	}
	slog.Info("database.query", "operation", "monitor.deactivate", "monitor_id", id, "rows", result.RowsAffected())
	return nil
}
func (s *Store) StoreMonitor(ctx context.Context, mon *monitor.Monitor) error {
	if mon == nil || mon.From == nil || mon.To == nil || mon.Train == nil {
		return errors.New("incomplete monitor")
	}
	date := time.Date(mon.Date.Year, time.Month(mon.Date.Month), mon.Date.Day, 0, 0, 0, 0, time.UTC)
	if date.Year() != mon.Date.Year || int(date.Month()) != mon.Date.Month || date.Day() != mon.Date.Day || mon.Date.Year < 1 {
		return errors.New("invalid monitor date")
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	start := time.Now()
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO rzd_monitor.monitors
 (chat_id, from_station_id, from_station_name, to_station_id, to_station_name,
 departure_date, train_number) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		mon.ChatId, mon.From.Id, mon.From.Name, mon.To.Id, mon.To.Name, date, mon.Train.Number).Scan(&id)
	if err != nil {
		return dbError("monitor.store", err)
	}
	mon.Id, mon.Scheduled = id, true
	slog.Info("database.query", "operation", "monitor.store", "monitor_id", id, "chat_id", mon.ChatId, "duration_ms", time.Since(start).Milliseconds())
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}
