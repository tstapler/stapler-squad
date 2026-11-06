package deliverygate

import "time"

// StatsFileVersion is the schema version of the persisted stats document.
const StatsFileVersion = 1

// PersistedCounter is one reduced counter key (counter, kind, class) of a bucket.
type PersistedCounter struct {
	Counter string `json:"counter"`
	Kind    string `json:"kind"`
	Class   string `json:"class"`
	Count   int64  `json:"count"`
}

// PersistedBucket is one persisted hour. Durations are milliseconds.
type PersistedBucket struct {
	HourStart      time.Time          `json:"hour_start"`
	UptimeMs       int64              `json:"uptime_ms"`
	GateOnMs       int64              `json:"gate_on_ms"`
	GateOnMsByKind map[string]int64   `json:"gate_on_ms_by_kind,omitempty"`
	Seen           int64              `json:"hidden_events_seen"`
	WhileOn        int64              `json:"hidden_events_while_on"`
	RoutineWhileOn int64              `json:"routine_events_while_on"`
	Counters       []PersistedCounter `json:"counters,omitempty"`
}

// FlagChange is one recorded change of hidden_session_gate.
type FlagChange struct {
	Scope string    `json:"scope"` // "global" or "kind:<name>"
	Value bool      `json:"value"`
	At    time.Time `json:"at"`
	// Mutation is the FlagMutation enum name (SET_ENABLED, SET_DISABLED,
	// CLEAR_SCOPE, RESET_GLOBAL) so a clear never reads as an explicit false.
	Mutation string `json:"mutation"`
}

// PersistedStats is the document a StatsStore reads and writes. The store
// adapter owns Checksum, WriterPID and WrittenAt; Stats owns the rest.
type PersistedStats struct {
	Version      int                  `json:"version"`
	WriterPID    int                  `json:"writer_pid"`
	WrittenAt    time.Time            `json:"written_at"`
	ProcessStart time.Time            `json:"process_start"`
	Buckets      []PersistedBucket    `json:"buckets"`
	FlagHistory  []FlagChange         `json:"flag_history"`
	LastOffFlip  map[string]time.Time `json:"last_off_flip,omitempty"`
	Checksum     string               `json:"checksum"`
}

// StatsFileStatus is what the RPC reports about the persisted file.
type StatsFileStatus struct {
	Loaded      bool
	Quarantined bool
	Writable    bool
}

// StatsStore persists the hourly buckets. Implementations never block the
// publish path: only the writer goroutine and the shutdown flush call them.
type StatsStore interface {
	// Load returns the previous window, or the zero value and false when there
	// is none (missing, corrupt or quarantined file).
	Load() (PersistedStats, bool)
	// Save writes the document. Errors are reported through Status, not raised
	// to a caller that cannot act on them.
	Save(PersistedStats) error
	Status() StatsFileStatus
}

// NoopStatsStore is the default store: no disk, no goroutine. Writable is
// false so a gate without a real store never claims a soak figure survives.
type NoopStatsStore struct{}

func (NoopStatsStore) Load() (PersistedStats, bool) { return PersistedStats{}, false }
func (NoopStatsStore) Save(PersistedStats) error    { return nil }
func (NoopStatsStore) Status() StatsFileStatus      { return StatsFileStatus{} }
