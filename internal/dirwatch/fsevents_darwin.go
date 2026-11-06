//go:build darwin

package dirwatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/internal/fsevents"
	"github.com/tstapler/stapler-squad/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	streamLatency = 50 * time.Millisecond
	eventBuffer   = 4096
)

type rootStream struct {
	stream *fsevents.Stream
	refs   map[string]struct{} // resolved watch paths served by this stream
	// startedAt/firstEvent feed fs.stream_first_event_lag: FSEvents can deliver
	// the first event seconds late (or never) with no error, so the lag is the
	// only visible signal.
	startedAt  time.Time
	firstEvent atomic.Bool
}

type fseventsWatcher struct {
	cfg    config
	events chan fsnotify.Event
	errs   chan error

	mu       sync.Mutex
	closed   bool
	byName   map[string]string   // registered name -> resolved path
	byReal   map[string][]string // resolved path -> registered names (aliases)
	streams  map[string]*rootStream
	fallback *fsnotify.Watcher // kqueue, for paths FSEvents cannot take (stream budget/start failure)
	fbNames  map[string]struct{}
	warned   bool
}

// fallbackWatches counts paths served by kqueue because FSEvents was unavailable.
var fallbackWatches atomic.Int64 //nolint:gochecknoglobals // feeds the fs.fallback_watches gauge

// DefaultMaxStreams leaves headroom under the ~512 per-process FSEvents limit
// for streams owned by other packages (e.g. the working-tree watcher).
const DefaultMaxStreams = 300

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
		byReal:  make(map[string][]string),
		streams: make(map[string]*rootStream),
		fbNames: make(map[string]struct{}),
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
	if _, dup := w.fbNames[name]; dup {
		w.mu.Unlock()
		return nil
	}
	rs, fresh, err := w.streamForLocked(root)
	if err != nil {
		// Stream budget spent or FSEventStreamStart refused: degrade to kqueue for
		// this path rather than silently dropping the watch.
		ferr := w.addFallbackLocked(name, err)
		w.mu.Unlock()
		return ferr
	}
	rs.refs[real] = struct{}{}
	w.byName[name] = real
	w.byReal[real] = append(w.byReal[real], name)
	w.mu.Unlock()
	if fresh {
		// A just-started stream is not yet registered with fseventsd, so a write
		// right after Add can be missed; flushing narrows that window (the stream
		// also replays from a pre-Start event id). Outside mu: the flush waits on
		// callbacks that take mu.
		rs.stream.FlushSync()
	}
	return nil
}

func (w *fseventsWatcher) addFallbackLocked(name string, cause error) error {
	if w.fallback == nil {
		kq, err := fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		w.fallback = kq
		go w.forwardFallback(kq)
	}
	if err := w.fallback.Add(name); err != nil {
		return err
	}
	w.fbNames[name] = struct{}{}
	fallbackWatches.Add(1)
	if !w.warned {
		w.warned = true
		log.Warn("dirwatch: FSEvents unavailable for a path, using kqueue (one fd per file in it)",
			"watcher", w.cfg.name, "path", name, "cause", cause, "streams", fsevents.LiveStreams())
	}
	return nil
}

func (w *fseventsWatcher) forwardFallback(kq *fsnotify.Watcher) {
	for {
		select {
		case ev, ok := <-kq.Events:
			if !ok {
				return
			}
			w.mu.Lock()
			if !w.closed {
				select {
				case w.events <- ev:
				default:
					w.overflowLocked()
				}
			}
			w.mu.Unlock()
		case err, ok := <-kq.Errors:
			if !ok {
				return
			}
			w.mu.Lock()
			if !w.closed {
				select {
				case w.errs <- err:
				default:
				}
			}
			w.mu.Unlock()
		}
	}
}

