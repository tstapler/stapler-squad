package session

// nudge_outcomes.go is the durable nudge_outcome_total{kind,outcome} counter
// (Story 4.1.2). Q5's exit criteria for the diagnose and stall nudge flags need
// real-world outcome counts that survive restarts, so every record is written
// through to a small JSON file next to the config. Volume is a handful of
// nudges per day at most, so write-through is cheaper than a flush loop and
// loses nothing on a crash.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// NudgeKind says which nudge path produced an outcome.
type NudgeKind string

const (
	// NudgeKindStall is the in-process stall nudge of the autonomous driver.
	NudgeKindStall NudgeKind = "stall"
	// NudgeKindDiagnose is the diagnose_nudge_session MCP tool.
	NudgeKindDiagnose NudgeKind = "diagnose"
)

// NudgeOutcomeLabel is the bounded outcome vocabulary of the counter.
type NudgeOutcomeLabel string

const (
	NudgeOutcomeDelivered     NudgeOutcomeLabel = "delivered"
	NudgeOutcomeUncertain     NudgeOutcomeLabel = "uncertain"
	NudgeOutcomeSkippedCap    NudgeOutcomeLabel = "skipped_cap"
	NudgeOutcomeFailed        NudgeOutcomeLabel = "failed"
	NudgeOutcomeCancelled     NudgeOutcomeLabel = "cancelled"
	NudgeOutcomeNotIdle       NudgeOutcomeLabel = "not_idle"
	NudgeOutcomeLeaseBusy     NudgeOutcomeLabel = "lease_busy"
	NudgeOutcomeOwnership     NudgeOutcomeLabel = "ownership"
	NudgeOutcomeRestartFailed NudgeOutcomeLabel = "restart_failed"
)

const nudgeOutcomesFile = "nudge_outcomes.json"

// NudgeOutcomeCount is one labeled series of nudge_outcome_total.
type NudgeOutcomeCount struct {
	Kind    NudgeKind         `json:"kind"`
	Outcome NudgeOutcomeLabel `json:"outcome"`
	Value   int64             `json:"value"`
}

// NudgeOutcomeReport is the endpoint body: totals plus the first-record time.
type NudgeOutcomeReport struct {
	// Since is when the first outcome was ever recorded; nil before any.
	Since  *time.Time          `json:"since"`
	Totals []NudgeOutcomeCount `json:"nudge_outcome_total"`
}

type nudgeOutcomeKey struct {
	Kind    NudgeKind
	Outcome NudgeOutcomeLabel
}

// nudgeOutcomeFile is the on-disk shape.
type nudgeOutcomeFile struct {
	Since  *time.Time          `json:"since"`
	Totals []NudgeOutcomeCount `json:"totals"`
}

// NudgeOutcomeRecorder counts nudge outcomes, in memory when path is empty and
// written through to path otherwise.
type NudgeOutcomeRecorder struct {
	mu     sync.Mutex
	path   string
	now    func() time.Time
	since  *time.Time
	totals map[nudgeOutcomeKey]int64
	otel   metric.Int64Counter
}

// NewNudgeOutcomeRecorder loads any totals already persisted under dir. An empty
// dir gives a memory-only recorder. now is a test seam; nil uses real time.
func NewNudgeOutcomeRecorder(dir string, now func() time.Time) *NudgeOutcomeRecorder {
	if now == nil {
		now = time.Now
	}
	r := &NudgeOutcomeRecorder{now: now, totals: map[nudgeOutcomeKey]int64{}}
	if c, err := telemetry.GetMeter().Int64Counter("nudge_outcome_total",
		metric.WithDescription("Nudge attempts by kind (stall, diagnose) and outcome")); err == nil {
		r.otel = c
	}
	if dir == "" {
		return r
	}
	r.path = filepath.Join(dir, nudgeOutcomesFile)
	r.load()
	return r
}

