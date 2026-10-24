package session

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/tstapler/stapler-squad/config"
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

// ClassModeSource is an EngineModeSource that resolves a mode per caller class.
type ClassModeSource interface {
	EngineModeSource
	ModeFor(class CallerClass) EngineMode
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
	// ClassGuardNew is a Default built-in guard that denies in the enforce
	// composition where the legacy path (an Unchecked caller) never asked.
	ClassGuardNew DivergenceClass = "guard_new"

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

// ClassedShadowRecorder is the optional recorder interface that keeps a
// separate series per caller class; the selector uses it when the recorder
// implements it and falls back to the unclassed methods otherwise.
type ClassedShadowRecorder interface {
	RecordShadowCallFor(class CallerClass, m EngineMethod, p CallPath)
	RecordDivergenceFor(class CallerClass, m EngineMethod, c DivergenceClass, config string, p CallPath)
	RecordNotComparedFor(class CallerClass, r NotComparedReason)
}

// ShadowObservation is one shadowed call as the ledger recorder needs it: the
// edge (To is empty for AllowedTransitions), both answers rendered as text, and
// a hash of the item id (never the id itself).
type ShadowObservation struct {
	Method           EngineMethod
	Path             CallPath
	From, To         BacklogStatus
	ItemHash         string
	Class            DivergenceClass // empty: no divergence
	Config           string
	Default, Enforce string
	SkippedStateful  bool
	// CallerClass and Caller identify the registry caller; empty means the
	// selector's own (operator-policy) methods were called.
	CallerClass CallerClass
	Caller      TransitionCaller
}

// ObservationRecorder is the optional richer recorder interface; the selector
// calls it once per shadowed call when the recorder implements it.
type ObservationRecorder interface {
	RecordObservation(ShadowObservation)
}

// shadowResult is what a compare closure reports back to shadow().
type shadowResult struct {
	Class            DivergenceClass
	Default, Enforce string
	SkippedStateful  bool
}

// HashItemID returns the redacted item reference used in repro records.
func HashItemID(id string) string {
	if id == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:4])
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

// CanTransition implements WorkflowEngine for the operator policy.
func (s *SwitchableWorkflowEngine) CanTransition(from, to BacklogStatus, snap *StageConfigSnapshot) bool {
	return s.operatorView().CanTransition(from, to, snap)
}

// AllowedTransitions implements WorkflowEngine for the operator policy.
func (s *SwitchableWorkflowEngine) AllowedTransitions(from BacklogStatus, snap *StageConfigSnapshot) []BacklogStatus {
	return s.operatorView().AllowedTransitions(from, snap)
}

// PendingGates implements WorkflowEngine for the operator policy.
func (s *SwitchableWorkflowEngine) PendingGates(item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot) ([]GateStatus, error) {
	return s.operatorView().PendingGates(item, to, snap)
}

// ValidateGates implements WorkflowEngine for the operator policy.
func (s *SwitchableWorkflowEngine) ValidateGates(item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot) error {
	return s.operatorView().ValidateGates(item, to, snap)
}

// ModeFor is the mode the selector applies to class: its enforce flag if on,
// else Shadow if the shadow flag is on, else Off. A source that is not a
// ClassModeSource only knows the operator mode; every other class is then
// capped at Shadow, since no enforce flag can exist for it yet.
func (s *SwitchableWorkflowEngine) ModeFor(class CallerClass) EngineMode {
	if s.modes == nil {
		return ModeOff
	}
	if cm, ok := s.modes.(ClassModeSource); ok {
		return cm.ModeFor(class)
	}
	m := s.modes.Mode()
	if class != ClassOperator && m == ModeEnforce {
		return ModeShadow
	}
	return m
}

// ForClass returns a WorkflowEngine bound to class and guard policy, so the
// gate and legacy callers share one interface. The legacy answer is the
// Default engine's.
func (s *SwitchableWorkflowEngine) ForClass(class CallerClass, guards GuardPolicy) WorkflowEngine {
	return engineView{s: s, pol: callPolicy{Class: class, Guards: guards}}
}

// ForClassQuiet is ForClass without enforce-denial counting, for re-deciding or
// probing a denial that was already counted.
func (s *SwitchableWorkflowEngine) ForClassQuiet(class CallerClass, guards GuardPolicy) WorkflowEngine {
	return engineView{s: s, pol: callPolicy{Class: class, Guards: guards}, quiet: true}
}

