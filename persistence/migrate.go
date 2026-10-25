package persistence

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

const DatabaseSchema = "rzd_monitor"

//go:embed migrations/*.up.sql
var migrations embed.FS

func connectionConfig(databaseURL string) (*pgx.ConnConfig, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL")
	}
	config := poolConfig.ConnConfig
	config.ConnectTimeout = 10 * time.Second
	config.RuntimeParams["application_name"] = "rzd-monitor-migrate"
	config.RuntimeParams["statement_timeout"] = "60000"
	config.RuntimeParams["lock_timeout"] = "30000"
	return config, nil
}

func latestMigrationVersion() (uint, error) {
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return 0, err
	}
	var latest uint
	for _, file := range files {
		version, err := strconv.ParseUint(strings.SplitN(file.Name(), "_", 2)[0], 10, 32)
		if err != nil {
			return 0, err
		}
		if uint(version) > latest {
			latest = uint(version)
		}
	}
	return latest, nil
}

// Migrate only runs explicitly via `rzd-monitor migrate`, never on bot startup.
// SQL files and migration history are shipped together with the application.
func Migrate(ctx context.Context, databaseURL string) error {
	config, err := connectionConfig(databaseURL)
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*config)
	defer db.Close()
	db.SetMaxOpenConns(2)
	setupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := db.PingContext(setupCtx); err != nil {
		return dbError("migration connect", err)
	}
	// The migration driver's version table needs its schema to already exist.
	// Serialize schema bootstrapping, before golang-migrate takes its own lock.
	tx, err := db.BeginTx(setupCtx, nil)
	if err != nil {
		return dbError("migration bootstrap", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(setupCtx, "SELECT pg_advisory_xact_lock(791682374125)"); err == nil {
		_, err = tx.ExecContext(setupCtx, `CREATE SCHEMA IF NOT EXISTS rzd_monitor`)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		return dbError("create schema", err)
	}
	driver, err := migratepgx.WithInstance(db, &migratepgx.Config{SchemaName: DatabaseSchema, StatementTimeout: 60 * time.Second})
	if err != nil {
		return dbError("initialize migrations", err)
	}
	source, err := iofs.New(migrations, "migrations")
	if err != nil {
		driver.Close()
		return err
	}
	runner, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		source.Close()
		driver.Close()
		return err
	}
	defer runner.Close()
	runner.LockTimeout = 30 * time.Second
	slog.Info("database.migration_started", "schema", DatabaseSchema)
	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		var dirty migrate.ErrDirty
		if errors.As(err, &dirty) {
			return fmt.Errorf("database migration version %d is dirty; inspect the failed migration before repair", dirty.Version)
		}
		var detail migratedb.Error
		var detailPtr *migratedb.Error
		if errors.As(err, &detail) {
			return dbError("migrate", detail.OrigErr)
		}
		if errors.As(err, &detailPtr) {
			return dbError("migrate", detailPtr.OrigErr)
		}
		return dbError("migrate", err)
	}
	version, dirty, err := runner.Version()
	if err != nil {
		return dbError("read migration version", err)
	}
	slog.Info("database.migration_complete", "schema", DatabaseSchema, "version", version, "dirty", dirty)
	return nil
}
