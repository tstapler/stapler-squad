// Package sqlitedsn builds modernc.org/sqlite DSN query strings.
//
// Every sql.Open("sqlite", ...) call site in this repo used to build its DSN
// with ad hoc fmt.Sprintf/string-concatenation, each reimplementing its own
// "?" vs "&" bookkeeping and its own copy of param names. That let a
// shared-cache in-memory DSN (which already carries its own "?query=string")
// end up with a corrupting second "?", and let param names drift (_fk=1 vs
// _foreign_keys=on) with no compiler check. Builder centralizes both: the
// "?"-vs-"&" decision is made once, from the path itself, so no call site can
// get it wrong, and every param has a single named method.
package sqlitedsn

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Builder incrementally assembles a modernc.org/sqlite DSN. The zero value
// is not usable; construct with New.
type Builder struct {
	path     string
	hasQuery bool
	params   []string
}

// New starts a Builder for the given database path or DSN. If path already
// contains a "?" — a URI-style DSN that carries its own query string, e.g. a
// shared-cache in-memory test database such as "file:test?cache=shared" —
// subsequent params are appended with "&" instead of "?", and WithWAL
// becomes a no-op, since shared-cache in-memory databases don't support WAL.
func New(path string) *Builder {
	return &Builder{path: path, hasQuery: strings.Contains(path, "?")}
}

func (b *Builder) with(param string) *Builder {
	b.params = append(b.params, param)
	return b
}

// WithWAL sets _journal_mode=WAL. No-op if the base path already carries a
// query string, since shared-cache in-memory databases don't support WAL.
func (b *Builder) WithWAL() *Builder {
	if b.hasQuery {
		return b
	}
	return b.with("_journal_mode=WAL")
}

// WithSynchronousNormal sets _synchronous=NORMAL — safe alongside WAL, since
// WAL's checkpoint (not every commit) is where an fsync is required for
// durability.
func (b *Builder) WithSynchronousNormal() *Builder {
	return b.with("_synchronous=NORMAL")
}

// WithForeignKeys enables foreign-key enforcement via _foreign_keys=on.
func (b *Builder) WithForeignKeys() *Builder {
	return b.with("_foreign_keys=on")
}

// WithForeignKeysShort enables foreign-key enforcement via the _fk=1 alias.
// Kept as a distinct method (rather than reusing WithForeignKeys) only to
// preserve existing DSNs byte-for-byte; both set the same PRAGMA.
func (b *Builder) WithForeignKeysShort() *Builder {
	return b.with("_fk=1")
}

// WithBusyTimeout sets busy_timeout so lock contention blocks up to d before
// returning SQLITE_BUSY, instead of modernc.org/sqlite's default of 0
// (immediate failure on contention).
func (b *Builder) WithBusyTimeout(d time.Duration) *Builder {
	return b.with("_timeout=" + strconv.FormatInt(d.Milliseconds(), 10))
}

// WithQueryOnly opens the connection read-only at the SQLite level, via
// _query_only=1.
func (b *Builder) WithQueryOnly() *Builder {
	return b.with("_query_only=1")
}

// WithTxLockImmediate sets _txlock=immediate: a transaction takes its
// RESERVED lock at BEGIN instead of on its first write statement, so it
// can't lose a write race to another connection in between.
func (b *Builder) WithTxLockImmediate() *Builder {
	return b.with("_txlock=immediate")
}

// WithPragma sets an arbitrary PRAGMA via the generic _pragma=name(value)
// syntax, for pragmas with no dedicated method above.
func (b *Builder) WithPragma(name, value string) *Builder {
	return b.with(fmt.Sprintf("_pragma=%s(%s)", name, value))
}

// WithEntTimeCompat sets the three params ent's generated UPDATE...RETURNING
// statements and UTC timestamp round-tripping require against
// modernc.org/sqlite:
//
//   - _texttotime=1: ent's generated UPDATE...RETURNING statements produce
//     result columns with an empty SQLite decltype (unlike plain SELECTs,
//     which report DATETIME and auto-convert without this flag) —
//     modernc.org/sqlite only upgrades those empty-decltype TEXT values to
//     time.Time when this param is set, otherwise Scan fails with
//     "unsupported Scan...storing driver.Value type string into type
//     *time.Time".
//   - _time_format=sqlite: without it, the driver writes time.Time values
//     using Go's time.Time.String() (e.g. "2006-01-02 15:04:05.999999999
//     -0700 MST"), which its own read-side parser (parseTimeFormats in
//     modernc.org/sqlite) never matches — the offset is space-separated with
//     a zone abbreviation, not the colon-separated "-07:00" the parser
//     expects. _time_format=sqlite makes the driver write with
//     parseTimeFormats[0], the exact layout its own reader tries first.
//   - _timezone=UTC: without it, a parsed time.Time carries a distinct,
//     driver-synthesized zero-offset *time.Location rather than the
//     time.UTC package singleton — the instant is correct either way, but
//     assert.Equal(t, time.UTC, x.Location()) (used throughout this repo's
//     tests) does a deep struct comparison and fails on that Location
//     identity mismatch. _timezone=UTC routes every read through the
//     driver's applyTimezone(t) -> t.In(time.UTC), which resolves to the
//     time.UTC singleton.
func (b *Builder) WithEntTimeCompat() *Builder {
	return b.with("_texttotime=1").with("_time_format=sqlite").with("_timezone=UTC")
}

// Build assembles the final DSN string.
func (b *Builder) Build() string {
	if len(b.params) == 0 {
		return b.path
	}
	sep := "?"
	if b.hasQuery {
		sep = "&"
	}
	return b.path + sep + strings.Join(b.params, "&")
}