// ForCaller returns a WorkflowEngine bound to a registry entry: its class,
// guard policy and forwarding flag, with an Unchecked caller's legacy answer
// being "allow".
func (s *SwitchableWorkflowEngine) ForCaller(spec TransitionCallerSpec) WorkflowEngine {
	return engineView{
		s:      s,
		pol:    callPolicy{Class: spec.Class, Guards: spec.Guards, Forwarding: spec.Forwarding},
		caller: spec.Name, unchecked: spec.Legacy == LegacyUnchecked,
	}
}

func (s *SwitchableWorkflowEngine) operatorView() engineView {
	return engineView{s: s, pol: operatorPolicy}
}

// engineView is one caller's lens on the selector: its policy, registry name
// and legacy behavior. Off and Shadow return the legacy answer.
type engineView struct {
	s         *SwitchableWorkflowEngine
	pol       callPolicy
	caller    TransitionCaller
	unchecked bool // a bare storage write today: the legacy answer is allow
	// quiet suppresses enforce-denial counting and logging: a re-decide or probe
	// of an already-recorded denial is not a new denial of a real attempt.
	quiet bool
}

// Quiet returns the same view without denial counting.
func (v engineView) Quiet() WorkflowEngine {
	v.quiet = true
	return v
}

// NoteGraphDenied counts a graph denial that the caller confirmed after
// evaluating quietly (no operator override freed it).
func (v engineView) NoteGraphDenied(from, to BacklogStatus) {
	if v.quiet || v.mode() != ModeEnforce {
		return
	}
	v.s.noteDenied(from, to, DenialConfigGraph)
}

var _ WorkflowEngine = engineView{}

func (v engineView) mode() EngineMode { return v.s.ModeFor(v.pol.Class) }

func (v engineView) legacyCan(from, to BacklogStatus, snap *StageConfigSnapshot) bool {
	return v.unchecked || v.s.def.CanTransition(from, to, snap)
}

func (v engineView) legacyGates(item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot) ([]GateStatus, error) {
	if v.unchecked {
		return nil, nil
	}
	return v.s.def.PendingGates(item, to, snap)
}

func (v engineView) legacyValidate(item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot) error {
	if v.unchecked {
		return nil
	}
	return v.s.def.ValidateGates(item, to, snap)
}

// CanTransition implements WorkflowEngine.
func (v engineView) CanTransition(from, to BacklogStatus, snap *StageConfigSnapshot) bool {
	s := v.s
	switch v.mode() {
	case ModeEnforce:
		allowed, reason := s.enf.canTransitionFor(v.pol, from, to, snap)
		s.noteFallback(reason)
		if !allowed && !v.quiet {
			s.noteDenied(from, to, DenialConfigGraph)
		}
		return allowed
	case ModeShadow:
		legacy := v.legacyCan(from, to, snap)
		v.shadow(MethodCanTransition, PathTransition, from, to, "", func() shadowResult {
			enf, _ := s.enf.canTransitionFor(v.pol, from, to, snap)
			r := shadowResult{Default: allowText(legacy), Enforce: allowText(enf)}
			if enf != legacy {
				r.Class = ClassGraph
			}
			return r
		})
		return legacy
	default:
		return v.legacyCan(from, to, snap)
	}
}

// OverridesGraph reports whether an operator's override_reason frees a graph
// denial. It is true only in Enforce, where the Configured graph is the
// authority; Off and Shadow keep the legacy answer. Only the operator
// transition RPC qualifies: other operator writers (OverrideVerdict) always
// carry a reason for a different purpose and must still honor a disabled edge.
func (v engineView) OverridesGraph(overrideReason string, to BacklogStatus) bool {
	if v.caller != CallerOperatorTransitionRPC || v.mode() != ModeEnforce || !v.s.enf.bypassesGraph(v.pol, overrideReason, to) {
		return false
	}
	v.s.infof("[SwitchableWorkflowEngine] operator override bypassed the graph to=%s", to)
	return true
}

// AllowedTransitions implements WorkflowEngine. It is a read: Default answers
// in Off and Shadow whatever the caller's legacy policy.
func (v engineView) AllowedTransitions(from BacklogStatus, snap *StageConfigSnapshot) []BacklogStatus {
	s := v.s
	switch v.mode() {
	case ModeEnforce:
		out, reason := s.enf.allowedTransitionsFor(v.pol, from, snap)
		s.noteFallback(reason)
		return out
	case ModeShadow:
		def := s.def.AllowedTransitions(from, snap)
		v.shadow(MethodAllowedTransitions, PathRead, from, "", "", func() shadowResult {
			enf, _ := s.enf.allowedTransitionsFor(v.pol, from, snap)
			r := shadowResult{Default: fmt.Sprint(def), Enforce: fmt.Sprint(enf)}
			if r.Default != r.Enforce {
				r.Class = ClassGraph
			}
			return r
		})
		return def
	default:
		return s.def.AllowedTransitions(from, snap)
	}
}

