package dbpool

import (
	"errors"
	"fmt"
	"strings"
)

// UseCase names one logical database. It is the stable identity used in
// /actuator/db and in the db.logical metric attribute.
type UseCase string

const (
	// UseCaseCore is sessions.db: the session graph, backlog and workflow state.
	UseCaseCore UseCase = "core"
	// UseCaseEscape is analytics.db: analytics_events (escape_events moved out, see UseCaseEscapeEvents).
	UseCaseEscape UseCase = "escape"
	// UseCaseEscapeEvents is db/escape_events.db: the terminal escape-event stream,
	// on its own writer so its bulk inserts and retention deletes never queue
	// analytics_events inserts.
	UseCaseEscapeEvents UseCase = "escape_events"
)

// fkMode is how foreign-key enforcement is requested in the DSN.
type fkMode int

const (
	fkNone fkMode = iota
	fkShort
	fkLong
)

// Spec describes one database to open. Build it with a typed constructor.
type Spec struct {
	UseCase UseCase
	// PoolName is the dbobs label (metrics, logs, /actuator/db); the read pool is PoolName+"-read".
	PoolName string
	Path     string
	Class    Class
	Profile  Profile

	autoVacuumIncremental bool
	foreignKeys           fkMode
	entTimeCompat         bool
}

var errBadPath = errors.New("dbpool: path must be a plain file path")

func (s Spec) validate() error {
	switch {
	case s.UseCase == "" || s.PoolName == "":
		return errors.New("dbpool: use case and pool name are required")
	case s.Path == "" || s.Path == ":memory:":
		return fmt.Errorf("%w: %q", errBadPath, s.Path)
	case strings.ContainsAny(s.Path, "?#") || strings.HasPrefix(s.Path, "file:"):
		// sqlitedsn.WithWAL is a no-op on a path that already has a query.
		return fmt.Errorf("%w: %q", errBadPath, s.Path)
	case strings.EqualFold(s.Profile.Synchronous, "OFF") && s.Class != ClassCache:
		return fmt.Errorf("dbpool: synchronous=OFF is only allowed for %s databases, not %s", ClassCache, s.Class)
	}
	return nil
}

// CoreSpec is sessions.db: ent time compat, foreign keys, incremental auto_vacuum
// and a read pool of readers connections (0 disables the reader/writer split).
func CoreSpec(path string, readers int) Spec {
	p := ProfileFor(ClassState)
	p.Readers = readers
	return Spec{UseCase: UseCaseCore, PoolName: "ent", Path: path, Class: ClassState, Profile: p,
		autoVacuumIncremental: true, foreignKeys: fkShort, entTimeCompat: true}
}

// EscapeSpec is analytics.db: a single writer connection, foreign keys, incremental auto_vacuum.
func EscapeSpec(path string) Spec {
	return Spec{UseCase: UseCaseEscape, PoolName: "analytics", Path: path, Class: ClassTelemetry,
		Profile: ProfileFor(ClassTelemetry), autoVacuumIncremental: true, foreignKeys: fkLong}
}

// EscapeEventsSpec is db/escape_events.db: telemetry class with its own single
// writer. escape_events has no foreign keys, but ent's SQLite migrator refuses a
// connection without foreign_keys on, so it is requested like analytics.db. Incremental
// auto_vacuum lets retention deletes return pages.
func EscapeEventsSpec(path string) Spec {
	return Spec{UseCase: UseCaseEscapeEvents, PoolName: "escape-events", Path: path, Class: ClassTelemetry,
		Profile: ProfileFor(ClassTelemetry), autoVacuumIncremental: true, foreignKeys: fkLong}
}
