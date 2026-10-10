package deliverygate

import (
	"container/list"
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

const (
	logWindow      = 60 * time.Second
	logLimiterKeys = 4096
)

// Clock is the injected time source (tests use a fake; production time.Now).
type Clock func() time.Time

// repoLogHandler routes slog records to the repo's logging package so a Gate
// built without an injected logger never touches the global slog default.
type repoLogHandler struct{ attrs []slog.Attr }

func (repoLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h repoLogHandler) Handle(_ context.Context, r slog.Record) error {
	args := make([]any, 0, 2*(len(h.attrs)+r.NumAttrs()))
	for _, a := range h.attrs {
		args = append(args, a.Key, a.Value.Any())
	}
	r.Attrs(func(a slog.Attr) bool {
		args = append(args, a.Key, a.Value.Any())
		return true
	})
	switch {
	case r.Level >= slog.LevelError:
		log.Error(r.Message, args...)
	case r.Level >= slog.LevelWarn:
		log.Warn(r.Message, args...)
	default:
		log.Info(r.Message, args...)
	}
	return nil
}

func (h repoLogHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return repoLogHandler{attrs: append(append([]slog.Attr(nil), h.attrs...), a...)}
}

func (h repoLogHandler) WithGroup(string) slog.Handler { return h }

type limiterKey struct {
	event, session, typ, reason string
}

type limiterEntry struct {
	key        limiterKey
	lastLogged time.Time
	suppressed uint64
}

// rateLimitedLogger emits one line per (event, session, type, reason) per
// logWindow, carrying suppressed_since_last. The key map is LRU-bounded.
type rateLimitedLogger struct {
	logger *slog.Logger
	now    Clock

	mu    sync.Mutex // guards only the limiter map; never held while logging
	order *list.List
	items map[limiterKey]*list.Element
}

func newRateLimitedLogger(l *slog.Logger, now Clock) *rateLimitedLogger {
	return &rateLimitedLogger{logger: l, now: now, order: list.New(), items: map[limiterKey]*list.Element{}}
}

// permit reports whether a line for k may be emitted now (and the count of
// lines suppressed since the last one). It allocates nothing, so callers can
// build log attributes only after it returns true.
func (r *rateLimitedLogger) permit(k limiterKey) (since uint64, ok bool) {
	return r.permitEvery(logWindow, k)
}

func (r *rateLimitedLogger) permitEvery(window time.Duration, k limiterKey) (uint64, bool) {
	n := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if el, ok := r.items[k]; ok {
		e := el.Value.(*limiterEntry)
		r.order.MoveToFront(el)
		if n.Sub(e.lastLogged) < window {
			e.suppressed++
			return 0, false
		}
		since := e.suppressed
		e.suppressed = 0
		e.lastLogged = n
		return since, true
	}
	r.items[k] = r.order.PushFront(&limiterEntry{key: k, lastLogged: n})
	if r.order.Len() > logLimiterKeys {
		oldest := r.order.Back()
		r.order.Remove(oldest)
		delete(r.items, oldest.Value.(*limiterEntry).key)
	}
	return 0, true
}

// log emits msg at level unless the same key already logged inside logWindow.
// Prefer permit on hot paths: log's variadic attrs are built before the check.
func (r *rateLimitedLogger) log(level slog.Level, msg string, k limiterKey, attrs ...any) {
	r.logEvery(logWindow, level, msg, k, attrs...)
}

// logEvery is log with an explicit window (e.g. one WARN per hour).
func (r *rateLimitedLogger) logEvery(window time.Duration, level slog.Level, msg string, k limiterKey, attrs ...any) {
	since, ok := r.permitEvery(window, k)
	if !ok {
		return
	}
	attrs = append(attrs, "suppressed_since_last", since)
	r.logger.Log(context.Background(), level, msg, attrs...)
}
