package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// Resolution records how Selector.Resolve chose a backend for one call.
type Resolution struct {
	Feature string
	// Requested is the backend precedence selected before capability and
	// availability checks.
	Requested string
	// Used is the backend that actually serves the call.
	Used string
	// FallbackReason is non-empty only when Used != Requested.
	FallbackReason string
}

// FallbackRecorder persists a fallback Resolution. Must not block the call path.
type FallbackRecorder func(Resolution)

// Selector resolves a feature to a Backend: stage program > per-feature
// override > global default > claude. A backend that is unavailable or lacks a
// needed capability is replaced by claude with a logged, recorded reason.
// Settings are read through a provider on every call, so edits apply live.
type Selector struct {
	settings func() BackendSettings

	mu       sync.RWMutex
	backends map[string]Backend
	record   FallbackRecorder
}

// NewSelector returns a Selector reading live settings from settings (nil = zero settings).
func NewSelector(settings func() BackendSettings) *Selector {
	if settings == nil {
		settings = func() BackendSettings { return BackendSettings{} }
	}
	return &Selector{settings: settings, backends: make(map[string]Backend)}
}

// Register adds or replaces a backend under b.Name(). A nil b is ignored.
func (s *Selector) Register(b Backend) {
	if b == nil {
		return
	}
	s.mu.Lock()
	s.backends[b.Name()] = b
	s.mu.Unlock()
}

// SetFallbackRecorder installs the persistence hook for fallbacks.
func (s *Selector) SetFallbackRecorder(r FallbackRecorder) {
	s.mu.Lock()
	s.record = r
	s.mu.Unlock()
}

// Settings returns the current settings snapshot.
func (s *Selector) Settings() BackendSettings { return s.settings() }

func (s *Selector) lookup(name string) (Backend, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.backends[name]
	return b, ok
}

// requestedBackend applies the precedence order without any capability checks.
func (s *Selector) requestedBackend(set BackendSettings, feature FeatureKey, stageProgram string) string {
	if stageProgram != "" {
		return stageProgram
	}
	if name := set.PerFeature[featureSettingKey(feature)]; name != "" {
		return name
	}
	if set.Default != "" {
		return set.Default
	}
	return BackendClaude
}

// Requested returns the backend name settings select for feature, before any
// capability or availability fallback.
func (s *Selector) Requested(feature FeatureKey) string {
	return s.requestedBackend(s.settings(), feature, "")
}

// Resolve picks the backend for feature. stageProgram is an explicit
// per-invocation program (e.g. a backlog stage executor) that outranks settings.
// The returned Backend is nil only when claude itself is unregistered and the
// request cannot be served; callers should treat that as ErrNoBackend.
func (s *Selector) Resolve(feature FeatureKey, stageProgram string, need Caps) (Backend, Resolution) {
	set := s.settings()
	res := Resolution{Feature: string(feature), Requested: s.requestedBackend(set, feature, stageProgram)}
	res.Used = res.Requested

	if b, ok := s.lookup(res.Requested); !ok {
		res.FallbackReason = "unsupported_program"
	} else if !b.Available() {
		res.FallbackReason = res.Requested + "_unavailable"
	} else if miss := b.Capabilities().Missing(need); len(miss) > 0 {
		res.FallbackReason = res.Requested + "_lacks_" + strings.Join(miss, "+")
	} else {
		return b, res
	}

	fb, ok := s.lookup(BackendClaude)
	if !ok || fb.Capabilities().Missing(need) != nil {
		res.Used = ""
		s.noteFallback(res)
		return nil, res
	}
	res.Used = BackendClaude
	s.noteFallback(res)
	return fb, res
}

func (s *Selector) noteFallback(res Resolution) {
	log.Warn("headless backend fallback", "feature", res.Feature, "requested", res.Requested, "used", res.Used, "reason", res.FallbackReason)
	s.mu.RLock()
	rec := s.record
	s.mu.RUnlock()
	if rec != nil {
		rec(res)
	}
}

