package workspace

import (
	"context"
	"database/sql/driver"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	ssqlog "github.com/tstapler/stapler-squad/log"
)

// captureWarnLogs temporarily redirects the log package's injectable slog
// seam to a buffer and returns a function that restores the original logger
// and returns everything captured while redirected. Not safe to use from a
// test marked t.Parallel() -- it swaps process-global state.
func captureWarnLogs() func() string {
	buf := &strings.Builder{}
	var mu sync.Mutex
	h := slog.NewTextHandler(&syncWriter{w: buf, mu: &mu}, &slog.HandlerOptions{Level: slog.LevelDebug})
	original := ssqlog.SetSlogDefaultForTest(slog.New(h))
	return func() string {
		ssqlog.SetSlogDefaultForTest(original)
		mu.Lock()
		defer mu.Unlock()
		return buf.String()
	}
}

type syncWriter struct {
	w  *strings.Builder
	mu *sync.Mutex
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// newMockPostgresHandle wires up a sqlmock-backed *sql.Conn as the dedicated
// connection for a postgresHandle, the same shape Acquire() builds in
// production (one connection per held resource). db.SetMaxIdleConns(0) is
// set after checking the connection out (not before) so it doesn't
// prematurely trigger a close of sqlmock's single pooled fake connection --
// only once the connection under test is returned to the pool (via the
// production code's conn.Close()) does the pool actually invoke the driver
// Close(), which is what lets these tests observe/inject a Close() failure.
func newMockPostgresHandle(t *testing.T, resource string, key int64) (*PostgresAdvisoryLock, sqlmock.Sqlmock, *postgresHandle) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("db.Conn: %v", err)
	}
	db.SetMaxIdleConns(0)

	l := NewPostgresAdvisoryLock(db, 0)
	h := &postgresHandle{resource: resource, key: key, conn: conn, parent: l}
	l.handles[resource] = h

	return l, mock, h
}

// TestPostgresAdvisoryLock_Close_UnlocksBeforeClosingConnection verifies the
// PR's core fix: Close() must run "SELECT pg_advisory_unlock" on each held
// connection before closing it, so the lock still clears server-side even if
// the subsequent conn.Close() itself fails. sqlmock's ordered-expectations
// mode (the default) fails the test if the calls happen out of order.
func TestPostgresAdvisoryLock_Close_UnlocksBeforeClosingConnection(t *testing.T) {
	l, mock, _ := newMockPostgresHandle(t, "res-1", 42)

	mock.ExpectExec("SELECT pg_advisory_unlock").WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectClose()

	if err := l.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations (order or call missing): %v", err)
	}

	if len(l.handles) != 0 {
		t.Errorf("expected handles map to be cleared after Close, got %d entries", len(l.handles))
	}
}

// TestPostgresAdvisoryLock_Close_WarnsWhenConnectionCloseFails covers the
// PR's stated risk: if closing the connection doesn't succeed cleanly, the
// lock may remain held server-side until the connection is reaped, so
// Close() must surface a warning rather than staying silent.
//
// Note on how the failure is injected: database/sql's (*sql.Conn).Close()
// unconditionally discards the underlying driver connection's physical
// Close() error (verified by reading go/src/database/sql/sql.go: Close()
// calls c.close(nil), which calls dc.releaseConn(nil) -> db.putConn(dc, nil,
// true) -> putConnDBLocked, whose own dc.Close() return value is never
// captured) -- so a plain "make the mocked driver Close() return an error"
// injection can't reach handle.conn.Close()'s err at all; sqlmock records
// the call but (*sql.Conn).Close() still returns nil to our code. The one
// real, reachable way handle.conn.Close() returns non-nil is if the
// connection was already auto-closed by an earlier operation on it: when
// the advisory-unlock ExecContext call itself fails with driver.ErrBadConn,
// database/sql proactively closes and invalidates the *sql.Conn right then
// (see closemuRUnlockCondReleaseConn), so the explicit conn.Close() that
// follows immediately returns sql.ErrConnDone instead of nil. That's what
// this test exercises.
func TestPostgresAdvisoryLock_Close_WarnsWhenConnectionCloseFails(t *testing.T) {
	l, mock, _ := newMockPostgresHandle(t, "res-1", 99)

	mock.ExpectExec("SELECT pg_advisory_unlock").WithArgs(int64(99)).WillReturnError(driver.ErrBadConn)
	mock.ExpectClose() // triggered internally by the ErrBadConn auto-close, not by our explicit conn.Close()

	restore := captureWarnLogs()
	err := l.Close()
	logs := restore()

	if err != nil {
		t.Fatalf("Close returned error %v; current implementation always returns nil (errors are logged, not propagated)", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	t.Logf("captured logs: %q", logs)
	if !strings.Contains(logs, "lock may remain held until connection is reaped") {
		t.Errorf("expected a warning about the lock possibly remaining held, got logs:\n%s", logs)
	}
	if !strings.Contains(logs, "res-1") {
		t.Errorf("expected the warning to identify the affected resource, got logs:\n%s", logs)
	}

	if len(l.handles) != 0 {
		t.Errorf("expected handles map to be cleared even when the underlying Close failed, got %d entries", len(l.handles))
	}
}

// TestPostgresAdvisoryLock_Close_StillClosesWhenUnlockFails ensures a failed
// explicit pg_advisory_unlock doesn't stop Close() from still attempting to
// close the connection (which also releases the advisory lock as a
// fallback per Postgres's connection-scoped advisory lock semantics).
func TestPostgresAdvisoryLock_Close_StillClosesWhenUnlockFails(t *testing.T) {
	l, mock, _ := newMockPostgresHandle(t, "res-2", 7)

	mock.ExpectExec("SELECT pg_advisory_unlock").WithArgs(int64(7)).WillReturnError(errors.New("connection already broken"))
	mock.ExpectClose()

	restore := captureWarnLogs()
	err := l.Close()
	_ = restore()

	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected conn.Close() to still be called even though the explicit unlock failed: %v", err)
	}
}
