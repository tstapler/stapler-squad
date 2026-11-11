package shadowledger

// shadowledger.go — the ShadowRecorder behind the selector's Shadow
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
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/hostclaim"
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

func newClassLedger() *session.ClassLedger {
	return &session.ClassLedger{
		Calls: map[string]int64{}, Divergence: map[string]int64{}, NotCompared: map[string]int64{},
		Edges: map[string]session.EdgeStats{}, ConfigSeen: map[string]int64{},
	}
}

func ledgerCallKey(m session.EngineMethod, p session.CallPath) string {
	return string(m) + "|" + string(p)
}

func ledgerDivergenceKey(m session.EngineMethod, c session.DivergenceClass, cfg string, p session.CallPath) string {
	return strings.Join([]string{string(m), string(c), cfg, string(p)}, "|")
}

func normalizeConfigLabel(l string) string {
	if l == session.ConfigSeeded || l == session.ConfigEdited {
		return l
	}
	return session.ConfigUnlabeled
}

type liveEdge struct{ transition, read, diverge, skips atomic.Int64 }

type shadowKey struct {
	m session.EngineMethod
	p session.CallPath
}

type divKey struct {
	m session.EngineMethod
	c session.DivergenceClass
	f string
	p session.CallPath
}

var (
	shadowMethods = []session.EngineMethod{session.MethodCanTransition, session.MethodAllowedTransitions, session.MethodPendingGates, session.MethodValidateGates}
	shadowPaths   = []session.CallPath{session.PathTransition, session.PathRead}
	shadowClasses = []session.DivergenceClass{session.ClassGraph, session.ClassGateMissing, session.ClassGateExtra, session.ClassErrorKind, session.ClassGuardNew}
	ledgerClasses = []session.CallerClass{session.ClassOperator, session.ClassAgent, session.ClassSystem}
	shadowConfigs = []string{session.ConfigSeeded, session.ConfigEdited, session.ConfigUnlabeled}
)

// ShadowLedgerRecorder implements ShadowRecorder and ObservationRecorder. The
// call path touches atomics only; the mutex guards the divergence ring, which
// is written only when a divergence occurs.
type ShadowLedgerRecorder struct {
	class session.CallerClass // the class the unclassed ShadowRecorder methods record into
	dir   string
	clock hostclaim.Clock
	warn  func(format string, args ...any)

	base session.ShadowLedger // loaded at start; never mutated afterwards

	live map[session.CallerClass]*liveClass // fixed at construction, read-only after

	mu    sync.Mutex // guards ring, start
	ring  []session.ReproRecord
	start time.Time

	flushMu     sync.Mutex
	lastFlushAt time.Time
}

// liveClass is one class's in-process counters: atomics on the call path, a
// mutex only for the rare per-caller divergence map.
type liveClass struct {
	calls       *session.CounterSet[shadowKey]
	divergences *session.CounterSet[divKey]
	notCompared *session.CounterSet[session.NotComparedReason]
	configSeen  *session.CounterSet[string]
	edges       sync.Map // string -> *liveEdge
	edgeCount   atomic.Int64
	firstSample atomic.Int64
	lastSample  atomic.Int64
	active      atomic.Bool

	mu            sync.Mutex // guards callerDiv, callerEdgeDiv
	callerDiv     map[string]int64
	callerEdgeDiv map[string]int64
}

func newLiveClass() *liveClass {
	return &liveClass{
		calls:       session.NewCounterSet(crossShadowKeys()...),
		divergences: session.NewCounterSet(crossDivKeys()...),
		notCompared: session.NewCounterSet(session.NotComparedStatefulGate),
		configSeen:  session.NewCounterSet(shadowConfigs...),
		callerDiv:   map[string]int64{},

		callerEdgeDiv: map[string]int64{},
	}
}

var (
	_ session.ShadowRecorder        = (*ShadowLedgerRecorder)(nil)
	_ session.ClassedShadowRecorder = (*ShadowLedgerRecorder)(nil)
	_ session.ObservationRecorder   = (*ShadowLedgerRecorder)(nil)
)