// SelectingClient adapts a Selector to PoolClient so existing call sites switch
// backends by injection only. StageProgram, when set, outranks settings.
type SelectingClient struct {
	Selector     *Selector
	StageProgram string
}

var _ PoolClient = (*SelectingClient)(nil)

// CallBlocking resolves a backend for key, translates the model name for it,
// and guarantees sink fires exactly once (unpriced if the backend didn't price).
func (c *SelectingClient) CallBlocking(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions, sink CostSink) (string, error) {
	need := CapsNeeded(systemPrompt, opts)
	b, res := c.Selector.Resolve(key, c.StageProgram, need)
	if b == nil {
		return "", fmt.Errorf("%w: feature %q requested %q (%s)", ErrNoBackend, key, res.Requested, res.FallbackReason)
	}
	opts.Model = c.Selector.Settings().TranslateModel(b.Name(), opts.Model)

	reported := false
	wrapped := func(usd float64, priced bool) {
		reported = true
		if sink != nil {
			sink(usd, priced)
		}
	}
	out, err := b.CallBlocking(ctx, key, systemPrompt, userPrompt, opts, wrapped)
	if err == nil && !reported && sink != nil {
		sink(0, false)
	}
	return out, err
}

var (
	defaultSelectorMu sync.RWMutex
	defaultSelector   *Selector
)

// SetDefaultSelector installs the process-wide selector used by call sites that
// can't take an injected client (e.g. session.Instance helpers). Safe for concurrent use.
func SetDefaultSelector(s *Selector) {
	defaultSelectorMu.Lock()
	defaultSelector = s
	defaultSelectorMu.Unlock()
}

// DefaultSelector returns the installed selector, or nil before wiring.
func DefaultSelector() *Selector {
	defaultSelectorMu.RLock()
	defer defaultSelectorMu.RUnlock()
	return defaultSelector
}

// DefaultClient returns a PoolClient routed through the default selector, or
// nil (an untyped nil interface) before wiring.
func DefaultClient() PoolClient {
	if s := DefaultSelector(); s != nil {
		return &SelectingClient{Selector: s}
	}
	return nil
}

// ResumeEnv resolves the backend for a feature that must resume a claude
// conversation in a subprocess the caller builds itself. It returns extra
// environment entries for that subprocess (ANTHROPIC_BASE_URL for consolette,
// none for claude) or an error when no backend with Resume capability is
// available. With no default selector it returns (nil, nil): plain claude.
func ResumeEnv(feature FeatureKey) ([]string, error) {
	sel := DefaultSelector()
	if sel == nil {
		return nil, nil
	}
	b, res := sel.Resolve(feature, "", Caps{Resume: true, WorkDir: true})
	if b == nil {
		return nil, fmt.Errorf("%w: feature %q (%s)", ErrNoBackend, feature, res.FallbackReason)
	}
	if b.Name() == BackendConsolette {
		return []string{"ANTHROPIC_BASE_URL=" + sel.Settings().ConsoletteURL()}, nil
	}
	return nil, nil
}

// Backends returns the registered backends sorted by name.
func (s *Selector) Backends() []Backend {
	s.mu.RLock()
	out := make([]Backend, 0, len(s.backends))
	for _, b := range s.backends {
		out = append(out, b)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// NewFileFallbackRecorder appends each fallback as one JSON line to path, so the
// reason survives restarts. Write errors are logged, never returned to the call path.
func NewFileFallbackRecorder(path string) FallbackRecorder {
	var mu sync.Mutex
	return func(r Resolution) {
		line, err := json.Marshal(struct {
			Time string `json:"time"`
			Resolution
		}{time.Now().UTC().Format(time.RFC3339), r})
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			log.Warn("headless backend fallback not persisted", "path", path, "err", err)
			return
		}
		defer f.Close()
		if _, err := f.Write(append(line, '\n')); err != nil {
			log.Warn("headless backend fallback not persisted", "path", path, "err", err)
		}
	}
}
