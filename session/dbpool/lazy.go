package dbpool

import (
	"errors"
	"sync"
	"time"
)

// Lazy opens a database on first use and closes it after it has been idle, so a
// rarely used small database costs no pool, WAL/SHM pair or file descriptors
// while unused. Use it only for rebuildable (telemetry or cache) databases:
// a state database stays open for the life of the process.
//
// Callers go through With so a database cannot be closed while a call uses it.
type Lazy struct {
	spec Spec
	idle time.Duration
	now  func() time.Time
	open func(Spec) (*Database, error)

	mu       sync.Mutex // guards the fields below; held across Open so concurrent first users share one open
	db       *Database
	inflight int
	lastUse  time.Time
	opens    int
	closed   bool
}

// ErrLazyClosed is returned by With after Close.
var ErrLazyClosed = errors.New("dbpool: lazy database is closed")

// NewLazy returns an unopened Lazy for spec that closes after idle without use.
func NewLazy(spec Spec, idle time.Duration) *Lazy {
	return &Lazy{spec: spec, idle: idle, now: time.Now, open: Open}
}

// With runs fn against the open database, opening it first if needed. The
// database is pinned open until fn returns.
func (l *Lazy) With(fn func(*Database) error) error {
	db, err := l.acquire()
	if err != nil {
		return err
	}
	defer l.release()
	return fn(db)
}

func (l *Lazy) acquire() (*Database, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, ErrLazyClosed
	}
	if l.db == nil {
		db, err := l.open(l.spec)
		if err != nil {
			return nil, err
		}
		l.db = db
		l.opens++
	}
	l.inflight++
	l.lastUse = l.now()
	return l.db, nil
}

func (l *Lazy) release() {
	l.mu.Lock()
	l.inflight--
	l.lastUse = l.now()
	l.mu.Unlock()
}

// CloseIdle closes the database when it is open, unused by any call and idle
// for at least the configured duration. It reports whether it closed.
func (l *Lazy) CloseIdle() (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.db == nil || l.inflight > 0 || l.now().Sub(l.lastUse) < l.idle {
		return false, nil
	}
	// Closed under the lock so a concurrent With cannot open a second Database
	// on the same file while this one is still checkpointing.
	db := l.db
	l.db = nil
	return true, db.Close()
}

// Close closes the database if open, regardless of idleness, and makes later
// With calls fail with ErrLazyClosed. Callers must have stopped using the Lazy.
func (l *Lazy) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	db := l.db
	l.db = nil
	if db == nil {
		return nil
	}
	return db.Close()
}

// Opens reports how many times the database has been opened (for tests and metrics).
func (l *Lazy) Opens() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.opens
}