// streamForLocked returns the stream covering root, starting one if needed and
// absorbing any narrower streams so no event is delivered twice.
func (w *fseventsWatcher) streamForLocked(root string) (rs *rootStream, fresh bool, err error) {
	for r, rs := range w.streams {
		if covers(r, root) {
			return rs, false, nil
		}
	}
	max := int64(w.cfg.maxStreams)
	if max <= 0 {
		max = DefaultMaxStreams
	}
	if fsevents.LiveStreams() >= max {
		return nil, false, errStreamBudget
	}
	since := fsevents.CurrentEventID()
	if since == 0 {
		since = fsevents.SinceNow // 0 would replay the volume's whole history
	}
	rs = &rootStream{refs: make(map[string]struct{}), startedAt: time.Now()}
	s, err := fsevents.NewStream(fsevents.Config{
		Paths:       []string{root},
		SinceWhen:   since,
		Latency:     streamLatency,
		NoWatchRoot: true, // WatchRoot costs ~14 fds per stream; removals arrive as events on the parent
		OnEvents: func(evs []fsevents.Event) {
			if rs.firstEvent.CompareAndSwap(false, true) {
				recordFirstEventLag(w.cfg.name, time.Since(rs.startedAt))
			}
			w.onEvents(evs)
		},
	})
	if err != nil {
		return nil, false, err
	}
	if err := s.Start(); err != nil {
		return nil, false, err
	}
	rs.stream = s
	for r, old := range w.streams {
		if covers(root, r) {
			for p := range old.refs {
				rs.refs[p] = struct{}{}
			}
			delete(w.streams, r)
			go w.stopStream(old.stream) // never from under mu: Stop waits on the callback thread
		}
	}
	w.streams[root] = rs
	return rs, true, nil
}

func (w *fseventsWatcher) stopStream(s *fsevents.Stream) { s.Stop() }

var errStreamBudget = errors.New("dirwatch: FSEvents stream budget spent")

func (w *fseventsWatcher) Remove(name string) error {
	name = filepath.Clean(name)
	w.mu.Lock()
	if _, ok := w.fbNames[name]; ok {
		delete(w.fbNames, name)
		fallbackWatches.Add(-1)
		kq := w.fallback
		w.mu.Unlock()
		return kq.Remove(name)
	}
	real, ok := w.byName[name]
	if !ok {
		w.mu.Unlock()
		return fsnotify.ErrNonExistentWatch
	}
	delete(w.byName, name)
	names := w.byReal[real][:0:0]
	for _, n := range w.byReal[real] {
		if n != name {
			names = append(names, n)
		}
	}
	if len(names) > 0 {
		w.byReal[real] = names // another alias still watches this path
		w.mu.Unlock()
		return nil
	}
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
		w.stopStream(s)
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
	kq := w.fallback
	fallbackWatches.Add(-int64(len(w.fbNames)))
	w.fbNames = make(map[string]struct{})
	w.mu.Unlock()
	for _, rs := range streams {
		w.stopStream(rs.stream)
	}
	if kq != nil {
		_ = kq.Close()
	}
	// Callbacks and forwarders hold mu while sending and check closed, so none can send now.
	w.mu.Lock()
	close(w.events)
	close(w.errs)
	w.mu.Unlock()
	return nil
}

