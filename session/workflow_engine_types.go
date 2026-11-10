package session

// Workflow-engine types and the shadow ledger that the session core uses and the backlog lifecycle code builds on.

import (
	"sort"
	"time"
)

// StageDefinition is this package's plain-data view of one stage graph node.
// Field names/semantics mirror ent's BacklogStage schema
// (session/ent/schema/backlog_stage.go) exactly.
type StageDefinition struct {
	Slug       string
	IsEntry    bool
	IsTerminal bool
}

// TransitionDefinition is this package's plain-data view of one (fromStage,
// toStage) edge, mirroring ent's StageTransition schema
// (session/ent/schema/stage_transition.go). GateCount is the number of gates
// attached to this edge — Story 2.6.2's cycle-escape lint only needs "does
// this edge have at least one gate," never gate kind/config, so a count
// (rather than a slice of gate detail) keeps this struct storage-free and
// small.
type TransitionDefinition struct {
	FromSlug  string
	ToSlug    string
	Enabled   bool
	GateCount int
}

// CallerClass is the caller class a ledger section is kept for; the recorder
// keeps one section per class.
type CallerClass string

const (
	ClassOperator CallerClass = "operator"
	ClassAgent    CallerClass = "agent"
	ClassSystem   CallerClass = "system"
)

// Config labels of the `config` series.
const (
	ConfigSeeded    = "seeded"
	ConfigEdited    = "edited"
	ConfigUnlabeled = "unlabeled"
)

// EdgeStats counts shadowed calls over one (from,to) edge. Read-path samples
// never count toward soak evidence; StatefulSkips marks an edge whose stateful
// gate the shadow could not evaluate.
type EdgeStats struct {
	TransitionSamples int64 `json:"transition_samples"`
	ReadSamples       int64 `json:"read_samples"`
	Divergences       int64 `json:"divergences"`
	StatefulSkips     int64 `json:"stateful_skips"`
}

// ClassLedger is the persisted aggregate for one caller class. Keys are
// "Method|path", "Method|class|config|path" and the NotComparedReason.
type ClassLedger struct {
	Calls       map[string]int64     `json:"calls"`
	Divergence  map[string]int64     `json:"divergence"`
	NotCompared map[string]int64     `json:"not_compared"`
	Edges       map[string]EdgeStats `json:"edges"`
	ConfigSeen  map[string]int64     `json:"config_seen"`
	// CallerDivergence counts divergences per registry caller, keyed
	// "caller|divergence_class".
	CallerDivergence map[string]int64 `json:"caller_divergence,omitempty"`
	// CallerEdgeDivergence is the same count per edge, keyed
	// "caller|from->to|divergence_class": the unit of the owner's guard_new
	// acknowledgement.
	CallerEdgeDivergence map[string]int64 `json:"caller_edge_divergence,omitempty"`
	FirstSampleAt        time.Time        `json:"first_sample_at"`
	LastSampleAt         time.Time        `json:"last_sample_at"`
}

// ReproRecord is a redacted divergence: an item id hash, never a title or id.
type ReproRecord struct {
	At         time.Time `json:"at"`
	Class      string    `json:"class"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	ItemHash   string    `json:"item_hash,omitempty"`
	Divergence string    `json:"divergence"`
	Caller     string    `json:"caller,omitempty"`
	Config     string    `json:"config"`
	Default    string    `json:"default"`
	Enforce    string    `json:"enforce"`
}

// ShadowLedger is the whole persisted soak state.
type ShadowLedger struct {
	Version      int                     `json:"version"`
	BinaryStarts []time.Time             `json:"binary_starts"`
	Classes      map[string]*ClassLedger `json:"classes"`
	Repro        []ReproRecord           `json:"repro"`
	SavedAt      time.Time               `json:"saved_at"`
}

// EdgeKey renders an edge; an empty to (AllowedTransitions) renders "from->*".
func EdgeKey(from, to BacklogStatus) string {
	if to == "" {
		return string(from) + "->*"
	}
	return string(from) + "->" + string(to)
}

// sortedKeys returns m's keys in order, for stable output.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
