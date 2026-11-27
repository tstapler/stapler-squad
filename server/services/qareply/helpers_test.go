package qareply

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	ssqlog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
)

// TestMain gives this binary the hermetic setup server/services' own tests run under.
func TestMain(m *testing.M) {
	os.Exit(svctest.Main(m, func() {}))
}

// Several fixtures here are copies of server/services' test helpers, which a
// leaf cannot import.

const (
	auditTestFile = "/cfg/audit/" + auditkit.AuditFileName

	outApplied  = sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED
	outResolved = sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED
	outInvalid  = sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID
	outFlagOff  = sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF
	outForbid   = sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN
	outStale    = sessionv1.ActionOutcome_ACTION_OUTCOME_STALE
)

var testFlagRegistry = sync.OnceValue(func() *featureflags.Registry { return flagsvc.FlagRegistry() })

// gateTestClock is a mutex-guarded manual clock.
type gateTestClock struct {
	mu sync.Mutex
	t  time.Time
}

func newGateTestClock() *gateTestClock {
	return &gateTestClock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
}

func (c *gateTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *gateTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// namedPM gives the test instance a tmux session name.
type namedPM struct {
	*replyPM
	name string
}

func (n namedPM) GetSessionIdentifier() string { return n.name }

// replyEnv is a Service over a real poller, storage, memory audit sink and
// metrics, plus a registered single-select question on a hidden session whose
// pane shows the real captured dialog.
type replyEnv struct {
	t       *testing.T
	svc     *Service
	storage *session.Storage
	poller  *session.ReviewQueuePoller
	mfs     *memfs.MemFS
	sink    *auditkit.AuditSink
	gate    *deliverygate.Gate
	clock   *gateTestClock
	pm      *replyPM
	inst    *instance.Instance
	qid     string

	// The seams behind Ports; tests may replace them at any time.
	peer             func(connect.AnyRequest) string
	controllerActive func(*instance.Instance) bool
	canary           *questionkit.DialogMatchCanary
	mirror           *questionkit.GuidanceHookMirror
}

// ports binds the Service to this env.
func (e *replyEnv) ports() Ports {
	return Ports{
		FindInstance:     e.poller.FindInstance,
		AuditSink:        func() *auditkit.AuditSink { return e.sink },
		Count:            func(label string) { e.gate.Metrics().Add(deliverygate.CounterReply, label) },
		Canary:           func() *questionkit.DialogMatchCanary { return e.canary },
		InstalledMirror:  func() *questionkit.GuidanceHookMirror { return e.mirror },
		Now:              e.clock.Now,
		PeerAddr:         func(r connect.AnyRequest) string { return e.peer(r) },
		ControllerActive: func(i *instance.Instance) bool { return e.controllerActive(i) },
		// Poll timers fire at once; the 5s send timeout never does (nothing sleeps).
		After: func(d time.Duration) <-chan time.Time {
			if d >= time.Second {
				return nil
			}
			ch := make(chan time.Time, 1)
			ch <- time.Time{}
			return ch
		},
	}
}

// newBareEnv is the env without a Service or any session.
func newBareEnv(t *testing.T) *replyEnv {
	t.Helper()
	svctest.WithIsolatedStateDir(t) // flag flips persist to the config file
	mfs := memfs.New()
	sink := auditkit.NewAuditSink(func() (string, error) { return "/cfg", nil }, auditkit.WithAuditFS(mfs),
		auditkit.WithAuditLogger(slog.New(slog.DiscardHandler)))
	t.Cleanup(sink.Close)
	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	t.Cleanup(func() { svctest.StopPollerSessionDrivers(poller) })
	return &replyEnv{
		t: t, storage: svctest.CreateTestStorage(t), poller: poller, mfs: mfs, sink: sink,
		gate: deliverygate.NewGate(), clock: newGateTestClock(),
		peer:             func(connect.AnyRequest) string { return replyLoopbackPeer },
		controllerActive: func(*instance.Instance) bool { return true },
	}
}

// requestCtx is the context the real server chain gives a handler.
func (e *replyEnv) requestCtx(listener string, requiresAuth bool, host string, hdr map[string]string) context.Context {
	cfg := VerdictServiceConfig("0.0.0.0:8543", []string{"onyx.lan"})
	return StampedContext(e.t, &cfg, listener, requiresAuth, host, hdr)
}

func (e *replyEnv) localCtx() context.Context {
	return e.requestCtx(access.ListenerLocal, false, "localhost:8543", nil)
}

type replyTarget struct {
	inst *instance.Instance
	pm   *replyPM
}

// addSession registers a started instance on a recording pane.
func (e *replyEnv) addSession(title string, hidden bool, tweak ...func(*instance.Instance)) *replyTarget {
	e.t.Helper()
	pm := &replyPM{}
	inst := instance.NewStartedInstanceForTest(e.t, title, pm)
	inst.UUID = title + "-uuid"
	inst.Path = e.t.TempDir()
	inst.Status = domain.Active
	inst.Program = "claude"
	inst.Hidden = hidden
	inst.CreatedAt, inst.UpdatedAt = time.Now(), time.Now()
	for _, f := range tweak {
		f(inst)
	}
	svctest.AddInstanceToPoller(e.poller, inst)
	return &replyTarget{inst: inst, pm: pm}
}

// auditLines flushes the sink and returns every line, oldest first.
func (e *replyEnv) auditLines() []auditkit.AuditLine {
	e.t.Helper()
	e.sink.Close()
	return e.readLines()
}

// readLines returns the lines written so far.
func (e *replyEnv) readLines() []auditkit.AuditLine {
	e.t.Helper()
	raw, ok := e.mfs.Get(auditTestFile)
	if !ok || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var out []auditkit.AuditLine
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var al auditkit.AuditLine
		require.NoError(e.t, json.Unmarshal([]byte(l), &al), l)
		out = append(out, al)
	}
	return out
}

