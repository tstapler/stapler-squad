package session

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// BacklogItemIDPrefix is prepended to every generated public backlog item ID
// (e.g. "bl_01HXYZ..."), distinguishing it at a glance from the legacy
// internal UUID primary key and letting a dispatcher route a lookup string
// without a database round trip.
const BacklogItemIDPrefix = "bl_"

// backlogItemIDEntropy is the shared, mutex-guarded monotonic entropy source
// for BacklogItemID generation. oklog/ulid/v2's ulid.Monotonic is explicitly
// documented as unsafe for concurrent use — it keeps mutable internal state
// (the last-generated randomness component) that it increments when two IDs
// are requested within the same millisecond, and concurrent callers racing
// on that state can produce duplicate or non-monotonic IDs. New(...) is
// called only while holding backlogItemIDMu, which serializes every
// generation across the process, giving both uniqueness and monotonic
// ordering under concurrent callers.
var (
	backlogItemIDMu      sync.Mutex
	backlogItemIDEntropy = ulid.Monotonic(rand.Reader, 0)
)

// BacklogItemID is a type-safe wrapper around a ULID used as the public,
// sortable, prefixed identifier for a backlog item ("bl_<26-char ULID>").
// The zero value is not a valid ID — construct one via NewBacklogItemID or
// ParseBacklogItemID.
type BacklogItemID struct {
	value ulid.ULID
	valid bool
}

// NewBacklogItemID generates a new BacklogItemID from the current time and
// the shared monotonic entropy source. Safe for concurrent use from any
// number of goroutines.
func NewBacklogItemID() (BacklogItemID, error) {
	backlogItemIDMu.Lock()
	defer backlogItemIDMu.Unlock()

	id, err := ulid.New(ulid.Timestamp(time.Now()), backlogItemIDEntropy)
	if err != nil {
		return BacklogItemID{}, fmt.Errorf("failed to generate backlog item id: %w", err)
	}
	return BacklogItemID{value: id, valid: true}, nil
}

// ParseBacklogItemID parses a string of the form "bl_<ULID>" into a
// BacklogItemID, returning a descriptive error if the prefix is missing or
// the remainder is not a well-formed ULID.
func ParseBacklogItemID(s string) (BacklogItemID, error) {
	if !strings.HasPrefix(s, BacklogItemIDPrefix) {
		return BacklogItemID{}, fmt.Errorf("backlog item id %q: missing required %q prefix", s, BacklogItemIDPrefix)
	}
	rest := strings.TrimPrefix(s, BacklogItemIDPrefix)
	parsed, err := ulid.ParseStrict(rest)
	if err != nil {
		return BacklogItemID{}, fmt.Errorf("backlog item id %q: invalid ULID after %q prefix: %w", s, BacklogItemIDPrefix, err)
	}
	return BacklogItemID{value: parsed, valid: true}, nil
}

// IsBacklogItemIDShape reports whether s looks like a BacklogItemID (i.e.
// starts with BacklogItemIDPrefix) without fully validating the ULID
// portion. Used by dispatch code to decide which lookup path to take before
// paying for full parsing/validation.
func IsBacklogItemIDShape(s string) bool {
	return strings.HasPrefix(s, BacklogItemIDPrefix)
}

// String returns the prefixed string form, e.g. "bl_01HXYZABCDEFGHJKMNPQRSTVWX".
// Returns "" for the zero value.
func (id BacklogItemID) String() string {
	if !id.valid {
		return ""
	}
	return BacklogItemIDPrefix + id.value.String()
}

// IsValid reports whether id was constructed via NewBacklogItemID or
// ParseBacklogItemID. The zero value is not valid.
func (id BacklogItemID) IsValid() bool { return id.valid }

// PublicID centralizes the "is this row's public_id absent" decision for
// BacklogItemData, per session/ent/schema/backlog_item.go's documented
// representation: the public_id column is Optional().Unique() without
// .Nillable(), so ent's generated Go field is a plain string and every unset
// row's underlying SQL value is NULL, which the ent client reads back as the
// Go zero value "". PublicID treats "" as absent and otherwise parses
// PublicIDRaw, returning (zero value, false) rather than a parse error for
// "" specifically, since absence is an expected, common state (every
// pre-backfill row), not a malformed input. Callers (Story 1.3's dispatcher,
// Story 1.4's backfill) must use this accessor instead of comparing
// PublicIDRaw == "" directly.
func (d *BacklogItemData) PublicID() (BacklogItemID, bool) {
	if d.PublicIDRaw == "" {
		return BacklogItemID{}, false
	}
	id, err := ParseBacklogItemID(d.PublicIDRaw)
	if err != nil {
		return BacklogItemID{}, false
	}
	return id, true
}