func (r *NudgeOutcomeRecorder) load() {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warn("nudge outcome ledger unreadable; starting empty", "path", r.path, "err", err)
		}
		return
	}
	var f nudgeOutcomeFile
	if err := json.Unmarshal(data, &f); err != nil {
		log.Warn("nudge outcome ledger corrupt; starting empty", "path", r.path, "err", err)
		return
	}
	r.since = f.Since
	for _, c := range f.Totals {
		r.totals[nudgeOutcomeKey{c.Kind, c.Outcome}] += c.Value
	}
}

// Record counts one outcome, logs it and persists the new totals.
func (r *NudgeOutcomeRecorder) Record(kind NudgeKind, outcome NudgeOutcomeLabel, sessionID string) {
	log.Info(fmt.Sprintf("nudge outcome kind=%s outcome=%s session=%s", kind, outcome, sessionID))
	if r.otel != nil {
		r.otel.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("kind", string(kind)), attribute.String("outcome", string(outcome))))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.since == nil {
		t := r.now().UTC()
		r.since = &t
	}
	r.totals[nudgeOutcomeKey{kind, outcome}]++
	r.persistLocked()
}

// persistLocked writes atomically; a failure is logged and the in-memory count stands.
func (r *NudgeOutcomeRecorder) persistLocked() {
	if r.path == "" {
		return
	}
	data, err := json.Marshal(nudgeOutcomeFile{Since: r.since, Totals: r.seriesLocked()})
	if err == nil {
		tmp := r.path + ".tmp"
		if err = os.WriteFile(tmp, data, 0o600); err == nil {
			err = os.Rename(tmp, r.path)
		}
	}
	if err != nil {
		log.Warn("nudge outcome ledger write failed; count kept in memory only", "path", r.path, "err", err)
	}
}

func (r *NudgeOutcomeRecorder) seriesLocked() []NudgeOutcomeCount {
	out := make([]NudgeOutcomeCount, 0, len(r.totals))
	for k, v := range r.totals {
		out = append(out, NudgeOutcomeCount{Kind: k.Kind, Outcome: k.Outcome, Value: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Outcome < out[j].Outcome
	})
	return out
}

// Snapshot returns the current totals, sorted by kind then outcome.
func (r *NudgeOutcomeRecorder) Snapshot() NudgeOutcomeReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	return NudgeOutcomeReport{Since: r.since, Totals: r.seriesLocked()}
}

var (
	defaultNudgeOutcomesMu sync.RWMutex
	defaultNudgeOutcomes   = NewNudgeOutcomeRecorder("", nil)
)

// InitNudgeOutcomes switches the process-wide recorder to the durable ledger in
// the config directory. Tests never call it, so they stay memory-only.
func InitNudgeOutcomes() {
	dir, err := config.GetConfigDir()
	if err != nil {
		log.Warn("nudge outcome ledger directory unavailable; totals will not persist", "err", err)
		return
	}
	setDefaultNudgeOutcomes(NewNudgeOutcomeRecorder(dir, nil))
}

func setDefaultNudgeOutcomes(r *NudgeOutcomeRecorder) {
	defaultNudgeOutcomesMu.Lock()
	defaultNudgeOutcomes = r
	defaultNudgeOutcomesMu.Unlock()
}

// DefaultNudgeOutcomes returns the process-wide recorder.
func DefaultNudgeOutcomes() *NudgeOutcomeRecorder {
	defaultNudgeOutcomesMu.RLock()
	defer defaultNudgeOutcomesMu.RUnlock()
	return defaultNudgeOutcomes
}

// RecordNudgeOutcome counts one outcome on the process-wide recorder.
func RecordNudgeOutcome(kind NudgeKind, outcome NudgeOutcomeLabel, sessionID string) {
	DefaultNudgeOutcomes().Record(kind, outcome, sessionID)
}

// SwapNudgeOutcomeRecorder installs r as the process-wide recorder and returns
// the function that restores the previous one (a seam for tests in other packages).
func SwapNudgeOutcomeRecorder(r *NudgeOutcomeRecorder) (restore func()) {
	prev := DefaultNudgeOutcomes()
	setDefaultNudgeOutcomes(r)
	return func() { setDefaultNudgeOutcomes(prev) }
}
