//go:build darwin

package dirwatch

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/internal/fsevents"
)

const (
	streamLatency = 50 * time.Millisecond
	eventBuffer   = 4096
)

type rootStream struct {
	stream *fsevents.Stream
	refs   map[string]struct{} // resolved watch paths served by this stream
}

type fseventsWatcher struct {
	cfg    config
	events chan fsnotify.Event
	errs   chan error

	mu      sync.Mutex
	closed  bool
	byName  map[string]string // registered name -> resolved path
	byReal  map[string]string // resolved path -> registered name
	streams map[string]*rootStream
}

// NewFDFree returns an FSEvents-backed Watcher, or false if CoreServices
// cannot be loaded.
func NewFDFree(opts ...Option) (Watcher, bool) {
	cfg := config{rootFor: func(p string) string { return p }}
	for _, o := range opts {
		o(&cfg)
	}
	// Probe now so the caller can fall back to fsnotify instead of failing on Add.
	if _, err := fsevents.NewStream(fsevents.Config{Paths: []string{"/"}}); err != nil {
		return nil, false
	}
	return &fseventsWatcher{
		cfg:     cfg,
		events:  make(chan fsnotify.Event, eventBuffer),
		errs:    make(chan error, 16),
		byName:  make(map[string]string),
		byReal:  make(map[string]string),
		streams: make(map[string]*rootStream),
	}, true
}

func (w *fseventsWatcher) Events() <-chan fsnotify.Event { return w.events }
func (w *fseventsWatcher) Errors() <-chan error          { return w.errs }

func covers(root, p string) bool {
	return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
}

func (w *fseventsWatcher) Add(name string) error {
	name = filepath.Clean(name)
	// FSEvents reports resolved paths, so key on those.
	real, err := filepath.EvalSymlinks(name)
	if err != nil {
		return err
	}
	root := filepath.Clean(w.cfg.rootFor(real))
	if !covers(root, real) {
		root = real
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return fsnotify.ErrClosed
	}
	if _, dup := w.byName[name]; dup {
		w.mu.Unlock()
		return nil
	}
	rs, fresh, err := w.streamForLocked(root)
	if err != nil {
		w.mu.Unlock()
		return err
	}
	rs.refs[real] = struct{}{}
	w.byName[name] = real
	w.byReal[real] = name
	w.mu.Unlock()
	if fresh {
		// A started stream is not yet registered with fseventsd; flushing makes
		// writes after Add returns visible, as with kqueue. Outside mu: the
		// flush waits on callbacks that take mu.
		_ = rs
	}
	return nil
}

// streamForLocked returns the stream covering root, starting one if needed and
// absorbing any narrower streams so no event is delivered twice.
func (w *fseventsWatcher) streamForLocked(root string) (rs *rootStream, fresh bool, err error) {
	for r, rs := range w.streams {
		if covers(r, root) {
			return rs, false, nil
		}
	}
	s, err := fsevents.NewStream(fsevents.Config{
		Paths:     []string{root},
		SinceWhen: fsevents.SinceNow,
		Latency:   streamLatency,
		OnEvents:  w.onEvents,
	})
	if err != nil {
		return nil, false, err
	}
	if err := s.Start(); err != nil {
		return nil, false, err
	}
	rs = &rootStream{stream: s, refs: make(map[string]struct{})}
	for r, old := range w.streams {
		if covers(root, r) {
			for p := range old.refs {
				rs.refs[p] = struct{}{}
			}
			delete(w.streams, r)
			go old.stream.Stop() // never from under mu: Stop waits on the callback thread
		}
	}
	w.streams[root] = rs
	return rs, true, nil
}

func (w *fseventsWatcher) Remove(name string) error {
	name = filepath.Clean(name)
	w.mu.Lock()
	real, ok := w.byName[name]
	if !ok {
		w.mu.Unlock()
		return fsnotify.ErrNonExistentWatch
	}
	delete(w.byName, name)
	delete(w.byReal, real)
	var stop []*fsevents.Stream
	for r, rs := range w.streams {
		if _, has := rs.refs[real]; !has {
			continue
		}
		delete(rs.refs, real)
		if len(rs.refs) == 0 {
			delete(w.streams, r)
			stop = append(stop, rs.stream)
		}
	}
	w.mu.Unlock()
	for _, s := range stop {
		s.Stop()
	}
	return nil
}

func (w *fseventsWatcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	streams := w.streams
	w.streams = make(map[string]*rootStream)
	w.mu.Unlock()
	for _, rs := range streams {
		rs.stream.Stop()
	}
	// Callbacks hold mu for their whole body and see closed, so none can send now.
	w.mu.Lock()
	close(w.events)
	close(w.errs)
	w.mu.Unlock()
	return nil
}

// onEvents runs on the FSEvents run-loop thread and must not block.
func (w *fseventsWatcher) onEvents(evs []fsevents.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	for _, e := range evs {
		if e.Flags&(fsevents.MustScanSubDirs|fsevents.UserDropped|fsevents.KernelDropped|fsevents.EventIdsWrapped) != 0 {
			w.overflowLocked()
			continue
		}
		p := filepath.Clean(e.Path)
		name, ok := w.byReal[p]
		if !ok {
			parent, ok2 := w.byReal[filepath.Dir(p)]
			if !ok2 {
				continue
			}
			name = filepath.Join(parent, filepath.Base(p))
		}
		op := opsFor(e.Flags)
		if op == 0 {
			continue
		}
		select {
		case w.events <- fsnotify.Event{Name: name, Op: op}:
		default:
			w.overflowLocked() // a full buffer means lost events; consumers rescan on overflow
		}
	}
}

func (w *fseventsWatcher) overflowLocked() {
	select {
	case w.errs <- fsnotify.ErrEventOverflow:
	default:
	}
}

func opsFor(f fsevents.EventFlags) fsnotify.Op {
	var op fsnotify.Op
	if f&fsevents.ItemCreated != 0 {
		op |= fsnotify.Create
	}
	if f&fsevents.ItemModified != 0 {
		op |= fsnotify.Write
	}
	if f&fsevents.ItemRenamed != 0 {
		op |= fsnotify.Rename
	}
	if f&fsevents.ItemRemoved != 0 {
		op |= fsnotify.Remove
	}
	return op
}
