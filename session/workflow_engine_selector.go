package session

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/tstapler/stapler-squad/log"
)

// EngineMode is how the selector uses the enforce composition.
type EngineMode int

const (
	// ModeOff answers with the Default engine only.
	ModeOff EngineMode = iota
	// ModeShadow answers with Default and compares enforceAnswer on the same input.
	ModeShadow
	// ModeEnforce answers with enforceAnswer.
	ModeEnforce
)

// EngineModeSource supplies the live mode; implementations must be cheap and
// safe for concurrent use (see CachedModeSource).
type EngineModeSource interface {
	Mode() EngineMode
}

// EngineMethod, CallPath, DivergenceClass and NotComparedReason are the closed
// label sets of the shadow metrics.
type (
	EngineMethod      string
	CallPath          string
	DivergenceClass   string
	NotComparedReason string
)

const (
	MethodCanTransition      EngineMethod = "CanTransition"
	MethodAllowedTransitions EngineMethod = "AllowedTransitions"
	MethodPendingGates       EngineMethod = "PendingGates"
	MethodValidateGates      EngineMethod = "ValidateGates"

	// PathTransition is a real or attempted state change; PathRead is a UI
	// read, shadow-sampled one call in readSampleEvery.
	PathTransition CallPath = "transition"
	PathRead       CallPath = "read"

	ClassGraph       DivergenceClass = "graph"
	ClassGateMissing DivergenceClass = "gate_missing"
	ClassGateExtra   DivergenceClass = "gate_extra"
	ClassErrorKind   DivergenceClass = "error_kind"

	NotComparedStatefulGate NotComparedReason = "stateful_gate"
)

const (
	readSampleEvery = 10
	fallbackWarnGap = time.Minute
)

// ShadowRecorder receives shadow observations. It must be read-only and
// cheap; the selector recovers from any panic it raises. A nil recorder
// discards everything.
type ShadowRecorder interface {
	RecordShadowCall(m EngineMethod, p CallPath)
	RecordDivergence(m EngineMethod, c DivergenceClass, config string, p CallPath)
	RecordNotCompared(r NotComparedReason)
}

type discardRecorder struct{}

func (discardRecorder) RecordShadowCall(EngineMethod, CallPath)                          {}
func (discardRecorder) RecordDivergence(EngineMethod, DivergenceClass, string, CallPath) {}
func (discardRecorder) RecordNotCompared(NotComparedReason)                              {}

// counterSet is a fixed-key set of atomic counters: no lock, no allocation.
type counterSet[K comparable] struct {
	keys []K
	vals []atomic.Int64
}

func newCounterSet[K comparable](keys ...K) *counterSet[K] {
	return &counterSet[K]{keys: keys, vals: make([]atomic.Int64, len(keys))}
}

func (c *counterSet[K]) inc(k K) {
	for i, key := range c.keys {
		if key == k {
			c.vals[i].Add(1)
			return
		}
	}
}

func (c *counterSet[K]) get(k K) int64 {
	for i, key := range c.keys {
		if key == k {
			return c.vals[i].Load()
		}
	}
	return 0
}

// SwitchableWorkflowEngine is the WorkflowEngine seam over the Default engine,
// the enforce composition and a ShadowRecorder, selected live by an
// EngineModeSource. Off and Shadow always return the Default answer.
type SwitchableWorkflowEngine struct {
	def      *DefaultWorkflowEngine
	enf      enforceAnswer
	modes    EngineModeSource
	recorder ShadowRecorder
	clock    Clock

	configLabel func() string
	sampleRead  func() bool
	warnf       func(format string, args ...any)
	infof       func(format string, args ...any)

	readTicks        atomic.Uint64
	lastFallbackWarn atomic.Int64

	fallbacks    *counterSet[FallbackReason]
	denials      *counterSet[DenialSource]
	shadowErrors atomic.Int64
}

var _ WorkflowEngine = (*SwitchableWorkflowEngine)(nil)

// SelectorOption customizes a SwitchableWorkflowEngine.
type SelectorOption func(*SwitchableWorkflowEngine)

// WithShadowRecorder sets the recorder (default: discard).
func WithShadowRecorder(r ShadowRecorder) SelectorOption {
	return func(s *SwitchableWorkflowEngine) {
		if r != nil {
			s.recorder = r
		}
	}
}

// WithSelectorClock sets the clock used for the fallback WARN rate limit.
func WithSelectorClock(c Clock) SelectorOption {
	return func(s *SwitchableWorkflowEngine) { s.clock = c }
}

// WithConfigLabeler supplies the `config` label ("seeded" or "edited") for
// divergence series; the default labels everything "unlabeled".
func WithConfigLabeler(f func() string) SelectorOption {
	return func(s *SwitchableWorkflowEngine) { s.configLabel = f }
}

