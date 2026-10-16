package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestSQLiteTransactionLock_Acquire_RollbackFailsAfterCreateTableError covers
// the first of the two Rollback call sites in Acquire(): the lock table's
// CREATE TABLE statement fails, Acquire tries to Rollback the still-open
// BEGIN IMMEDIATE transaction to release the database-wide write lock, and
// that Rollback itself fails. Per the PR's own risk statement, a silently
// swallowed failure here leaves the whole database write-locked for every
// future Acquire -- so this asserts both that a warning is logged and that
// Acquire still returns the original (create-table) error rather than
// masking it with the rollback failure.
func TestSQLiteTransactionLock_Acquire_RollbackFailsAfterCreateTableError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	createTableErr := errors.New("disk I/O error")
	rollbackErr := errors.New("cannot rollback - no transaction is active")

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS workspace_locks").WillReturnError(createTableErr)
	mock.ExpectRollback().WillReturnError(rollbackErr)

	l := NewSQLiteTransactionLock(db)

	restore := captureWarnLogs()
	_, err = l.Acquire(context.Background(), "res-1", 0)
	logs := restore()

	if err == nil {
		t.Fatal("expected Acquire to return an error")
	}
	if !strings.Contains(err.Error(), "create lock table") || !strings.Contains(err.Error(), createTableErr.Error()) {
		t.Errorf("expected the original create-table error to surface, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	if !strings.Contains(logs, "rollback failed after create-lock-table error") {
		t.Errorf("expected a warning about the failed rollback, got logs:\n%s", logs)
	}
	if !strings.Contains(logs, "res-1") {
		t.Errorf("expected the warning to identify the affected resource, got logs:\n%s", logs)
	}

	// The failed resource must not be left registered as held -- a stuck
	// phantom entry would make every subsequent Acquire on it think the
	// lock is already reentrant-held by this process.
	l.mu.Lock()
	_, stillTracked := l.locks["res-1"]
	l.mu.Unlock()
	if stillTracked {
		t.Error("resource should not remain tracked in l.locks after a failed Acquire")
	}
}

// TestSQLiteTransactionLock_Acquire_RollbackFailsAfterInsertError covers the
// second Rollback call site: the lock-record INSERT fails (e.g. because the
// resource is already locked by another owner) and the subsequent Rollback
// also fails.
func TestSQLiteTransactionLock_Acquire_RollbackFailsAfterInsertError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	insertErr := errors.New("UNIQUE constraint failed: workspace_locks.resource")
	rollbackErr := errors.New("cannot rollback - no transaction is active")

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS workspace_locks").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT OR REPLACE INTO workspace_locks").WillReturnError(insertErr)
	mock.ExpectRollback().WillReturnError(rollbackErr)

	l := NewSQLiteTransactionLock(db)

	restore := captureWarnLogs()
	_, err = l.Acquire(context.Background(), "res-2", 0)
	logs := restore()

	if err == nil {
		t.Fatal("expected Acquire to return an error")
	}
	if !strings.Contains(err.Error(), "insert lock record") {
		t.Errorf("expected the original insert error to surface, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	if !strings.Contains(logs, "rollback failed after insert-lock-record error") {
		t.Errorf("expected a warning about the failed rollback, got logs:\n%s", logs)
	}

	l.mu.Lock()
	_, stillTracked := l.locks["res-2"]
	l.mu.Unlock()
	if stillTracked {
		t.Error("resource should not remain tracked in l.locks after a failed Acquire")
	}
}

// TestSQLiteTransactionLock_Close_RollbackFailure covers Close()'s own
// Rollback call: per the file's package doc, SQLite locking here is
// database-wide (BEGIN IMMEDIATE), so a failed Rollback during Close leaves
// the write lock stuck for every future Acquire, not just this resource --
// this asserts the warning fires and that Close still clears its local
// bookkeeping (l.locks) so it doesn't also leave the in-memory map stuck,
// even though it can't do anything about the underlying database lock
// itself.
func TestSQLiteTransactionLock_Close_RollbackFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS workspace_locks").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT OR REPLACE INTO workspace_locks").WillReturnResult(sqlmock.NewResult(0, 0))

	l := NewSQLiteTransactionLock(db)
	handle, err := l.Acquire(context.Background(), "res-3", 0)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if !handle.IsValid() {
		t.Fatal("expected handle to be valid immediately after Acquire")
	}

	rollbackErr := errors.New("cannot rollback - no transaction is active")
	mock.ExpectRollback().WillReturnError(rollbackErr)

	restore := captureWarnLogs()
	closeErr := l.Close()
	logs := restore()

	if closeErr != nil {
		t.Fatalf("Close returned error %v; current implementation always returns nil (errors are logged, not propagated)", closeErr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	if !strings.Contains(logs, "rollback failed during Close") {
		t.Errorf("expected a warning about the failed rollback during Close, got logs:\n%s", logs)
	}
	if !strings.Contains(logs, "res-3") {
		t.Errorf("expected the warning to identify the affected resource, got logs:\n%s", logs)
	}

	l.mu.Lock()
	remaining := len(l.locks)
	l.mu.Unlock()
	if remaining != 0 {
		t.Errorf("expected l.locks to be cleared after Close even though the underlying rollback failed, got %d entries", remaining)
	}
}

// TestSQLiteTransactionLock_Acquire_Reentrant is a control case establishing
// that a held (non-released) handle is returned as-is on a second Acquire
// for the same resource, unaffected by the rollback-failure handling added
// above.
func TestSQLiteTransactionLock_Acquire_Reentrant(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS workspace_locks").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT OR REPLACE INTO workspace_locks").WillReturnResult(sqlmock.NewResult(0, 0))

	l := NewSQLiteTransactionLock(db)
	h1, err := l.Acquire(context.Background(), "res-4", time.Second)
	if err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}

	h2, err := l.Acquire(context.Background(), "res-4", time.Second)
	if err != nil {
		t.Fatalf("second (reentrant) Acquire failed: %v", err)
	}
	if h1 != h2 {
		t.Error("expected reentrant Acquire to return the same handle")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
