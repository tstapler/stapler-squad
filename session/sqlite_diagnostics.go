package session

import (
	"database/sql"
	"fmt"
	"time"

	"claude-squad/log"
)

// SQLiteDiagnostics provides diagnostic information about SQLite connections
type SQLiteDiagnostics struct {
	db *sql.DB
}

// NewSQLiteDiagnostics creates a new diagnostics helper
func NewSQLiteDiagnostics(db *sql.DB) *SQLiteDiagnostics {
	return &SQLiteDiagnostics{db: db}
}

// LogConnectionPoolStats logs current connection pool statistics
func (d *SQLiteDiagnostics) LogConnectionPoolStats() {
	stats := d.db.Stats()
	log.InfoLog.Printf("=== SQLite Connection Pool Stats ===")
	log.InfoLog.Printf("  Open Connections: %d", stats.OpenConnections)
	log.InfoLog.Printf("  In Use: %d", stats.InUse)
	log.InfoLog.Printf("  Idle: %d", stats.Idle)
	log.InfoLog.Printf("  Wait Count: %d", stats.WaitCount)
	log.InfoLog.Printf("  Wait Duration: %v", stats.WaitDuration)
	log.InfoLog.Printf("  Max Idle Closed: %d", stats.MaxIdleClosed)
	log.InfoLog.Printf("  Max Idle Time Closed: %d", stats.MaxIdleTimeClosed)
	log.InfoLog.Printf("  Max Lifetime Closed: %d", stats.MaxLifetimeClosed)

	// Alert on potential issues
	if stats.WaitCount > 100 {
		log.ErrorLog.Printf("⚠️  High connection wait count: %d", stats.WaitCount)
	}
	if stats.WaitDuration > 5*time.Second {
		log.ErrorLog.Printf("⚠️  High connection wait duration: %v", stats.WaitDuration)
	}
	if stats.InUse == stats.OpenConnections && stats.OpenConnections > 0 {
		log.ErrorLog.Printf("⚠️  All connections in use - potential connection pool exhaustion")
	}
}

// CheckDatabaseLocks queries SQLite for active locks
func (d *SQLiteDiagnostics) CheckDatabaseLocks() error {
	// Query PRAGMA for lock information
	var lockStatus string
	err := d.db.QueryRow("PRAGMA locking_mode").Scan(&lockStatus)
	if err != nil {
		return fmt.Errorf("failed to check locking mode: %w", err)
	}
	log.InfoLog.Printf("SQLite locking mode: %s", lockStatus)

	// Check journal mode
	var journalMode string
	err = d.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		return fmt.Errorf("failed to check journal mode: %w", err)
	}
	log.InfoLog.Printf("SQLite journal mode: %s", journalMode)

	// Check busy timeout
	var busyTimeout int
	err = d.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout)
	if err != nil {
		return fmt.Errorf("failed to check busy timeout: %w", err)
	}
	log.InfoLog.Printf("SQLite busy timeout: %d ms", busyTimeout)

	return nil
}

// MeasureQueryTime measures and logs query execution time
func (d *SQLiteDiagnostics) MeasureQueryTime(name string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)

	if duration > 100*time.Millisecond {
		log.ErrorLog.Printf("⚠️  Slow query '%s': %v", name, duration)
	} else {
		log.InfoLog.Printf("Query '%s': %v", name, duration)
	}

	return err
}

// MonitorConnectionPool periodically logs connection pool stats
func (d *SQLiteDiagnostics) MonitorConnectionPool(interval time.Duration, done <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			d.LogConnectionPoolStats()
		}
	}
}

// GetDatabaseSize returns the size of the database file in bytes
func (d *SQLiteDiagnostics) GetDatabaseSize() (int64, error) {
	var pageCount, pageSize int64
	err := d.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	if err != nil {
		return 0, fmt.Errorf("failed to get page count: %w", err)
	}

	err = d.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	if err != nil {
		return 0, fmt.Errorf("failed to get page size: %w", err)
	}

	size := pageCount * pageSize
	log.InfoLog.Printf("Database size: %d pages × %d bytes = %d bytes (%.2f MB)",
		pageCount, pageSize, size, float64(size)/(1024*1024))

	return size, nil
}

// CheckIntegrity runs SQLite PRAGMA integrity_check
func (d *SQLiteDiagnostics) CheckIntegrity() error {
	rows, err := d.db.Query("PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("failed to run integrity check: %w", err)
	}
	defer rows.Close()

	log.InfoLog.Printf("=== SQLite Integrity Check ===")
	allOK := true
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("failed to scan integrity check result: %w", err)
		}
		log.InfoLog.Printf("  %s", result)
		if result != "ok" {
			allOK = false
		}
	}

	if allOK {
		log.InfoLog.Printf("✅ Database integrity check passed")
	} else {
		log.ErrorLog.Printf("❌ Database integrity check failed")
	}

	return rows.Err()
}