// WithReadSampler replaces the 1-in-10 read-path sampler (a test seam).
func WithReadSampler(f func() bool) SelectorOption {
	return func(s *SwitchableWorkflowEngine) { s.sampleRead = f }
}

// WithSelectorLoggers replaces the WARN and INFO sinks (a test seam).
func WithSelectorLoggers(warnf, infof func(format string, args ...any)) SelectorOption {
	return func(s *SwitchableWorkflowEngine) { s.warnf, s.infof = warnf, infof }
}

// NewSwitchableWorkflowEngine builds the selector. cfg may be nil (the
// enforce composition then answers from Default with reason cfg_nil); modes
// may be nil (always Off).
func NewSwitchableWorkflowEngine(def *DefaultWorkflowEngine, cfg *ConfiguredWorkflowEngine, modes EngineModeSource, opts ...SelectorOption) *SwitchableWorkflowEngine {
	s := &SwitchableWorkflowEngine{
		def:         def,
		enf:         enforceAnswer{def: def, cfg: cfg},
		modes:       modes,
		recorder:    discardRecorder{},
		clock:       realClock{},
		configLabel: func() string { return "unlabeled" },
		warnf:       func(f string, a ...any) { log.WarningLog().Printf(f, a...) },
		infof:       func(f string, a ...any) { log.InfoLog().Printf(f, a...) },
		fallbacks:   newCounterSet(FallbackCfgNil, FallbackCacheEmpty, FallbackLoadError),
		denials:     newCounterSet(DenialConfigGraph, DenialDefaultGuard, DenialOperatorGate),
	}
	s.sampleRead = func() bool { return s.readTicks.Add(1)%readSampleEvery == 0 }
	for _, o := range opts {
		o(s)
	}
	return s
}

// FallbackTotal returns workflow_engine_fallback_total{reason}.
func (s *SwitchableWorkflowEngine) FallbackTotal(r FallbackReason) int64 { return s.fallbacks.get(r) }

// EnforceDeniedTotal returns workflow_engine_enforce_denied_total{source}.
func (s *SwitchableWorkflowEngine) EnforceDeniedTotal(src DenialSource) int64 {
	return s.denials.get(src)
}

// ShadowErrorsTotal returns workflow_engine_shadow_errors_total.
func (s *SwitchableWorkflowEngine) ShadowErrorsTotal() int64 { return s.shadowErrors.Load() }

func (s *SwitchableWorkflowEngine) mode() EngineMode {
	if s.modes == nil {
		return ModeOff
	}
	return s.modes.Mode()
}

// CanTransition implements WorkflowEngine.
func (s *SwitchableWorkflowEngine) CanTransition(from, to BacklogStatus, snap *StageConfigSnapshot) bool {
	switch s.mode() {
	case ModeEnforce:
		allowed, reason := s.enf.canTransition(from, to, snap)
		s.noteFallback(reason)
		if !allowed {
			s.noteDenied(from, to, DenialConfigGraph)
		}
		return allowed
	case ModeShadow:
		def := s.def.CanTransition(from, to, snap)
		s.shadow(MethodCanTransition, PathTransition, func() DivergenceClass {
			if enf, _ := s.enf.canTransition(from, to, snap); enf != def {
				return ClassGraph
			}
			return ""
		})
		return def
	default:
		return s.def.CanTransition(from, to, snap)
	}
}

// AllowedTransitions implements WorkflowEngine.
func (s *SwitchableWorkflowEngine) AllowedTransitions(from BacklogStatus, snap *StageConfigSnapshot) []BacklogStatus {
	switch s.mode() {
	case ModeEnforce:
		out, reason := s.enf.allowedTransitions(from, snap)
		s.noteFallback(reason)
		return out
	case ModeShadow:
		def := s.def.AllowedTransitions(from, snap)
		s.shadow(MethodAllowedTransitions, PathRead, func() DivergenceClass {
			if enf, _ := s.enf.allowedTransitions(from, snap); fmt.Sprint(enf) != fmt.Sprint(def) {
				return ClassGraph
			}
			return ""
		})
		return def
	default:
		return s.def.AllowedTransitions(from, snap)
	}
}

// PendingGates implements WorkflowEngine.
func (s *SwitchableWorkflowEngine) PendingGates(item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot) ([]GateStatus, error) {
	switch s.mode() {
	case ModeEnforce:
		statuses, _, reason, err := s.enf.pendingGates(item, to, snap, evalAllGates)
		s.noteFallback(reason)
		return statuses, err
	case ModeShadow:
		def, defErr := s.def.PendingGates(item, to, snap)
		s.shadow(MethodPendingGates, PathRead, func() DivergenceClass {
			enf, skipped, _, _ := s.enf.pendingGates(item, to, snap, evalStructuralOnly)
			s.noteSkipped(skipped)
			return compareGateStatuses(def, enf)
		})
		return def, defErr
	default:
		return s.def.PendingGates(item, to, snap)
	}
}

