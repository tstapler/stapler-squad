package dbpool

import "time"

// Class is a durability class; it selects the default Profile.
type Class string

const (
	// ClassState holds must-not-lose data (sessions, backlog, workflow).
	ClassState Class = "state"
	// ClassTelemetry holds loss-tolerant, rebuildable data.
	ClassTelemetry Class = "telemetry"
	// ClassCache is for disposable data; the only class allowed synchronous=OFF.
	ClassCache Class = "cache"
)

// Profile is the per-class pragma and pool shape. Zero values keep the SQLite or
// driver default, so a profile only states what it deliberately changes.
type Profile struct {
	// Synchronous is "", "OFF", "NORMAL" or "FULL". "" keeps the driver default.
	Synchronous string
	// BusyTimeout is how long a connection retries on SQLITE_BUSY.
	BusyTimeout time.Duration
	// WALAutocheckpoint is the page threshold; 0 keeps the SQLite default (1000).
	WALAutocheckpoint int
	// JournalSizeLimit caps the -wal file left behind after a checkpoint, in bytes.
	JournalSizeLimit int64
	// Readers is the query_only read pool size; 0 serves reads from the writer.
	Readers int
}

const (
	defaultBusyTimeout      = 5 * time.Second
	defaultJournalSizeLimit = 64 << 20
)

// ProfileFor returns the default profile of a class.
//
// state: synchronous=NORMAL, as #1289 set for both ent pools. telemetry: NORMAL and
// wal_autocheckpoint=1000 are exactly what analytics.db ran before this
// package; ADR-036's larger autocheckpoint and a reader pool wait for a
// benchmark. cache: OFF is safe only because the data is disposable.
func ProfileFor(c Class) Profile {
	switch c {
	case ClassTelemetry:
		return Profile{Synchronous: "NORMAL", BusyTimeout: defaultBusyTimeout, WALAutocheckpoint: 1000, JournalSizeLimit: defaultJournalSizeLimit}
	case ClassCache:
		return Profile{Synchronous: "OFF", BusyTimeout: time.Second, WALAutocheckpoint: 4000, JournalSizeLimit: defaultJournalSizeLimit, Readers: 1}
	default:
		return Profile{Synchronous: "NORMAL", BusyTimeout: defaultBusyTimeout, JournalSizeLimit: defaultJournalSizeLimit, Readers: 4}
	}
}
