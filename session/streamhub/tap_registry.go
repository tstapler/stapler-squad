package streamhub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

const (
	// TapDefaultTTL is how long a runtime enable lasts when the caller gives none.
	TapDefaultTTL = 30 * time.Minute
	// TapMaxTTL is the longest a runtime enable can last; longer requests are clamped.
	TapMaxTTL = 4 * time.Hour
)

// ErrTapAllEnabled is returned when disabling named sessions while the tap is on
// for all sessions: the sessions would stay on, so the request is ambiguous.
var ErrTapAllEnabled = errors.New("capture tap is enabled for all sessions; disable all sessions first")

// TapRegistry owns the capture tap's runtime state. Streams hold a handle from
// Handle for their whole lifetime; enabling or disabling takes effect on live
// handles immediately, so no restart is needed.
//
// The directory is fixed server-side (never supplied by a caller): the
// STAPLER_SQUAD_CAPTURE_TAP_DIR override, else <state dir>/tap, so instance and
// workspace isolation apply. Setting that variable also enables the tap for all
// sessions at startup with no TTL; every runtime enable has a TTL.
type TapRegistry struct {
	now       func() time.Time
	envDir    string
	dirFn     func() (string, error)
	afterFunc func(d time.Duration, f func()) (stop func() bool)

	// mu never waits on a sink lock: sink methods called under it are atomics
	// and TryLock only, so a pump stuck on a hung disk cannot block the registry.
	mu          sync.Mutex
	stopTimer   func() bool // stops the single pending sweep timer; nil when none
	handles     map[string]*CaptureTap
	allOn       bool
	allDeadline time.Time // zero = no expiry
	sessions    map[string]time.Time
}

// TapRegistryOptions injects the registry's environment; zero fields use the
// process defaults.
type TapRegistryOptions struct {
	Now    func() time.Time
	EnvDir string                 // value of CaptureTapDirEnv
	DirFn  func() (string, error) // default: <config dir>/tap
	// AfterFunc schedules f after d and returns its stop function. Default: time.AfterFunc.
	AfterFunc func(d time.Duration, f func()) (stop func() bool)
}

// NewTapRegistry returns a registry that is off unless opts.EnvDir is set.
func NewTapRegistry(opts TapRegistryOptions) *TapRegistry {
	r := &TapRegistry{
		now:       opts.Now,
		envDir:    opts.EnvDir,
		dirFn:     opts.DirFn,
		afterFunc: opts.AfterFunc,
		handles:   map[string]*CaptureTap{},
		sessions:  map[string]time.Time{},
	}
	if r.now == nil {
		r.now = time.Now
	}
	if r.dirFn == nil {
		r.dirFn = defaultTapDir
	}
	if r.afterFunc == nil {
		r.afterFunc = func(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }
	}
	if r.envDir != "" {
		r.allOn = true
		log.Warn("streamhub capture tap ACTIVE: recording raw terminal output for all sessions, which may include secrets",
			"dir", r.envDir, "env", CaptureTapDirEnv, "max_bytes_per_file", defaultTapMaxBytes)
	}
	return r
}

func defaultTapDir() (string, error) {
	base, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "tap"), nil
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *TapRegistry
)

// DefaultTapRegistry returns the process-wide registry, created on first use
// from CaptureTapDirEnv. The server calls it at startup so the env override's
// ACTIVE warning is logged then.
func DefaultTapRegistry() *TapRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewTapRegistry(TapRegistryOptions{EnvDir: os.Getenv(CaptureTapDirEnv)})
	})
	return defaultRegistry
}

// Dir returns the directory tap files are written to, or "" when it cannot be
// resolved.
func (r *TapRegistry) Dir() string {
	dir, _ := r.resolveDir()
	return dir
}

// resolveDir fails rather than falling back to a predictable temp path: without
// a resolvable state directory the tap is unavailable.
func (r *TapRegistry) resolveDir() (string, error) {
	if r.envDir != "" {
		return r.envDir, nil
	}
	dir, err := r.dirFn()
	if err != nil {
		return "", fmt.Errorf("capture tap unavailable: cannot resolve state directory: %w", err)
	}
	return dir, nil
}

