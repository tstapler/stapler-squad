// Package dirwatch provides a directory watcher that costs no file descriptor
// per watched file. fsnotify's kqueue backend (macOS) opens one fd for every
// file inside every watched directory, so watching ~1,400 directories held
// ~9,000 fds in a long-lived instance. The FSEvents-backed watcher here holds
// no fds per watched path or stream (measured: +0 fds across 280 streams) but
// each stream locks an OS thread and the kernel refuses new ones near 512 per
// process, so streams are shared (RootAt, WithRootFunc), budgeted, and past the
// budget a path falls back to kqueue. On platforms where fsnotify is already
// cheap (inotify) NewFDFree reports false and callers keep using fsnotify.
package dirwatch

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
)

// backendEnv set to "fsnotify" disables the FSEvents backend (rollback knob if
// FSEvents delivery proves unreliable on a machine).
const backendEnv = "STAPLER_SQUAD_DIRWATCH"

var forceFSNotify atomic.Bool //nolint:gochecknoglobals // test/rollback switch read by New

// ForceFSNotify makes New skip FSEvents (true) or allow it again (false). For
// tests: TestMain forces it where tests need synchronous in-kernel delivery,
// and a serial (non-parallel) test may lift it to measure the FSEvents backend.
func ForceFSNotify(force bool) { forceFSNotify.Store(force) }

// Watcher mirrors the subset of *fsnotify.Watcher the repo's watchers use.
// Like fsnotify, a watch on a directory reports events for its direct
// children and for the directory itself, not for deeper descendants.
type Watcher interface {
	Add(name string) error
	Remove(name string) error
	Events() <-chan fsnotify.Event
	Errors() <-chan error
	Close() error
}

// Option configures New and NewFDFree.
type Option func(*config)

type config struct {
	name       string
	warnAt     int
	maxStreams int
	// rootFor maps a resolved watch path to the path whose FSEvents stream
	// should cover it, letting many sibling watches share one stream.
	rootFor func(resolved string) string
}

// WithRootFunc lets many watched paths share one underlying stream: fn maps a
// resolved watch path to the (ancestor-or-self) path to stream. Events are
// still filtered to the paths actually added.
func WithRootFunc(fn func(resolved string) string) Option {
	return func(c *config) { c.rootFor = fn }
}

// WithName labels the watcher in the fs.watches gauge and in cap warnings.
func WithName(name string) Option { return func(c *config) { c.name = name } }

// WithWarnAt sets the watch count above which a WARN is logged (once per
// crossing). Zero means DefaultWarnAt.
func WithWarnAt(n int) Option { return func(c *config) { c.warnAt = n } }

// DefaultWarnAt is deliberately above a normal install (hundreds of repos and
// worktrees) so crossing it means something is watching more than it uses.
const DefaultWarnAt = 4000

// New returns the cheapest available watcher: FSEvents on darwin (no fd per
// watched file), else fsnotify. The result counts its watches, exports them as
// the fs.watches gauge and warns once above the WithWarnAt threshold; it never
// refuses an Add, because a refused watch is a silently missed event.
func New(opts ...Option) (Watcher, error) {
	cfg := newConfig(opts)
	if !forceFSNotify.Load() && os.Getenv(backendEnv) != "fsnotify" {
		if w, ok := NewFDFree(opts...); ok {
			return instrument(w, cfg), nil
		}
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return instrument(fsnotifyWatcher{fw}, cfg), nil
}

func newConfig(opts []Option) config {
	cfg := config{warnAt: DefaultWarnAt, rootFor: func(p string) string { return p }}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.warnAt <= 0 {
		cfg.warnAt = DefaultWarnAt
	}
	if cfg.name == "" {
		cfg.name = "unnamed"
	}
	return cfg
}

type fsnotifyWatcher struct{ w *fsnotify.Watcher }

func (f fsnotifyWatcher) Add(name string) error         { return f.w.Add(name) }
func (f fsnotifyWatcher) Remove(name string) error      { return f.w.Remove(name) }
func (f fsnotifyWatcher) Events() <-chan fsnotify.Event { return f.w.Events }
func (f fsnotifyWatcher) Errors() <-chan error          { return f.w.Errors }
func (f fsnotifyWatcher) Close() error                  { return f.w.Close() }

// WithMaxStreams caps FSEvents streams process-wide before Adds degrade to
// kqueue; zero means DefaultMaxStreams (darwin only).
func WithMaxStreams(n int) Option { return func(c *config) { c.maxStreams = n } }

// RootAt makes every watched path under root share one stream on root. root is
// resolved through symlinks; paths outside it keep their own stream.
func RootAt(root string) Option {
	return WithRootFunc(func(p string) string {
		if r, err := filepath.EvalSymlinks(root); err == nil {
			root = r
		}
		if p == root || strings.HasPrefix(p, root+string(filepath.Separator)) {
			return root
		}
		return p
	})
}
