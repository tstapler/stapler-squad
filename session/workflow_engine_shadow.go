package session

// workflow_engine_shadow.go — the ShadowRecorder behind the selector's Shadow
// mode: lock-free counters on the call path, a bounded once-a-minute ledger on
// disk (aggregates over closed-enum labels plus a ring of the last 200 repro
// records), so a restart does not reset a soak window.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

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

const (
	shadowLedgerFile     = "workflow-engine-shadow-ledger.json"
	shadowLedgerVersion  = 1
	reproRingSize        = 200
	maxLedgerBytes       = 1 << 20
	maxBinaryStarts      = 64
	maxLedgerEdges       = 256
	otherEdgeKey         = "other"
	shadowFlushInterval  = time.Minute
	labelerRefreshWindow = 30 * time.Second
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
	FirstSampleAt    time.Time        `json:"first_sample_at"`
	LastSampleAt     time.Time        `json:"last_sample_at"`
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

func newClassLedger() *ClassLedger {
	return &ClassLedger{
		Calls: map[string]int64{}, Divergence: map[string]int64{}, NotCompared: map[string]int64{},
		Edges: map[string]EdgeStats{}, ConfigSeen: map[string]int64{},
	}
}

func ledgerCallKey(m EngineMethod, p CallPath) string { return string(m) + "|" + string(p) }

func ledgerDivergenceKey(m EngineMethod, c DivergenceClass, cfg string, p CallPath) string {
	return strings.Join([]string{string(m), string(c), cfg, string(p)}, "|")
}

// EdgeKey renders an edge; an empty to (AllowedTransitions) renders "from->*".
func EdgeKey(from, to BacklogStatus) string {
	if to == "" {
		return string(from) + "->*"
	}
	return string(from) + "->" + string(to)
}

func normalizeConfigLabel(l string) string {
	if l == ConfigSeeded || l == ConfigEdited {
		return l
	}
	return ConfigUnlabeled
}

type liveEdge struct{ transition, read, diverge, skips atomic.Int64 }

type shadowKey struct {
	m EngineMethod
	p CallPath
}

type divKey struct {
	m EngineMethod
	c DivergenceClass
	f string
	p CallPath
}

var (
	shadowMethods = []EngineMethod{MethodCanTransition, MethodAllowedTransitions, MethodPendingGates, MethodValidateGates}
	shadowPaths   = []CallPath{PathTransition, PathRead}
	shadowClasses = []DivergenceClass{ClassGraph, ClassGateMissing, ClassGateExtra, ClassErrorKind, ClassGuardNew}
	ledgerClasses = []CallerClass{ClassOperator, ClassAgent, ClassSystem}
	shadowConfigs = []string{ConfigSeeded, ConfigEdited, ConfigUnlabeled}
)

// ShadowLedgerRecorder implements ShadowRecorder and ObservationRecorder. The
// call path touches atomics only; the mutex guards the divergence ring, which
// is written only when a divergence occurs.
type ShadowLedgerRecorder struct {
	class CallerClass // the class the unclassed ShadowRecorder methods record into
	dir   string
	clock Clock
	warn  func(format string, args ...any)

	base ShadowLedger // loaded at start; never mutated afterwards

	live map[CallerClass]*liveClass // fixed at construction, read-only after

	mu    sync.Mutex // guards ring, start
	ring  []ReproRecord
	start time.Time

	flushMu     sync.Mutex
	lastFlushAt time.Time
}

// liveClass is one class's in-process counters: atomics on the call path, a
// mutex only for the rare per-caller divergence map.
type liveClass struct {
	calls       *counterSet[shadowKey]
	divergences *counterSet[divKey]
	notCompared *counterSet[NotComparedReason]
	configSeen  *counterSet[string]
	edges       sync.Map // string -> *liveEdge
	edgeCount   atomic.Int64
	firstSample atomic.Int64
	lastSample  atomic.Int64
	active      atomic.Bool

	mu        sync.Mutex // guards callerDiv
	callerDiv map[string]int64
}

func newLiveClass() *liveClass {
	return &liveClass{
		calls:       newCounterSet(crossShadowKeys()...),
		divergences: newCounterSet(crossDivKeys()...),
		notCompared: newCounterSet(NotComparedStatefulGate),
		configSeen:  newCounterSet(shadowConfigs...),
		callerDiv:   map[string]int64{},
	}
}

var (
	_ ShadowRecorder        = (*ShadowLedgerRecorder)(nil)
	_ ClassedShadowRecorder = (*ShadowLedgerRecorder)(nil)
	_ ObservationRecorder   = (*ShadowLedgerRecorder)(nil)
)

// ShadowRecorderOption customizes NewShadowLedgerRecorder.
type ShadowRecorderOption func(*ShadowLedgerRecorder)

// WithShadowClass sets the caller class this recorder writes (default operator).
func WithShadowClass(c CallerClass) ShadowRecorderOption {
	return func(r *ShadowLedgerRecorder) { r.class = c }
}

// WithShadowClock sets the clock (a test seam).
func WithShadowClock(c Clock) ShadowRecorderOption {
	return func(r *ShadowLedgerRecorder) { r.clock = c }
}

// WithShadowWarn replaces the WARN sink (a test seam).
func WithShadowWarn(f func(format string, args ...any)) ShadowRecorderOption {
	return func(r *ShadowLedgerRecorder) { r.warn = f }
}

// ShadowLedgerDir resolves the ledger directory through config.GetConfigDir so
// a manual instance or a test never overwrites the live soak window.
func ShadowLedgerDir() (string, error) { return config.GetConfigDir() }

// ShadowLedgerPath is the ledger file inside dir.
func ShadowLedgerPath(dir string) string { return filepath.Join(dir, shadowLedgerFile) }

// NewShadowLedgerRecorder loads the ledger in dir (a corrupt or oversized file
// starts fresh with a WARN) and records this binary's start time.
func NewShadowLedgerRecorder(dir string, opts ...ShadowRecorderOption) *ShadowLedgerRecorder {
	r := &ShadowLedgerRecorder{
		class: ClassOperator, dir: dir, clock: realClock{},
		warn: func(f string, a ...any) { log.WarningLog().Printf(f, a...) },
		live: map[CallerClass]*liveClass{},
	}
	for _, c := range ledgerClasses {
		r.live[c] = newLiveClass()
	}
	for _, o := range opts {
		o(r)
	}
	if r.live[r.class] == nil {
		r.live[r.class] = newLiveClass()
	}
	r.start = r.clock.Now().UTC()
	if dir != "" {
		r.base = r.load()
	}
	if r.base.Classes == nil {
		r.base = ShadowLedger{Version: shadowLedgerVersion, Classes: map[string]*ClassLedger{}}
	}
	return r
}

func crossShadowKeys() []shadowKey {
	keys := make([]shadowKey, 0, len(shadowMethods)*len(shadowPaths))
	for _, m := range shadowMethods {
		for _, p := range shadowPaths {
			keys = append(keys, shadowKey{m, p})
		}
	}
	return keys
}

func crossDivKeys() []divKey {
	keys := make([]divKey, 0, len(shadowMethods)*len(shadowClasses)*len(shadowConfigs)*len(shadowPaths))
	for _, m := range shadowMethods {
		for _, c := range shadowClasses {
			for _, f := range shadowConfigs {
				for _, p := range shadowPaths {
					keys = append(keys, divKey{m, c, f, p})
				}
			}
		}
	}
	return keys
}

// forClass returns the live counters for c; an empty or unknown class records
// into the recorder's own class.
func (r *ShadowLedgerRecorder) forClass(c CallerClass) (CallerClass, *liveClass) {
	if lc := r.live[c]; lc != nil {
		return c, lc
	}
	return r.class, r.live[r.class]
}

// RecordShadowCall implements ShadowRecorder.
func (r *ShadowLedgerRecorder) RecordShadowCall(m EngineMethod, p CallPath) {
	r.RecordShadowCallFor(r.class, m, p)
}

// RecordShadowCallFor implements ClassedShadowRecorder.
func (r *ShadowLedgerRecorder) RecordShadowCallFor(class CallerClass, m EngineMethod, p CallPath) {
	_, lc := r.forClass(class)
	lc.active.Store(true)
	lc.calls.inc(shadowKey{m, p})
	now := r.clock.Now().UnixNano()
	lc.firstSample.CompareAndSwap(0, now)
	lc.lastSample.Store(now)
}

// RecordDivergence implements ShadowRecorder.
func (r *ShadowLedgerRecorder) RecordDivergence(m EngineMethod, c DivergenceClass, config string, p CallPath) {
	r.RecordDivergenceFor(r.class, m, c, config, p)
}

// RecordDivergenceFor implements ClassedShadowRecorder.
func (r *ShadowLedgerRecorder) RecordDivergenceFor(class CallerClass, m EngineMethod, c DivergenceClass, config string, p CallPath) {
	_, lc := r.forClass(class)
	lc.active.Store(true)
	lc.divergences.inc(divKey{m, c, normalizeConfigLabel(config), p})
}

// RecordNotCompared implements ShadowRecorder.
func (r *ShadowLedgerRecorder) RecordNotCompared(reason NotComparedReason) {
	r.RecordNotComparedFor(r.class, reason)
}

// RecordNotComparedFor implements ClassedShadowRecorder.
func (r *ShadowLedgerRecorder) RecordNotComparedFor(class CallerClass, reason NotComparedReason) {
	_, lc := r.forClass(class)
	lc.active.Store(true)
	lc.notCompared.inc(reason)
}

// RecordObservation implements ObservationRecorder: per-edge counts, the
// window's config label, the per-caller divergence count and the redacted
// repro ring, all in the observation's class section.
func (r *ShadowLedgerRecorder) RecordObservation(o ShadowObservation) {
	class, lc := r.forClass(o.CallerClass)
	lc.active.Store(true)
	lc.configSeen.inc(normalizeConfigLabel(o.Config))
	e := lc.edge(EdgeKey(o.From, o.To))
	if o.Path == PathTransition {
		e.transition.Add(1)
	} else {
		e.read.Add(1)
	}
	if o.SkippedStateful {
		e.skips.Add(1)
	}
	if o.Class == "" {
		return
	}
	e.diverge.Add(1)
	if o.Caller != "" {
		lc.mu.Lock()
		lc.callerDiv[string(o.Caller)+"|"+string(o.Class)]++
		lc.mu.Unlock()
	}
	rec := ReproRecord{
		At: r.clock.Now().UTC(), Class: string(class), Method: string(o.Method), Path: string(o.Path),
		From: string(o.From), To: string(o.To), ItemHash: o.ItemHash, Divergence: string(o.Class), Caller: string(o.Caller),
		Config: normalizeConfigLabel(o.Config), Default: o.Default, Enforce: o.Enforce,
	}
	r.mu.Lock()
	r.ring = append(r.ring, rec)
	if len(r.ring) > reproRingSize {
		r.ring = append(r.ring[:0], r.ring[len(r.ring)-reproRingSize:]...)
	}
	r.mu.Unlock()
}

// edge returns the counters for key, folding edges past maxLedgerEdges into
// "other" so a custom-stage explosion cannot grow the ledger without bound.
func (lc *liveClass) edge(key string) *liveEdge {
	if v, ok := lc.edges.Load(key); ok {
		return v.(*liveEdge)
	}
	if lc.edgeCount.Load() >= maxLedgerEdges {
		key = otherEdgeKey
	}
	v, loaded := lc.edges.LoadOrStore(key, &liveEdge{})
	if !loaded {
		lc.edgeCount.Add(1)
	}
	return v.(*liveEdge)
}

// Snapshot merges the loaded ledger with this process's live counts. It is
// read-only and safe to call concurrently with recording.
func (r *ShadowLedgerRecorder) Snapshot() ShadowLedger {
	out := ShadowLedger{
		Version: shadowLedgerVersion, SavedAt: r.clock.Now().UTC(),
		Classes: map[string]*ClassLedger{},
	}
	for k, v := range r.base.Classes {
		out.Classes[k] = cloneClassLedger(v)
	}
	out.BinaryStarts = capBinaryStarts(append(append([]time.Time(nil), r.base.BinaryStarts...), r.start))
	for _, class := range r.liveClassOrder() {
		lc := r.live[class]
		if class != r.class && !lc.active.Load() {
			continue
		}
		cl := out.Classes[string(class)]
		if cl == nil {
			cl = newClassLedger()
			out.Classes[string(class)] = cl
		}
		lc.mergeInto(cl)
	}

	r.mu.Lock()
	live := append([]ReproRecord(nil), r.ring...)
	r.mu.Unlock()
	repro := append(append([]ReproRecord(nil), r.base.Repro...), live...)
	if len(repro) > reproRingSize {
		repro = repro[len(repro)-reproRingSize:]
	}
	out.Repro = repro
	return out
}

// liveClassOrder lists the live classes in a stable order.
func (r *ShadowLedgerRecorder) liveClassOrder() []CallerClass {
	out := make([]CallerClass, 0, len(r.live))
	for c := range r.live {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (lc *liveClass) mergeInto(cl *ClassLedger) {
	for _, k := range lc.calls.keys {
		if n := lc.calls.get(k); n > 0 {
			cl.Calls[ledgerCallKey(k.m, k.p)] += n
		}
	}
	for _, k := range lc.divergences.keys {
		if n := lc.divergences.get(k); n > 0 {
			cl.Divergence[ledgerDivergenceKey(k.m, k.c, k.f, k.p)] += n
		}
	}
	for _, k := range lc.notCompared.keys {
		if n := lc.notCompared.get(k); n > 0 {
			cl.NotCompared[string(k)] += n
		}
	}
	for _, k := range lc.configSeen.keys {
		if n := lc.configSeen.get(k); n > 0 {
			cl.ConfigSeen[k] += n
		}
	}
	lc.mu.Lock()
	for k, n := range lc.callerDiv {
		if cl.CallerDivergence == nil {
			cl.CallerDivergence = map[string]int64{}
		}
		cl.CallerDivergence[k] += n
	}
	lc.mu.Unlock()
	lc.edges.Range(func(k, v any) bool {
		le := v.(*liveEdge)
		es := cl.Edges[k.(string)]
		es.TransitionSamples += le.transition.Load()
		es.ReadSamples += le.read.Load()
		es.Divergences += le.diverge.Load()
		es.StatefulSkips += le.skips.Load()
		cl.Edges[k.(string)] = es
		return true
	})
	if first := lc.firstSample.Load(); first != 0 {
		t := time.Unix(0, first).UTC()
		if cl.FirstSampleAt.IsZero() || t.Before(cl.FirstSampleAt) {
			cl.FirstSampleAt = t
		}
	}
	if last := lc.lastSample.Load(); last != 0 {
		if t := time.Unix(0, last).UTC(); t.After(cl.LastSampleAt) {
			cl.LastSampleAt = t
		}
	}
}

func cloneClassLedger(c *ClassLedger) *ClassLedger {
	out := newClassLedger()
	if c == nil {
		return out
	}
	for k, v := range c.Calls {
		out.Calls[k] = v
	}
	for k, v := range c.Divergence {
		out.Divergence[k] = v
	}
	for k, v := range c.NotCompared {
		out.NotCompared[k] = v
	}
	for k, v := range c.Edges {
		out.Edges[k] = v
	}
	for k, v := range c.ConfigSeen {
		out.ConfigSeen[k] = v
	}
	for k, v := range c.CallerDivergence {
		if out.CallerDivergence == nil {
			out.CallerDivergence = map[string]int64{}
		}
		out.CallerDivergence[k] = v
	}
	out.FirstSampleAt, out.LastSampleAt = c.FirstSampleAt, c.LastSampleAt
	return out
}

// capBinaryStarts keeps the earliest start (the span anchor) and the newest
// maxBinaryStarts-1, so history cannot be read from one young binary.
func capBinaryStarts(starts []time.Time) []time.Time {
	if len(starts) <= maxBinaryStarts {
		return starts
	}
	return append([]time.Time{starts[0]}, starts[len(starts)-(maxBinaryStarts-1):]...)
}

func (r *ShadowLedgerRecorder) load() ShadowLedger {
	l, err := ReadShadowLedgerFile(ShadowLedgerPath(r.dir))
	switch {
	case err == nil:
		return l
	case errors.Is(err, os.ErrNotExist):
		return ShadowLedger{}
	default:
		r.warn("[ShadowLedger] ledger unusable, starting a new window: %v", err)
		return ShadowLedger{}
	}
}

// ReadShadowLedgerFile reads and validates a ledger file; an oversized or
// corrupt file is an error, never a partial ledger.
func ReadShadowLedgerFile(path string) (ShadowLedger, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ShadowLedger{}, err
	}
	if info.Size() > maxLedgerBytes {
		return ShadowLedger{}, fmt.Errorf("ledger %s is %d bytes, over the %d byte cap", path, info.Size(), maxLedgerBytes)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- the config dir's own ledger file
	if err != nil {
		return ShadowLedger{}, err
	}
	var l ShadowLedger
	if err := json.Unmarshal(data, &l); err != nil {
		return ShadowLedger{}, fmt.Errorf("ledger %s is corrupt: %w", path, err)
	}
	if l.Version != shadowLedgerVersion {
		return ShadowLedger{}, fmt.Errorf("ledger %s has version %d, want %d", path, l.Version, shadowLedgerVersion)
	}
	if l.Classes == nil {
		l.Classes = map[string]*ClassLedger{}
	}
	return l, nil
}

// Flush writes the merged ledger atomically (temp file then rename).
func (r *ShadowLedgerRecorder) Flush() error {
	if r.dir == "" {
		return nil
	}
	r.flushMu.Lock()
	defer r.flushMu.Unlock()
	data, err := json.Marshal(r.Snapshot())
	if err != nil {
		return fmt.Errorf("marshal shadow ledger: %w", err)
	}
	path := ShadowLedgerPath(r.dir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write shadow ledger: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename shadow ledger: %w", err)
	}
	r.lastFlushAt = r.clock.Now()
	return nil
}

// FlushIfDue flushes when a minute has passed since the last flush (or none yet).
func (r *ShadowLedgerRecorder) FlushIfDue() error {
	r.flushMu.Lock()
	due := r.lastFlushAt.IsZero() || r.clock.Now().Sub(r.lastFlushAt) >= shadowFlushInterval
	r.flushMu.Unlock()
	if !due {
		return nil
	}
	return r.Flush()
}

// Run flushes once now (recording this binary's start) and then every minute
// until ctx ends, with a last flush on the way out.
func (r *ShadowLedgerRecorder) Run(ctx context.Context) {
	if err := r.Flush(); err != nil {
		r.warn("[ShadowLedger] flush failed: %v", err)
	}
	t := time.NewTicker(shadowFlushInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := r.Flush(); err != nil {
				r.warn("[ShadowLedger] final flush failed: %v", err)
			}
			return
		case <-t.C:
			if err := r.Flush(); err != nil {
				r.warn("[ShadowLedger] flush failed: %v", err)
			}
		}
	}
}

// NewConfigLabeler returns the selector's `config` labeler: "seeded" while the
// live stage configuration hashes to SeedFingerprint, "edited" once it does
// not, "unlabeled" when the fingerprint cannot be read. The result is cached
// for labelerRefreshWindow so the call path never reads the database per call.
func NewConfigLabeler(fingerprint func() (string, error), clock Clock) func() string {
	if clock == nil {
		clock = realClock{}
	}
	type entry struct {
		label   string
		expires time.Time
	}
	var cur atomic.Pointer[entry]
	var refreshing atomic.Bool
	seed := SeedFingerprint()
	return func() string {
		e := cur.Load()
		now := clock.Now()
		if e != nil && now.Before(e.expires) {
			return e.label
		}
		if !refreshing.CompareAndSwap(false, true) {
			if e != nil {
				return e.label
			}
			return ConfigUnlabeled
		}
		defer refreshing.Store(false)
		label := ConfigUnlabeled
		if fp, err := fingerprint(); err == nil {
			label = ConfigEdited
			if fp == seed {
				label = ConfigSeeded
			}
		}
		cur.Store(&entry{label: label, expires: now.Add(labelerRefreshWindow)})
		return label
	}
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
