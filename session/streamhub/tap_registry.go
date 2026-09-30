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
	afterFunc func(d time.Duration, f func())

	mu          sync.Mutex // lock order: mu, then a sink's mu
	handles     map[string]*CaptureTap
	allOn       bool
	allDeadline time.Time // zero = no expiry
	sessions    map[string]time.Time
}

// TapRegistryOptions injects the registry's environment; zero fields use the
// process defaults.
type TapRegistryOptions struct {
	Now       func() time.Time
	EnvDir    string                          // value of CaptureTapDirEnv
	DirFn     func() (string, error)          // default: <config dir>/tap
	AfterFunc func(d time.Duration, f func()) // default: time.AfterFunc
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
		r.afterFunc = func(d time.Duration, f func()) { time.AfterFunc(d, f) }
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
// from CaptureTapDirEnv.
func DefaultTapRegistry() *TapRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewTapRegistry(TapRegistryOptions{EnvDir: os.Getenv(CaptureTapDirEnv)})
	})
	return defaultRegistry
}

// Dir returns the directory tap files are written to.
func (r *TapRegistry) Dir() string { return r.resolveDir() }

func (r *TapRegistry) resolveDir() string {
	if r.envDir != "" {
		return r.envDir
	}
	dir, err := r.dirFn()
	if err != nil {
		// A private per-user temp dir keeps the tap usable when the state dir is unresolvable.
		log.Warn("streamhub capture tap: cannot resolve state directory", "error", err)
		return filepath.Join(os.TempDir(), fmt.Sprintf("stapler-squad-tap-%d", os.Getuid()))
	}
	return dir
}

// Handle returns the session's tap handle: never nil, inert while the tap is
// off for that session, and the same pointer on every call.
func (r *TapRegistry) Handle(sessionName string) *CaptureTap {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.handleLocked(sessionName)
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
// TapDefaultTTL and is clamped to TapMaxTTL. Enabling clears an earlier cap or
// error stop for the affected sessions.
func (r *TapRegistry) Set(enabled bool, sessionIDs []string, ttl time.Duration) (TapStatus, error) {
	r.mu.Lock()
	r.sweepLocked()
	if err := r.setLocked(enabled, sessionIDs, ttl); err != nil {
		r.mu.Unlock()
		return TapStatus{}, err
	}
	st := r.statusLocked()
	r.mu.Unlock()
	return st, nil
}

func (r *TapRegistry) setLocked(enabled bool, ids []string, ttl time.Duration) error {
	if !enabled {
		return r.disableLocked(ids)
	}
	switch {
	case ttl <= 0:
		ttl = TapDefaultTTL
	case ttl > TapMaxTTL:
		ttl = TapMaxTTL
	}
	r.enableLocked(ids, r.now().Add(ttl))
	r.afterFunc(ttl, r.sweep)
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
		"scope", scope, "sessions", ids, "dir", r.resolveDir(), "expires_at", deadline)
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

// sweep turns off every scope whose TTL has ended.
func (r *TapRegistry) sweep() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
}

func (r *TapRegistry) sweepLocked() {
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
}

// TapSessionStatus describes one session's tap file.
type TapSessionStatus struct {
	Name         string
	Path         string
	Enabled      bool
	BytesWritten int64 // file size, including earlier appends
	Capped       bool
	Failed       bool
	ExpiresAt    time.Time // zero = no expiry
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
	r.sweepLocked()
	return r.statusLocked()
}

func (r *TapRegistry) statusLocked() TapStatus {
	st := TapStatus{Dir: r.resolveDir(), AllEnabled: r.allOn}
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
		s.mu.Lock()
		ss := TapSessionStatus{
			Name: name, Path: s.path, Enabled: s.enabled.Load(),
			BytesWritten: s.written, Capped: s.capped, Failed: s.failed,
		}
		s.mu.Unlock()
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