func VerdictServiceConfig(listen string, verified []string) middleware.VerdictConfig {
	return middleware.VerdictConfig{
		VerifiedHosts:  func() []string { return verified },
		ListenAddr:     func() string { return listen },
		AllowedOrigins: func() []string { return []string{"https://ui.example"} },
		LocalIPs:       func() []net.IP { return []net.IP{net.ParseIP("192.168.1.50")} },
	}
}

// StampedContext runs the real stamp middleware and returns the context a
// handler behind it would see.
func StampedContext(t *testing.T, cfg *middleware.VerdictConfig, listener string, requiresAuth bool, host string, hdr map[string]string) context.Context {
	t.Helper()
	var got context.Context
	var h http.Handler = http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Context()
	})
	if cfg != nil {
		h = access.WithLocalWriteVerdictConfig(*cfg)(h)
	}
	h = access.WithRequestRecord(listener, requiresAuth)(h)
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Host = host
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	h.ServeHTTP(httptest.NewRecorder(), r)
	require.NotNil(t, got)
	return got
}

// captureInfoLog redirects the log package's slog seam to a buffer and returns a
// function that restores it and returns the captured output.
func captureInfoLog() func() string {
	svctest.SlogDefaultMu.Lock()
	buf := &svctest.SyncLogBuffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	original := ssqlog.SetSlogDefaultForTest(slog.New(h))
	return func() string {
		ssqlog.SetSlogDefaultForTest(original)
		svctest.SlogDefaultMu.Unlock()
		return buf.String()
	}
}

// fakeActionFlags resolves every point true unless listed in off.
type fakeActionFlags struct {
	off       map[string]bool
	answerOff bool
}

func (f fakeActionFlags) Inline(dp string) bool { return !f.off[dp] }
func (f fakeActionFlags) Answer(flagsvc.AnswerRoute) bool {
	return !f.answerOff
}

type recordingAudit struct {
	mu    sync.Mutex
	lines []auditkit.AuditLine
	fail  bool
}

func (a *recordingAudit) Append(l auditkit.AuditLine) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lines = append(a.lines, l)
	return nil
}

func (a *recordingAudit) AppendBounded(_ context.Context, l auditkit.AuditLine) error {
	if a.fail {
		return auditkit.ErrAuditFailed
	}
	return a.Append(l)
}

func answerReq(key string) *sessionv1.InvokeNotificationActionRequest {
	return &sessionv1.InvokeNotificationActionRequest{
		ActionKind: sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION, TargetKeyType: sessionv1.ActionTargetKey_ACTION_TARGET_KEY_QUESTION_ID,
		TargetKey: "q1", SessionId: "s1", IdempotencyKey: key,
		Params: &sessionv1.NotificationActionParams{Answer: "Redis"},
	}
}

// enableInline turns on the inline-action flag for the given scopes and the
// answer stages, in an isolated state dir.
func enableInline(t *testing.T, scopes ...string) {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	cfg := config.LoadConfig()
	for _, sc := range scopes {
		require.NoError(t, cfg.SetFeatureFlagScope(flagsvc.NotificationInlineActionsFlagName, sc, true))
	}
	for _, st := range []string{flagsvc.ScopeStageRender, flagsvc.ScopeStageAnswer} {
		require.NoError(t, cfg.SetFeatureFlagScope(flagsvc.AskUserQuestionStructuredFlagName, st, true))
	}
}