// TapName is the key a session's tap is recorded under: the session title, not
// the tmux session name. A distinct type so the two cannot be swapped silently.
type TapName string

// Handle returns the session's tap handle: never nil, inert while the tap is
// off for that session, and the same pointer on every call.
func (r *TapRegistry) Handle(name TapName) *CaptureTap {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.handleLocked(string(name))
}

func (r *TapRegistry) handleLocked(name string) *CaptureTap {
	if h, ok := r.handles[name]; ok {
		return h
	}
	s := &tapSink{reg: r, name: name, now: r.now, maxBytes: defaultTapMaxBytes}
	h := &CaptureTap{s: s}
	r.handles[name] = h
	on, dl := r.effectiveLocked(name)
	s.apply(on, dl)
	return h
}

// effectiveLocked reports whether name is recording and until when, taking the
// later deadline when both the all-sessions and per-session scopes apply.
func (r *TapRegistry) effectiveLocked(name string) (bool, time.Time) {
	on := false
	var dl time.Time
	merge := func(d time.Time) {
		switch {
		case !on:
			on, dl = true, d
		case dl.IsZero():
		case d.IsZero() || d.After(dl):
			dl = d
		}
	}
	if r.allOn {
		merge(r.allDeadline)
	}
	if d, ok := r.sessions[name]; ok {
		merge(d)
	}
	return on, dl
}

func (r *TapRegistry) reapplyLocked() {
	for name, h := range r.handles {
		on, dl := r.effectiveLocked(name)
		h.s.apply(on, dl)
	}
}

// Set enables or disables the tap. Empty sessionIDs means all sessions; a
// disable of all sessions also clears every per-session enable. ttl <= 0 uses
// TapDefaultTTL and is clamped to TapMaxTTL; ttl is ignored when disabling.
// Enabling clears an earlier cap or error stop for the affected sessions; a
// capped file is rotated to .old when it next opens. Enabling all sessions
// replaces the env override's no-expiry with the TTL, and disabling all
// sessions turns an env-enabled tap off until restart.
func (r *TapRegistry) Set(enabled bool, sessionIDs []string, ttl time.Duration) (TapStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	if err := r.setLocked(enabled, sessionIDs, ttl); err != nil {
		return TapStatus{}, err
	}
	return r.statusLocked(), nil
}

func (r *TapRegistry) setLocked(enabled bool, ids []string, ttl time.Duration) error {
	if !enabled {
		if err := r.disableLocked(ids); err != nil {
			return err
		}
		r.rescheduleLocked()
		return nil
	}
	if _, err := r.resolveDir(); err != nil {
		return err
	}
	switch {
	case ttl <= 0:
		ttl = TapDefaultTTL
	case ttl > TapMaxTTL:
		ttl = TapMaxTTL
	}
	r.enableLocked(ids, r.now().Add(ttl))
	r.rescheduleLocked()
	return nil
}

func (r *TapRegistry) enableLocked(ids []string, deadline time.Time) {
	scope := "all sessions"
	if len(ids) == 0 {
		r.allOn, r.allDeadline = true, deadline
		for _, h := range r.handles {
			h.s.reset()
		}
	} else {
		scope = "named sessions"
		for _, id := range ids {
			r.sessions[id] = deadline
			r.handleLocked(id).s.reset()
		}
	}
	r.reapplyLocked()
	log.Warn("streamhub capture tap ACTIVE: recording raw terminal output, which may include secrets",
		"scope", scope, "sessions", ids, "dir", r.Dir(), "expires_at", deadline)
}

func (r *TapRegistry) disableLocked(ids []string) error {
	if len(ids) == 0 {
		r.allOn, r.allDeadline = false, time.Time{}
		clear(r.sessions)
	} else {
		if r.allOn {
			return ErrTapAllEnabled
		}
		for _, id := range ids {
			delete(r.sessions, id)
		}
	}
	r.reapplyLocked()
	log.Warn("streamhub capture tap disabled", "sessions", ids)
	return nil
}