// ShadowRecorderOption customizes NewShadowLedgerRecorder.
type ShadowRecorderOption func(*ShadowLedgerRecorder)

// WithShadowClass sets the caller class this recorder writes (default operator).
func WithShadowClass(c session.CallerClass) ShadowRecorderOption {
	return func(r *ShadowLedgerRecorder) { r.class = c }
}

// WithShadowClock sets the clock (a test seam).
func WithShadowClock(c hostclaim.Clock) ShadowRecorderOption {
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
		class: session.ClassOperator, dir: dir, clock: hostclaim.RealClock{},
		warn: func(f string, a ...any) { log.WarningLog().Printf(f, a...) },
		live: map[session.CallerClass]*liveClass{},
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
		r.base = session.ShadowLedger{Version: shadowLedgerVersion, Classes: map[string]*session.ClassLedger{}}
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
func (r *ShadowLedgerRecorder) forClass(c session.CallerClass) (session.CallerClass, *liveClass) {
	if lc := r.live[c]; lc != nil {
		return c, lc
	}
	return r.class, r.live[r.class]
}

// RecordShadowCall implements ShadowRecorder.
func (r *ShadowLedgerRecorder) RecordShadowCall(m session.EngineMethod, p session.CallPath) {
	r.RecordShadowCallFor(r.class, m, p)
}

// RecordShadowCallFor implements ClassedShadowRecorder.
func (r *ShadowLedgerRecorder) RecordShadowCallFor(class session.CallerClass, m session.EngineMethod, p session.CallPath) {
	_, lc := r.forClass(class)
	lc.active.Store(true)
	lc.calls.Inc(shadowKey{m, p})
	now := r.clock.Now().UnixNano()
	lc.firstSample.CompareAndSwap(0, now)
	lc.lastSample.Store(now)
}

// RecordDivergence implements ShadowRecorder.
func (r *ShadowLedgerRecorder) RecordDivergence(m session.EngineMethod, c session.DivergenceClass, config string, p session.CallPath) {
	r.RecordDivergenceFor(r.class, m, c, config, p)
}

// RecordDivergenceFor implements ClassedShadowRecorder.
func (r *ShadowLedgerRecorder) RecordDivergenceFor(class session.CallerClass, m session.EngineMethod, c session.DivergenceClass, config string, p session.CallPath) {
	_, lc := r.forClass(class)
	lc.active.Store(true)
	lc.divergences.Inc(divKey{m, c, normalizeConfigLabel(config), p})
}

// RecordNotCompared implements ShadowRecorder.
func (r *ShadowLedgerRecorder) RecordNotCompared(reason session.NotComparedReason) {
	r.RecordNotComparedFor(r.class, reason)
}

// RecordNotComparedFor implements ClassedShadowRecorder.
func (r *ShadowLedgerRecorder) RecordNotComparedFor(class session.CallerClass, reason session.NotComparedReason) {
	_, lc := r.forClass(class)
	lc.active.Store(true)
	lc.notCompared.Inc(reason)
}

// RecordObservation implements ObservationRecorder: per-edge counts, the
// window's config label, the per-caller divergence count and the redacted
// repro ring, all in the observation's class section.
func (r *ShadowLedgerRecorder) RecordObservation(o session.ShadowObservation) {
	class, lc := r.forClass(o.CallerClass)
	lc.active.Store(true)
	lc.configSeen.Inc(normalizeConfigLabel(o.Config))
	e := lc.edge(session.EdgeKey(o.From, o.To))
	if o.Path == session.PathTransition {
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
		lc.callerEdgeDiv[string(o.Caller)+"|"+session.EdgeKey(o.From, o.To)+"|"+string(o.Class)]++
		lc.mu.Unlock()
	}
	rec := session.ReproRecord{
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
func (r *ShadowLedgerRecorder) Snapshot() session.ShadowLedger {
	out := session.ShadowLedger{
		Version: shadowLedgerVersion, SavedAt: r.clock.Now().UTC(),
		Classes: map[string]*session.ClassLedger{},
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
	live := append([]session.ReproRecord(nil), r.ring...)
	r.mu.Unlock()
	repro := append(append([]session.ReproRecord(nil), r.base.Repro...), live...)
	if len(repro) > reproRingSize {
		repro = repro[len(repro)-reproRingSize:]
	}
	out.Repro = repro
	return out
}

// liveClassOrder lists the live classes in a stable order.
func (r *ShadowLedgerRecorder) liveClassOrder() []session.CallerClass {
	out := make([]session.CallerClass, 0, len(r.live))
	for c := range r.live {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (lc *liveClass) mergeInto(cl *session.ClassLedger) {
	for _, k := range lc.calls.Keys {
		if n := lc.calls.Get(k); n > 0 {
			cl.Calls[ledgerCallKey(k.m, k.p)] += n
		}
	}
	for _, k := range lc.divergences.Keys {
		if n := lc.divergences.Get(k); n > 0 {
			cl.Divergence[ledgerDivergenceKey(k.m, k.c, k.f, k.p)] += n
		}
	}
	for _, k := range lc.notCompared.Keys {
		if n := lc.notCompared.Get(k); n > 0 {
			cl.NotCompared[string(k)] += n
		}
	}
	for _, k := range lc.configSeen.Keys {
		if n := lc.configSeen.Get(k); n > 0 {
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
	for k, n := range lc.callerEdgeDiv {
		if cl.CallerEdgeDivergence == nil {
			cl.CallerEdgeDivergence = map[string]int64{}
		}
		cl.CallerEdgeDivergence[k] += n
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

func cloneClassLedger(c *session.ClassLedger) *session.ClassLedger {
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
	for k, v := range c.CallerEdgeDivergence {
		if out.CallerEdgeDivergence == nil {
			out.CallerEdgeDivergence = map[string]int64{}
		}
		out.CallerEdgeDivergence[k] = v
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

func (r *ShadowLedgerRecorder) load() session.ShadowLedger {
	l, err := ReadShadowLedgerFile(ShadowLedgerPath(r.dir))
	switch {
	case err == nil:
		return l
	case errors.Is(err, os.ErrNotExist):
		return session.ShadowLedger{}
	default:
		r.warn("[ShadowLedger] ledger unusable, starting a new window: %v", err)
		return session.ShadowLedger{}
	}
}

// ReadShadowLedgerFile reads and validates a ledger file; an oversized or
// corrupt file is an error, never a partial ledger.
func ReadShadowLedgerFile(path string) (session.ShadowLedger, error) {
	info, err := os.Stat(path)
	if err != nil {
		return session.ShadowLedger{}, err
	}
	if info.Size() > maxLedgerBytes {
		return session.ShadowLedger{}, fmt.Errorf("ledger %s is %d bytes, over the %d byte cap", path, info.Size(), maxLedgerBytes)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- the config dir's own ledger file
	if err != nil {
		return session.ShadowLedger{}, err
	}
	var l session.ShadowLedger
	if err := json.Unmarshal(data, &l); err != nil {
		return session.ShadowLedger{}, fmt.Errorf("ledger %s is corrupt: %w", path, err)
	}
	if l.Version != shadowLedgerVersion {
		return session.ShadowLedger{}, fmt.Errorf("ledger %s has version %d, want %d", path, l.Version, shadowLedgerVersion)
	}
	if l.Classes == nil {
		l.Classes = map[string]*session.ClassLedger{}
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
func NewConfigLabeler(fingerprint func() (string, error), clock hostclaim.Clock) func() string {
	if clock == nil {
		clock = hostclaim.RealClock{}
	}
	type entry struct {
		label   string
		expires time.Time
	}
	var cur atomic.Pointer[entry]
	var refreshing atomic.Bool
	seed := session.SeedFingerprint()
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
			return session.ConfigUnlabeled
		}
		defer refreshing.Store(false)
		label := session.ConfigUnlabeled
		if fp, err := fingerprint(); err == nil {
			label = session.ConfigEdited
			if fp == seed {
				label = session.ConfigSeeded
			}
		}
		cur.Store(&entry{label: label, expires: now.Add(labelerRefreshWindow)})
		return label
	}
}
