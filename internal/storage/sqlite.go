package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/internal/history"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// SQLiteStore represents the SQLite database storage for history events.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore initializes a new SQLite connection and schema.
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	// Configure SQLite connection with WAL mode, busy_timeout, and _txlock=immediate
	db, err := sql.Open("sqlite", dsn+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.initializeSchema(); err != nil {
		return nil, err
	}

	return store, nil
}

// initializeSchema sets up the database schema if it doesn't exist.
func (s *SQLiteStore) initializeSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS history_events (
		id TEXT PRIMARY KEY,
		command TEXT NOT NULL,
		timestamp DATETIME,
		directory TEXT,
		exit_code INTEGER,
		program_source TEXT,
		is_redacted BOOLEAN
	);
	CREATE INDEX IF NOT EXISTS idx_timestamp ON history_events(timestamp);
	`
	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}
	return nil
}

// InsertEvent inserts a single history event with retry logic.
func (s *SQLiteStore) InsertEvent(ctx context.Context, e *history.Event) error {
	query := `
	INSERT INTO history_events (id, command, timestamp, directory, exit_code, program_source, is_redacted)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO NOTHING;
	`

	return s.executeWithRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, query, e.ID, e.Command, e.Timestamp, e.Directory, e.ExitCode, e.ProgramSource, e.IsRedacted)
		return err
	})
}

// InsertEvents performs a batch insert wrapped in a transaction.
func (s *SQLiteStore) InsertEvents(ctx context.Context, events []*history.Event) error {
	query := `
	INSERT INTO history_events (id, command, timestamp, directory, exit_code, program_source, is_redacted)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO NOTHING;
	`

	return s.executeWithRetry(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() // Rollback is a no-op if tx is already committed

		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, e := range events {
			_, err = stmt.ExecContext(ctx, e.ID, e.Command, e.Timestamp, e.Directory, e.ExitCode, e.ProgramSource, e.IsRedacted)
			if err != nil {
				return err
			}
		}

		return tx.Commit()
	})
}

// executeWithRetry implements exponential backoff retry logic.
func (s *SQLiteStore) executeWithRetry(ctx context.Context, operation func() error) error {
	const maxRetries = 5
	baseDelay := 50 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		err := operation()
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(baseDelay * (1 << i)): // Exponential backoff
			continue
		}
	}
	return fmt.Errorf("operation failed after %d retries", maxRetries)
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