// rescheduleLocked keeps exactly one timer, set for the earliest deadline.
func (r *TapRegistry) rescheduleLocked() {
	if r.stopTimer != nil {
		r.stopTimer()
		r.stopTimer = nil
	}
	var earliest time.Time
	consider := func(d time.Time) {
		if !d.IsZero() && (earliest.IsZero() || d.Before(earliest)) {
			earliest = d
		}
	}
	if r.allOn {
		consider(r.allDeadline)
	}
	for _, d := range r.sessions {
		consider(d)
	}
	if earliest.IsZero() {
		return
	}
	r.stopTimer = r.afterFunc(max(earliest.Sub(r.now()), 0), r.onTimer)
}

// onTimer expires finished scopes and re-arms the timer for the next deadline.
func (r *TapRegistry) onTimer() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	r.rescheduleLocked()
}

// sweepLocked turns off every scope whose TTL has ended and reports whether any did.
func (r *TapRegistry) sweepLocked() bool {
	now := r.now()
	expired := func(d time.Time) bool { return !d.IsZero() && !now.Before(d) }
	changed := false
	if r.allOn && expired(r.allDeadline) {
		r.allOn, r.allDeadline, changed = false, time.Time{}, true
		log.Warn("streamhub capture tap TTL expired: tap turned off for all sessions")
	}
	for id, d := range r.sessions {
		if expired(d) {
			delete(r.sessions, id)
			changed = true
			log.Warn("streamhub capture tap TTL expired: tap turned off", "session", id)
		}
	}
	if changed {
		r.reapplyLocked()
	}
	return changed
}

// TapSessionStatus describes one session's tap file.
type TapSessionStatus struct {
	Name         string
	Path         string
	Enabled      bool
	BytesWritten int64 // file size, including earlier appends
	Capped       bool
	Failed       bool
	// Rotated reports that the previous, full file was moved to Path+".old".
	Rotated   bool
	ExpiresAt time.Time // zero = no expiry
}

// TapStatus is a snapshot of the registry.
type TapStatus struct {
	Dir        string
	AllEnabled bool
	// Enabled reports whether any session is recording.
	Enabled bool
	// ExpiresAt is the latest deadline among the enabled scopes; zero when off or
	// when an enabled scope has no expiry (operator env override).
	ExpiresAt time.Time
	// SessionIDs lists sessions enabled by name, sorted.
	SessionIDs []string
	// Sessions lists handles that are recording or were stopped by the cap or an
	// error, sorted by name.
	Sessions []TapSessionStatus
}

// Status returns the current state after applying any expired TTLs.
func (r *TapRegistry) Status() TapStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sweepLocked() {
		r.rescheduleLocked()
	}
	return r.statusLocked()
}

func (r *TapRegistry) statusLocked() TapStatus {
	st := TapStatus{Dir: r.Dir(), AllEnabled: r.allOn}
	noExpiry := r.allOn && r.allDeadline.IsZero()
	if r.allOn {
		st.ExpiresAt = r.allDeadline
	}
	for id, d := range r.sessions {
		st.SessionIDs = append(st.SessionIDs, id)
		if d.IsZero() {
			noExpiry = true
		} else if d.After(st.ExpiresAt) {
			st.ExpiresAt = d
		}
	}
	if noExpiry {
		st.ExpiresAt = time.Time{}
	}
	st.Enabled = r.allOn || len(r.sessions) > 0
	sort.Strings(st.SessionIDs)
	for name, h := range r.handles {
		s := h.s
		ss := TapSessionStatus{
			Name: name, Enabled: s.enabled.Load(), BytesWritten: s.written.Load(),
			Capped: s.stop.Load() == tapCapped, Failed: s.stop.Load() == tapFailed,
			Rotated: s.rotated.Load(),
		}
		if p := s.pathv.Load(); p != nil {
			ss.Path = *p
		}
		if dl := s.deadline.Load(); dl != 0 {
			ss.ExpiresAt = time.Unix(0, dl)
		}
		if ss.Enabled || ss.Capped || ss.Failed {
			st.Sessions = append(st.Sessions, ss)
		}
	}
	sort.Slice(st.Sessions, func(i, j int) bool { return st.Sessions[i].Name < st.Sessions[j].Name })
	return st
}