// ValidateGates implements WorkflowEngine.
func (s *SwitchableWorkflowEngine) ValidateGates(item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot) error {
	switch s.mode() {
	case ModeEnforce:
		ans := s.enf.validateGates(item, to, snap, evalAllGates)
		s.noteFallback(ans.Fallback)
		if ans.Err != nil {
			s.noteDenied(item.Status, to, ans.Source)
		}
		return ans.Err
	case ModeShadow:
		defErr := s.def.ValidateGates(item, to, snap)
		s.shadow(MethodValidateGates, PathTransition, func() DivergenceClass {
			ans := s.enf.validateGates(item, to, snap, evalStructuralOnly)
			s.noteSkipped(ans.SkippedStateful)
			return compareGateErrors(defErr, ans.Err)
		})
		return defErr
	default:
		return s.def.ValidateGates(item, to, snap)
	}
}

// shadow runs compare under recover, records the call, and records the
// divergence class compare returns ("" means none). Read-path calls are
// sampled. Nothing in here may alter the caller's answer.
func (s *SwitchableWorkflowEngine) shadow(m EngineMethod, p CallPath, compare func() DivergenceClass) {
	if p == PathRead && !s.sampleRead() {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.shadowErrors.Add(1)
			s.warnf("[SwitchableWorkflowEngine] shadow %s panicked: %v\n%s", m, r, debug.Stack())
		}
	}()
	s.recorder.RecordShadowCall(m, p)
	if class := compare(); class != "" {
		s.recorder.RecordDivergence(m, class, s.configLabel(), p)
	}
}

func (s *SwitchableWorkflowEngine) noteSkipped(skipped bool) {
	if skipped {
		s.recorder.RecordNotCompared(NotComparedStatefulGate)
	}
}

func (s *SwitchableWorkflowEngine) noteDenied(from, to BacklogStatus, src DenialSource) {
	s.denials.inc(src)
	s.infof("[SwitchableWorkflowEngine] enforce denied from=%s to=%s reason=%s", from, to, src)
}

// noteFallback counts a fallback and WARNs at most once per fallbackWarnGap.
func (s *SwitchableWorkflowEngine) noteFallback(r FallbackReason) {
	if r == "" {
		return
	}
	s.fallbacks.inc(r)
	now := s.clock.Now().UnixNano()
	last := s.lastFallbackWarn.Load()
	if last != 0 && now-last < int64(fallbackWarnGap) {
		return
	}
	if s.lastFallbackWarn.CompareAndSwap(last, now) {
		s.warnf("[SwitchableWorkflowEngine] enforce falling back to the Default graph: reason=%s", r)
	}
}

func compareGateErrors(def, enf error) DivergenceClass {
	switch {
	case def != nil && enf == nil:
		return ClassGateMissing
	case def == nil && enf != nil:
		return ClassGateExtra
	case def != nil && !errors.Is(enf, def):
		return ClassErrorKind
	default:
		return ""
	}
}

func unsatisfiedKeys(statuses []GateStatus) map[string]bool {
	keys := make(map[string]bool)
	for _, g := range statuses {
		if !g.Satisfied {
			keys[fmt.Sprintf("%s/%s", g.GateID, g.Kind)] = true
		}
	}
	return keys
}

func compareGateStatuses(def, enf []GateStatus) DivergenceClass {
	d, e := unsatisfiedKeys(def), unsatisfiedKeys(enf)
	for k := range e {
		if !d[k] {
			return ClassGateExtra
		}
	}
	for k := range d {
		if !e[k] {
			return ClassGateMissing
		}
	}
	return ""
}

// CachedModeSource memoizes a mode loader for a TTL so the hot path never
// reads config per call; concurrent refreshes coalesce through singleflight.
type CachedModeSource struct {
	load  func() EngineMode
	ttl   time.Duration
	clock Clock
	cur   atomic.Pointer[modeEntry]
	group singleflight.Group
}

type modeEntry struct {
	mode    EngineMode
	expires time.Time
}

// NewCachedModeSource wraps load with a ttl cache; a nil clock uses real time.
func NewCachedModeSource(load func() EngineMode, ttl time.Duration, clock Clock) *CachedModeSource {
	if clock == nil {
		clock = realClock{}
	}
	return &CachedModeSource{load: load, ttl: ttl, clock: clock}
}

// Mode implements EngineModeSource.
func (c *CachedModeSource) Mode() EngineMode {
	if e := c.cur.Load(); e != nil && c.clock.Now().Before(e.expires) {
		return e.mode
	}
	v, _, _ := c.group.Do("mode", func() (any, error) {
		m := c.load()
		c.cur.Store(&modeEntry{mode: m, expires: c.clock.Now().Add(c.ttl)})
		return m, nil
	})
	return v.(EngineMode)
}
