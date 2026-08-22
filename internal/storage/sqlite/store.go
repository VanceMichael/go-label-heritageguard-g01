package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/migrations"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB     *sql.DB
	Now    func() time.Time
	Logger *slog.Logger
	mu     sync.RWMutex
}

func Open(ctx context.Context, path string, maxOpen int, logger *slog.Logger) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if path != ":memory:" {
		if err := ensureDir(path); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxOpen)
	db.SetConnMaxLifetime(0)
	store := &Store{DB: db, Now: time.Now, Logger: logger}
	if err := store.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." {
		return nil
	}
	return osMkdirAll(dir)
}

var osMkdirAll = func(path string) error { return mkdirAll(path) }

func (s *Store) configure(ctx context.Context) error {
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := s.DB.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure sqlite: %s: %w", pragma, err)
		}
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	migrationCtx := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		migrationCtx, cancel = context.WithDeadline(migrationCtx, deadline)
		defer cancel()
	}
	for _, name := range files {
		if err := s.applyMigration(migrationCtx, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, name string) error {
	contents, err := migrations.FS.ReadFile(name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("prepare migration table: %w", err)
	}
	var applied int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name).Scan(&applied); err != nil {
		return fmt.Errorf("check migration %s: %w", name, err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, name, s.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) Ping(ctx context.Context) error {
	if err := s.DB.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping: %w", err)
	}
	return nil
}

type TxRunner interface {
	WithTx(context.Context, func(context.Context, *sql.Tx) error) error
}

func (s *Store) WithTx(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func isNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func closeRows(rows *sql.Rows) error {
	if rows == nil {
		return nil
	}
	return rows.Close()
}

func drainRows(rows *sql.Rows) error {
	if rows == nil {
		return nil
	}
	for rows.Next() {
	}
	return errors.Join(rows.Err(), rows.Close())
}

func isCancelled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func closeQuietly(c io.Closer) { _ = c.Close() }