// PendingGates implements WorkflowEngine.
func (v engineView) PendingGates(item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot) ([]GateStatus, error) {
	s := v.s
	switch v.mode() {
	case ModeEnforce:
		statuses, _, reason, err := s.enf.pendingGatesFor(v.pol, item, to, snap, evalAllGates)
		s.noteFallback(reason)
		return statuses, err
	case ModeShadow:
		legacy, legacyErr := v.legacyGates(item, to, snap)
		v.shadow(MethodPendingGates, PathRead, item.Status, to, item.ItemID, func() shadowResult {
			enf, skipped, _, _ := s.enf.pendingGatesFor(v.pol, item, to, snap, evalStructuralOnly)
			v.noteSkipped(skipped)
			return shadowResult{
				Class:   v.compareGateStatuses(legacy, enf),
				Default: unsatisfiedText(legacy), Enforce: unsatisfiedText(enf), SkippedStateful: skipped,
			}
		})
		return legacy, legacyErr
	default:
		return v.legacyGates(item, to, snap)
	}
}

// ValidateGates implements WorkflowEngine.
func (v engineView) ValidateGates(item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot) error {
	s := v.s
	switch v.mode() {
	case ModeEnforce:
		ans := s.enf.validateGatesFor(v.pol, item, to, snap, evalAllGates)
		s.noteFallback(ans.Fallback)
		if ans.Err != nil && !v.quiet {
			s.noteDenied(item.Status, to, ans.Source)
		}
		return ans.Err
	case ModeShadow:
		legacyErr := v.legacyValidate(item, to, snap)
		v.shadow(MethodValidateGates, PathTransition, item.Status, to, item.ItemID, func() shadowResult {
			ans := s.enf.validateGatesFor(v.pol, item, to, snap, evalStructuralOnly)
			v.noteSkipped(ans.SkippedStateful)
			return shadowResult{
				Class:   v.compareGateErrors(legacyErr, ans),
				Default: errText(legacyErr), Enforce: errText(ans.Err), SkippedStateful: ans.SkippedStateful,
			}
		})
		return legacyErr
	default:
		return v.legacyValidate(item, to, snap)
	}
}

// compareGateErrors classifies a validate divergence. For an Unchecked caller a
// Default-guard denial is guard_new (a guard the path never asked), an
// operator-gate denial gate_extra.
func (v engineView) compareGateErrors(legacy error, ans gateAnswer) DivergenceClass {
	if v.unchecked && legacy == nil && ans.Err != nil && ans.Source == DenialDefaultGuard {
		return ClassGuardNew
	}
	return compareGateErrors(legacy, ans.Err)
}

func (v engineView) compareGateStatuses(legacy, enf []GateStatus) DivergenceClass {
	cls := compareGateStatuses(legacy, enf)
	if v.unchecked && cls == ClassGateExtra {
		return classifyExtraGates(legacy, enf)
	}
	return cls
}

// classifyExtraGates returns guard_new when every extra unsatisfied gate is a
// Default built-in guard, else gate_extra.
func classifyExtraGates(legacy, enf []GateStatus) DivergenceClass {
	d := unsatisfiedKeys(legacy)
	for _, g := range enf {
		if !g.Satisfied && !d[fmt.Sprintf("%s/%s", g.GateID, g.Kind)] && !strings.HasPrefix(g.GateID, builtInGateIDPrefix) {
			return ClassGateExtra
		}
	}
	return ClassGuardNew
}

// builtInGateIDPrefix marks a Default TransitionGuard gate (builtInGuardGate).
const builtInGateIDPrefix = "builtin:"

func (v engineView) noteSkipped(skipped bool) {
	if skipped {
		v.recordNotCompared(NotComparedStatefulGate)
	}
}

func (v engineView) recordNotCompared(r NotComparedReason) {
	if cr, ok := v.s.recorder.(ClassedShadowRecorder); ok {
		cr.RecordNotComparedFor(v.pol.Class, r)
		return
	}
	v.s.recorder.RecordNotCompared(r)
}

