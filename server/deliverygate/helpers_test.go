package deliverygate

import (
	"context"
	"log/slog"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

const (
	tTaskComplete = sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE
	tError        = sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
	tFailure      = sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE
	tWarning      = sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
	tApproval     = sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED
	tInfo         = sessionv1.NotificationType_NOTIFICATION_TYPE_INFO
)

// fakeClock is a manually advanced Clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// logRecord is one captured slog record.
type logRecord struct {
	Level slog.Level
	Msg   string
	Attrs map[string]any
}

// recHandler captures records for assertion; injected, never the global default.
type recHandler struct {
	mu   *sync.Mutex
	recs *[]logRecord
}

func newRecLogger() (*slog.Logger, func() []logRecord) {
	h := recHandler{mu: &sync.Mutex{}, recs: &[]logRecord{}}
	return slog.New(h), func() []logRecord {
		h.mu.Lock()
		defer h.mu.Unlock()
		return append([]logRecord(nil), (*h.recs)...)
	}
}

func (recHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h recHandler) Handle(_ context.Context, r slog.Record) error {
	rec := logRecord{Level: r.Level, Msg: r.Message, Attrs: map[string]any{}}
	r.Attrs(func(a slog.Attr) bool { rec.Attrs[a.Key] = a.Value.Any(); return true })
	h.mu.Lock()
	*h.recs = append(*h.recs, rec)
	h.mu.Unlock()
	return nil
}
func (h recHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h recHandler) WithGroup(string) slog.Handler      { return h }

func countMsg(recs []logRecord, msg string) int {
	n := 0
	for _, r := range recs {
		if r.Msg == msg {
			n++
		}
	}
	return n
}

// staticFlags returns a FlagLoader over a mutable setting.
type staticFlags struct {
	mu    sync.Mutex
	s     FlagSettings
	err   error
	reads int
}

func (f *staticFlags) load() (FlagSettings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.s, f.err
}

func (f *staticFlags) set(global bool) {
	f.mu.Lock()
	f.s = FlagSettings{Global: global}
	f.mu.Unlock()
}

func (f *staticFlags) setSettings(s FlagSettings) {
	f.mu.Lock()
	f.s = s
	f.mu.Unlock()
}

func (f *staticFlags) readCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

// notif builds an EventNotification for a session slot.
func notif(sessionID string, t sessionv1.NotificationType, md map[string]string) *events.Event {
	return events.NewNotificationEvent(sessionID, sessionID, "n-"+sessionID+"-"+t.String(), int32(t), 2, "t", "m", md)
}

// newTestGate builds a gate with a fake clock, recording logger, flag source
// and a seeded index containing the given entries.
func newTestGate(flagOn bool, entries ...Entry) (*Gate, *fakeClock, func() []logRecord, *staticFlags) {
	clk := newFakeClock()
	lg, recs := newRecLogger()
	flags := &staticFlags{}
	flags.set(flagOn)
	g := NewGate(WithClock(clk.Now), WithLogger(lg), WithFlagLoader(flags.load))
	g.Flags().Reload()
	g.Index().Replace(entries)
	return g, clk, recs, flags
}

var (
	hiddenReview   = Entry{UUID: "u-h1", Title: "review:abc", TmuxName: "ssq_review_abc", Hidden: true, Kind: KindReview}
	hiddenDiagnose = Entry{UUID: "u-h2", Title: "diagnose:abc", TmuxName: "ssq_diagnose_abc", Hidden: true, Kind: KindDiagnose}
	visibleSess    = Entry{UUID: "u-v1", Title: "my-work", TmuxName: "ssq_my_work"}
)

var _ = session.Status(0)