// onEvents runs on the FSEvents run-loop thread and must not block. Filesystem
// probes (opsFor) happen before taking mu so a slow stat never stalls Add/Remove.
func (w *fseventsWatcher) onEvents(evs []fsevents.Event) {
	type decoded struct {
		path    string
		dropped bool
		op      fsnotify.Op
	}
	batch := make([]decoded, 0, len(evs))
	for _, e := range evs {
		p := filepath.Clean(e.Path)
		if e.Flags&(fsevents.MustScanSubDirs|fsevents.UserDropped|fsevents.KernelDropped|fsevents.EventIdsWrapped) != 0 {
			batch = append(batch, decoded{path: p, dropped: true})
			continue
		}
		if op := opsFor(e.Flags, p); op != 0 {
			batch = append(batch, decoded{path: p, op: op})
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	for _, d := range batch {
		if d.dropped {
			// Only escalate when the dropped region can contain a watched path;
			// recursive churn under objects/** must not rescan every repo.
			if w.touchesWatchedLocked(d.path) {
				w.overflowLocked()
			}
			continue
		}
		names := w.byReal[d.path]
		if len(names) == 0 {
			parents := w.byReal[filepath.Dir(d.path)]
			if len(parents) == 0 {
				continue
			}
			names = make([]string, len(parents))
			for i, n := range parents {
				names[i] = filepath.Join(n, filepath.Base(d.path))
			}
		}
		for _, name := range names {
			select {
			case w.events <- fsnotify.Event{Name: name, Op: d.op}:
			default:
				w.overflowLocked() // a full buffer means lost events; consumers rescan on overflow
			}
		}
	}
}

// touchesWatchedLocked reports whether p is, contains, or is a direct child of a watched path.
func (w *fseventsWatcher) touchesWatchedLocked(p string) bool {
	if p == "." || p == "" || p == string(filepath.Separator) {
		return true // flags without a path: cannot scope, assume the worst
	}
	if _, ok := w.byReal[p]; ok {
		return true
	}
	if _, ok := w.byReal[filepath.Dir(p)]; ok {
		return true
	}
	for real := range w.byReal {
		if covers(p, real) {
			return true
		}
	}
	return false
}

func (w *fseventsWatcher) overflowLocked() {
	select {
	case w.errs <- fsnotify.ErrEventOverflow:
	default:
	}
}

// opsFor maps FSEvents flags to fsnotify ops. A rename that leaves the path in
// place (lock-file rename onto HEAD/index, tmp -> .idx) is a Create to fsnotify
// consumers, so add Create when the path exists.
func opsFor(f fsevents.EventFlags, path string) fsnotify.Op {
	var op fsnotify.Op
	if f&fsevents.ItemCreated != 0 {
		op |= fsnotify.Create
	}
	if f&fsevents.ItemModified != 0 {
		op |= fsnotify.Write
	}
	if f&fsevents.ItemRenamed != 0 {
		op |= fsnotify.Rename
		if _, err := os.Lstat(path); err == nil {
			op |= fsnotify.Create
		}
	}
	if f&fsevents.ItemRemoved != 0 {
		op |= fsnotify.Remove
	}
	return op
}

func init() {
	m := otel.Meter("stapler-squad/dirwatch")
	fb, _ := m.Int64ObservableGauge("fs.fallback_watches",
		metric.WithDescription("Watched paths served by kqueue because FSEvents was unavailable (stream budget spent or start refused); never re-promoted to FSEvents"))
	lag, lerr := m.Float64Histogram("fs.stream_first_event_lag",
		metric.WithUnit("s"),
		metric.WithDescription("Seconds from a stream starting to its first delivered event; FSEvents can deliver late or drop events with no error"))
	if lerr == nil {
		firstEventLag = lag
	}
	g, err := m.Int64ObservableGauge("fs.streams",
		metric.WithDescription("Live FSEvents streams opened by dirwatch; each locks an OS thread and the kernel refuses new ones near 512 per process"))
	if err != nil {
		return
	}
	_, _ = m.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(g, fsevents.LiveStreams())
		if fb != nil {
			o.ObserveInt64(fb, fallbackWatches.Load())
		}
		return nil
	}, g, fb)
}

var firstEventLag metric.Float64Histogram //nolint:gochecknoglobals // set once in init

func recordFirstEventLag(watcher string, d time.Duration) {
	if firstEventLag != nil {
		firstEventLag.Record(context.Background(), d.Seconds(), metric.WithAttributes(attribute.String("watcher", watcher)))
	}
	if d > 5*time.Second {
		log.Warn("dirwatch: FSEvents stream delivered its first event late", "watcher", watcher, "lag", d)
	}
}