// shadow runs compare under recover, records the call, and records the
// divergence class compare returns ("" means none). Read-path calls are
// sampled. Nothing in here may alter the caller's answer.
func (v engineView) shadow(m EngineMethod, p CallPath, from, to BacklogStatus, itemID string, compare func() shadowResult) {
	s := v.s
	if p == PathRead && !s.sampleRead() {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.shadowErrors.Add(1)
			s.warnf("[SwitchableWorkflowEngine] shadow %s panicked: %v\n%s", m, r, debug.Stack())
		}
	}()
	classed, isClassed := s.recorder.(ClassedShadowRecorder)
	if isClassed {
		classed.RecordShadowCallFor(v.pol.Class, m, p)
	} else {
		s.recorder.RecordShadowCall(m, p)
	}
	res := compare()
	label := s.configLabel()
	if res.Class != "" {
		if isClassed {
			classed.RecordDivergenceFor(v.pol.Class, m, res.Class, label, p)
		} else {
			s.recorder.RecordDivergence(m, res.Class, label, p)
		}
	}
	if or, ok := s.recorder.(ObservationRecorder); ok {
		or.RecordObservation(ShadowObservation{
			Method: m, Path: p, From: from, To: to, ItemHash: HashItemID(itemID), Class: res.Class,
			Config: label, Default: res.Default, Enforce: res.Enforce, SkippedStateful: res.SkippedStateful,
			CallerClass: v.pol.Class, Caller: v.caller,
		})
	}
}

func allowText(b bool) string {
	if b {
		return "allow"
	}
	return "deny"
}

func errText(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

func unsatisfiedText(statuses []GateStatus) string {
	keys := make([]string, 0, len(statuses))
	for k := range unsatisfiedKeys(statuses) {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Sprint(keys)
}

// ModeFromFlags collapses the shadow and enforce flags to the three legal
// modes: enforce wins, then shadow, else off.
func ModeFromFlags(shadow, enforce bool) EngineMode {
	switch {
	case enforce:
		return ModeEnforce
	case shadow:
		return ModeShadow
	default:
		return ModeOff
	}
}

// classEnforceFlag is one row of the class-to-flag table: the live flag whose
// value enforces the class, and the default declared once in config.
type classEnforceFlag struct {
	Name    string
	Default bool
}

// classEnforceFlags maps a class to its enforce flag; a class absent here
// resolves from the shadow flag alone (ADR-007 D8).
var classEnforceFlags = map[CallerClass]classEnforceFlag{
	ClassOperator: {Name: config.WorkflowEngineEnforceFeatureFlag, Default: config.WorkflowEngineEnforceDefault},
	ClassAgent:    {Name: config.WorkflowEngineEnforceAgentFeatureFlag, Default: config.WorkflowEngineEnforceAgentDefault},
	ClassSystem:   {Name: config.WorkflowEngineEnforceSystemFeatureFlag, Default: config.WorkflowEngineEnforceSystemDefault},
}

// ClassEnforceFlag returns class's enforce flag name and registered default;
// ok is false while no flag is registered for the class.
func ClassEnforceFlag(class CallerClass) (name string, def bool, ok bool) {
	f, ok := classEnforceFlags[class]
	return f.Name, f.Default, ok
}

// ModeForClassFlags is the production mode reader for class: both flags are
// read with their registered defaults, never plain GetFeatureFlag (which
// returns false when unset and would switch shadow off).
func ModeForClassFlags(class CallerClass, cfg *config.Config) EngineMode {
	shadow := cfg.GetFeatureFlagWithDefault(config.WorkflowEngineShadowFeatureFlag, config.WorkflowEngineShadowDefault)
	enforce := false
	if f, ok := classEnforceFlags[class]; ok {
		enforce = cfg.GetFeatureFlagWithDefault(f.Name, f.Default)
	}
	return ModeFromFlags(shadow, enforce)
}

// FlagClassModeSource resolves each class's mode from live config, cached per
// class for the TTL. Mode() is the operator class, so it also serves callers
// that only know EngineModeSource.
type FlagClassModeSource struct {
	byClass map[CallerClass]*CachedModeSource
}

var _ ClassModeSource = (*FlagClassModeSource)(nil)

// NewFlagClassModeSource reads load() at most once per ttl per class.
func NewFlagClassModeSource(load func() *config.Config, ttl time.Duration, clock Clock) *FlagClassModeSource {
	f := &FlagClassModeSource{byClass: map[CallerClass]*CachedModeSource{}}
	for _, c := range []CallerClass{ClassOperator, ClassAgent, ClassSystem} {
		f.byClass[c] = NewCachedModeSource(func() EngineMode { return ModeForClassFlags(c, load()) }, ttl, clock)
	}
	return f
}

// Mode implements EngineModeSource as the operator class.
func (f *FlagClassModeSource) Mode() EngineMode { return f.ModeFor(ClassOperator) }

// ModeFor implements ClassModeSource; an unknown class is Off.
func (f *FlagClassModeSource) ModeFor(class CallerClass) EngineMode {
	if src := f.byClass[class]; src != nil {
		return src.Mode()
	}
	return ModeOff
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
