// Package store owns the SQLite connection pool, schema migrations, and the
// single-instance lock.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/mrweez/iwmmd/migrations"
)

// ErrLocked is returned when another IWMMD process owns the data directory.
var ErrLocked = errors.New("another iwmmd instance is already using this data directory")

var migrateMu sync.Mutex

// Options controls SQLite and data-directory setup.
type Options struct {
	DataDir      string
	DSN          string
	ReadPoolSize int
}

// Store holds separate write and read SQLite pools.
type Store struct {
	Writer *sql.DB
	Reader *sql.DB

	lock *flock.Flock
}

// Open acquires the instance lock, opens the database pools, and verifies
// connectivity. Callers must call Close.
func Open(opts Options) (*Store, error) {
	if opts.DataDir == "" {
		return nil, errors.New("data directory is required")
	}
	if err := prepareDataDir(opts.DataDir); err != nil {
		return nil, err
	}
	lock, err := acquireLock(opts.DataDir)
	if err != nil {
		return nil, err
	}

	dsn, err := dataSourceName(opts)
	if err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	writer, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = lock.Unlock()
		return nil, fmt.Errorf("open sqlite writer: %w", err)
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)

	poolSize := opts.ReadPoolSize
	if poolSize < 1 {
		poolSize = 4
	}
	reader, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = writer.Close()
		_ = lock.Unlock()
		return nil, fmt.Errorf("open sqlite reader: %w", err)
	}
	reader.SetMaxOpenConns(poolSize)
	reader.SetMaxIdleConns(poolSize)

	ctx := context.Background()
	if err := writer.PingContext(ctx); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		_ = lock.Unlock()
		return nil, fmt.Errorf("ping sqlite writer: %w", err)
	}
	if err := reader.PingContext(ctx); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		_ = lock.Unlock()
		return nil, fmt.Errorf("ping sqlite reader: %w", err)
	}

	return &Store{Writer: writer, Reader: reader, lock: lock}, nil
}

// DatabasePath returns the default SQLite file path for a data directory.
func DatabasePath(dataDir string) string {
	return filepath.Join(dataDir, "iwmmd.db")
}

// OpenRaw opens a SQLite connection without taking the instance lock. It is
// intended for read-only maintenance commands such as backup.
func OpenRaw(opts Options) (*sql.DB, error) {
	dsn, err := dataSourceName(opts)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}

// Migrate applies all embedded schema migrations.
func (s *Store) Migrate() error {
	migrateMu.Lock()
	defer migrateMu.Unlock()

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	goose.SetBaseFS(migrations.FS)
	goose.SetTableName("schema_migrations")
	goose.SetLogger(goose.NopLogger())
	if err := goose.Up(s.Writer, "sqlite"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Ready verifies both database connectivity and that migrations have run.
func (s *Store) Ready(ctx context.Context) error {
	if err := s.Reader.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	var one int
	err := s.Reader.QueryRowContext(ctx,
		`SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations' LIMIT 1`,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("database schema is not initialized")
	}
	if err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	return nil
}

// Close closes both pools and releases the instance lock.
func (s *Store) Close() error {
	var errs []error
	if s.Reader != nil {
		if err := s.Reader.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.Writer != nil {
		if err := s.Writer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.lock != nil {
		if err := s.lock.Unlock(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func prepareDataDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data directory %s: %w", dir, err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat data directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("data directory %s is not a directory", dir)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure data directory %s: %w", dir, err)
	}
	return nil
}

func acquireLock(dir string) (*flock.Flock, error) {
	lock := flock.New(filepath.Join(dir, "iwmmd.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock data directory: %w", err)
	}
	if !ok {
		return nil, ErrLocked
	}
	return lock, nil
}

func dataSourceName(opts Options) (string, error) {
	if opts.DSN != "" {
		return opts.DSN, nil
	}
	if opts.DataDir == "" {
		return "", errors.New("data directory is required")
	}
	abs, err := filepath.Abs(DatabasePath(opts.DataDir))
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "temp_store(MEMORY)")
	u.RawQuery = q.Encode()
	return u.String(), nil
}
