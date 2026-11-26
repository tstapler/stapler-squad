package adminmisc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/executor"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/tymux"
)

// The golden was recorded from the pre-move code in server/services (origin/main)
// with ADMINMISC_GOLDEN_RECORD=1; this file is identical there except for the package
// clause. Any diff in status codes, headers, bodies, captured log records, global
// log-level state, breaker output or rollout status is a behaviour change.
const goldenPath = "testdata/adminmisc.golden"

type goldenOut struct {
	b strings.Builder
	t *testing.T
}

func (g *goldenOut) line(format string, args ...any) {
	fmt.Fprintf(&g.b, format, args...)
	g.b.WriteString("\n")
}

func (g *goldenOut) record(label string, rec *httptest.ResponseRecorder) {
	keys := make([]string, 0, len(rec.Header()))
	for k := range rec.Header() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	hs := make([]string, 0, len(keys))
	for _, k := range keys {
		hs = append(hs, k+"="+strings.Join(rec.Header()[k], ","))
	}
	g.line("%s -> %d headers[%s] body=%q", label, rec.Code, strings.Join(hs, ";"), rec.Body.String())
}

func (g *goldenOut) do(label string, mux *http.ServeMux, method, path, body string) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	g.record(label, rec)
}

// captureLogs swaps the slog default for a time-free text handler and returns the buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	prev := log.SetSlogDefaultForTest(slog.New(h))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })
	return &buf
}

var clientTimeRe = regexp.MustCompile(`client_time=(\S+)`)

// flushLogs records the captured records; client_time is formatted in the local zone, so it is shown as UTC.
func (g *goldenOut) flushLogs(label string, buf *bytes.Buffer) {
	g.line("%s logs:", label)
	for _, l := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		l = clientTimeRe.ReplaceAllStringFunc(l, func(m string) string {
			ts, err := time.Parse(time.RFC3339Nano, strings.TrimPrefix(m, "client_time="))
			if err != nil {
				return m
			}
			return "client_time=" + ts.UTC().Format(time.RFC3339Nano)
		})
		g.line("  %s", l)
	}
	buf.Reset()
}

type goldenClock struct{ t time.Time }

func (c *goldenClock) Now() time.Time { return c.t }

type goldenExec struct {
	err  error
	hook func()
}

func (e *goldenExec) Run(*exec.Cmd) error {
	if e.hook != nil {
		e.hook()
	}
	return e.err
}
func (e *goldenExec) Output(*exec.Cmd) ([]byte, error)         { return nil, e.Run(nil) }
func (e *goldenExec) CombinedOutput(*exec.Cmd) ([]byte, error) { return nil, e.Run(nil) }

func goldenCmd(args ...string) *exec.Cmd { return &exec.Cmd{Path: args[0], Args: args} }

func circuitBreakerScenario(g *goldenOut) {
	reg := executor.GetGlobalRegistry()
	g.line("# circuit breaker")
	g.line("preexisting-breakers=%d", len(reg.AllBreakers()))
	h := NewCircuitBreakerHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodPatch} {
		g.do("empty "+m, mux, m, "/api/debug/circuit-breakers", "")
	}
	g.do("other path", mux, http.MethodGet, "/api/debug/circuit-breakers/", "")

	clock := &goldenClock{t: time.Date(2020, 1, 2, 3, 4, 5, 123456789, time.FixedZone("x", 3600))}
	cfg := executor.CircuitBreakerConfig{FailureThreshold: 2, RecoveryTimeout: 10 * time.Second, MaxRecoveryTimeout: 25 * time.Second}
	fail := executor.NewCircuitBreakerExecutorWithClock(&goldenExec{err: errors.New("boom")}, cfg, clock)
	ok := executor.NewCircuitBreakerExecutorWithClock(&goldenExec{}, cfg, clock)
	reg.Register("gold-a", fail)
	reg.Register("gold-b", ok)
	defer reg.Unregister("gold-a")
	defer reg.Unregister("gold-b")

	_ = ok.Run(goldenCmd("git", "diff"))
	g.do("one closed", mux, http.MethodGet, "/api/debug/circuit-breakers", "")
	_ = ok.Run(goldenCmd("tmux", "-L", "sock", "capture-pane"))

	_ = fail.Run(goldenCmd("git", "status"))
	g.breakers("closed with one failure", mux)
	_ = fail.Run(goldenCmd("git", "status")) // trips OPEN
	_ = fail.Run(goldenCmd("tmux", "send-keys"))
	clock.t = clock.t.Add(10 * time.Second)
	_ = fail.Run(goldenCmd("git", "status")) // half-open probe fails: OPEN, backoff 1
	clock.t = clock.t.Add(20 * time.Second)
	_ = fail.Run(goldenCmd("git", "status")) // probe fails again: backoff capped at 25s
	_ = fail.Run(goldenCmd("git", "log"))

	g.breakers("mixed states", mux)

	probe := &goldenExec{err: errors.New("down")}
	pe := executor.NewCircuitBreakerExecutorWithClock(probe, cfg, clock)
	reg.Register("gold-c", pe)
	defer reg.Unregister("gold-c")
	_ = pe.Run(goldenCmd("tmux", "kill-session"))
	_ = pe.Run(goldenCmd("tmux", "kill-session"))
	clock.t = clock.t.Add(10 * time.Second)
	probe.err, probe.hook = nil, func() { g.breakers("half-open during probe", mux) }
	_ = pe.Run(goldenCmd("tmux", "kill-session"))
	probe.hook = nil
	g.breakers("after successful probe", mux)
	reg.ResetAll()
	g.breakers("after reset-all", mux)
}

// breakers records status, headers and the body with breakers sorted by key, since map order is random.
func (g *goldenOut) breakers(label string, mux *http.ServeMux) {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/debug/circuit-breakers", nil))
	rec.Body = bytes.NewBufferString(normalizeBreakers(rec.Body.String()))
	g.record(label+" (sorted by key)", rec)
}

// normalizeBreakers sorts the one-object-per-breaker JSON by key so map iteration order cannot matter.
func normalizeBreakers(body string) string {
	const open, closeS = `{"breakers":[`, `]}`
	body = strings.TrimSpace(body)
	inner := strings.TrimSuffix(strings.TrimPrefix(body, open), closeS)
	if inner == "" {
		return body
	}
	parts := strings.Split(inner, `},{"key":`)
	for i := range parts {
		if i > 0 {
			parts[i] = `{"key":` + parts[i]
		}
		if i < len(parts)-1 {
			parts[i] += `}`
		}
	}
	sort.Strings(parts)
	return open + strings.Join(parts, ",") + closeS
}

func clientLogScenario(g *goldenOut, logs *bytes.Buffer) {
	g.line("# client logs")
	clock := &goldenClock{t: time.Date(2021, 5, 6, 7, 8, 9, 0, time.UTC)}
	h := NewClientLogHandler()
	h.now = clock.Now
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	const path = "/api/client-logs"

	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead} {
		g.do("method "+m, mux, m, path, `{"entries":[]}`)
	}
	g.do("empty body", mux, http.MethodPost, path, ``)
	g.do("bad json", mux, http.MethodPost, path, `{`)
	g.do("wrong type", mux, http.MethodPost, path, `{"entries":"x"}`)
	g.do("null entries", mux, http.MethodPost, path, `{"entries":null}`)
	g.do("no entries", mux, http.MethodPost, path, `{"windowId":"w"}`)
	logs.Reset()

	long := strings.Repeat("é", 3000)
	g.do("mixed batch", mux, http.MethodPost, path, `{"windowId":"w1","sessionId":"s1","userAgent":"UA","entries":[`+
		`{"level":"warn","message":"a","url":"/p?token=secret#frag","timeMs":1600000000123},`+
		`{"level":"error","message":"b","stack":"line1\nline2","url":"https://x/y#z"},`+
		`{"level":"debug","message":"dropped"},{"level":"info","message":"dropped"},{"level":"WARN","message":"case"},{"level":"","message":"empty"},`+
		`{"level":"error","message":"","timeMs":-5}]}`)
	g.flushLogs("mixed batch", logs)

	g.do("long fields", mux, http.MethodPost, path, `{"windowId":"`+strings.Repeat("w", 100)+`","sessionId":"`+strings.Repeat("s", 200)+
		`","userAgent":"`+strings.Repeat("u", 300)+`","entries":[{"level":"error","message":"`+long+`","stack":"`+long+`","url":"/`+strings.Repeat("p", 600)+`"}]}`)
	g.flushLogs("long fields", logs)

	g.do("multibyte window", mux, http.MethodPost, path, `{"windowId":"`+strings.Repeat("é", 40)+`","entries":[{"level":"warn","message":"m"}]}`)
	g.flushLogs("multibyte window", logs)

	g.line("stripQuery: %q %q %q %q %q", stripQuery("/a?b"), stripQuery("/a#b"), stripQuery("?x"), stripQuery("#x"), stripQuery("/a#b?c"))
	g.line("truncate: %q %q %q %q %q", truncate("abc", 3), truncate("abc", 2), truncate("aé", 2), truncate("é", 1), truncate("", 0))

	oversize := `{"windowId":"` + strings.Repeat("x", maxClientLogBodyBytes) + `","entries":[]}`
	g.do("oversized body", mux, http.MethodPost, path, oversize)
	atCap := `{"windowId":"` + strings.Repeat("x", maxClientLogBodyBytes-len(`{"windowId":"","entries":[]}`)) + `","entries":[]}`
	g.line("at-cap len=%d cap=%d", len(atCap), maxClientLogBodyBytes)
	g.do("body exactly at cap", mux, http.MethodPost, path, atCap)
	g.do("body one over cap", mux, http.MethodPost, path, atCap[:len(atCap)-2]+` ,"z":1}`)

	g.line("constants: body=%d batch=%d field=%d budget=%d interval=%s", maxClientLogBodyBytes, maxClientLogBatch, maxClientLogFieldLen, clientLogBudgetPerMin, clientLogBudgetInterval)

	// Budget: window of 600 shared across batches, 100 per batch cap, only warn/error spend it.
	logs.Reset()
	h2 := NewClientLogHandler()
	h2.now = clock.Now
	mux2 := http.NewServeMux()
	h2.RegisterRoutes(mux2)
	batch := func(n int, level string) string {
		es := make([]string, n)
		for i := range es {
			es[i] = `{"level":"` + level + `","message":"m"}`
		}
		return `{"windowId":"b","entries":[` + strings.Join(es, ",") + `]}`
	}
	g.do("oversized batch capped at 100", mux2, http.MethodPost, path, batch(150, "warn"))
	logs.Reset()
	for i := 2; i <= 6; i++ {
		g.do(fmt.Sprintf("batch %d", i), mux2, http.MethodPost, path, batch(100, "warn"))
	}
	logs.Reset()
	g.do("budget exhausted", mux2, http.MethodPost, path, batch(1, "warn"))
	g.do("debug entries cost nothing when exhausted", mux2, http.MethodPost, path, batch(50, "debug"))
	g.do("empty batch when exhausted", mux2, http.MethodPost, path, batch(0, "warn"))
	g.line("logs after exhaustion=%d bytes", logs.Len())
	clock.t = clock.t.Add(clientLogBudgetInterval - time.Nanosecond)
	g.do("one ns before reset", mux2, http.MethodPost, path, batch(1, "error"))
	clock.t = clock.t.Add(time.Nanosecond)
	g.do("exactly at reset", mux2, http.MethodPost, path, batch(100, "error"))
	logs.Reset()
	for i := 0; i < 4; i++ {
		g.do(fmt.Sprintf("fill %d", i), mux2, http.MethodPost, path, batch(100, "error"))
	}
	g.do("50 of remaining 100", mux2, http.MethodPost, path, batch(50, "warn"))
	g.do("51 would exceed: whole batch rejected", mux2, http.MethodPost, path, batch(51, "warn"))
	g.do("50 fits exactly", mux2, http.MethodPost, path, batch(50, "warn"))
	g.do("one more rejected", mux2, http.MethodPost, path, batch(1, "warn"))
}

func logLevelScenario(t *testing.T, g *goldenOut, logs *bytes.Buffer) {
	g.line("# log level")
	logs.Reset()
	origLevel := log.GetRuntimeLevel()
	origPkgs := log.GetPackageLevels()
	t.Cleanup(func() {
		log.SetRuntimeLevel(origLevel)
		log.ClearAllPackageLevels()
		for p, l := range origPkgs {
			log.SetPackageLevel(p, l)
		}
	})
	log.ClearAllPackageLevels()
	log.SetRuntimeLevel(log.INFO)

	h := NewLogLevelHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	const base = "/api/debug/log-level"
	state := func(label string) {
		pk := log.GetPackageLevels()
		names := make([]string, 0, len(pk))
		for n := range pk {
			names = append(names, n+"="+pk[n].String())
		}
		sort.Strings(names)
		g.line("state[%s] runtime=%s packages=%v", label, log.GetRuntimeLevel(), names)
	}

	state("start")
	g.do("get", mux, http.MethodGet, base, "")
	for _, m := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		g.do("method "+m, mux, m, base, "")
	}
	g.do("get head", mux, http.MethodHead, base, "")
	for i, body := range []string{`{"level":"debug"}`, `{"level":"WARNING"}`, `{"level":"warn"}`, `{"level":"nonsense"}`, `{"level":""}`, `{}`, `{`, ``, `{"level":"error"}`, `{"level":"fatal"}`} {
		g.do(fmt.Sprintf("set %d %s", i, body), mux, http.MethodPost, base, body)
		state(fmt.Sprintf("after set %d", i))
	}
	g.flushLogs("global", logs)
	log.SetRuntimeLevel(log.INFO)

	pk := base + "/packages"
	g.do("packages empty", mux, http.MethodGet, pk, "")
	g.do("packages PUT", mux, http.MethodPut, pk, "")
	bodies := []string{
		`{"package":"session/tmux","level":"DEBUG"}`,
		`{"package":"session/tmux","level":"error"}`,
		`{"package":"session/tmux/sub","level":"warn"}`,
		`{"package":"a","level":"INFO"}`,
		`{"package":"server/auth","level":"ERROR"}`,
		`{"package":"server/auth","level":"fatal"}`,
		`{"package":"server/auth","level":"WARNING"}`,
		`{"package":"server/auth","level":"info"}`,
		`{"package":"server/auth/passkey","level":"ERROR"}`,
		`{"package":"server/auth/passkey","level":"debug"}`,
		`{"package":"server/authx","level":"ERROR"}`,
		`{"package":"server","level":"ERROR"}`,
		`{"package":"","level":"INFO"}`,
		`{"level":"INFO"}`,
		`{"package":"x","level":"bogus"}`,
		`{"package":"x"}`,
		`{"package":"x","level":""}`,
		`{`,
		``,
		`[]`,
		`{"package":"` + strings.Repeat("p", 5000) + `","level":"INFO"}`,
	}
	for i, body := range bodies {
		show := body
		if len(show) > 60 {
			show = show[:60] + "..."
		}
		g.do(fmt.Sprintf("pkg set %d %s", i, show), mux, http.MethodPost, pk, body)
		state(fmt.Sprintf("after pkg set %d", i))
	}
	g.do("packages sorted", mux, http.MethodGet, pk, "")
	g.flushLogs("package set", logs)

	for i, body := range []string{`{"package":"a"}`, `{"package":"a"}`, `{"package":"nope"}`, `{"package":""}`, `{}`, `{`, ``, `{"package":"` + strings.Repeat("p", 5000) + `"}`, `{"package":"session/tmux","level":"ignored"}`} {
		show := body
		if len(show) > 60 {
			show = show[:60] + "..."
		}
		g.do(fmt.Sprintf("pkg clear %d %s", i, show), mux, http.MethodDelete, pk, body)
		state(fmt.Sprintf("after pkg clear %d", i))
	}
	g.do("packages after clear", mux, http.MethodGet, pk, "")
	g.flushLogs("package clear", logs)
	g.line("constants: bodycap=%d audit=%v levels=%d", maxPackageLevelBodyBytes, auditSensitivePackages, len(validLogLevelNames))
	for _, p := range []string{"server/auth", "server/auth/x", "server/authx", "server", "", "x/server/auth"} {
		g.line("isAuditSensitive(%q)=%v", p, isAuditSensitive(p))
	}
}

// wireHex is the deterministic proto wire encoding (prototext output varies per binary).
func wireHex(m proto.Message) string {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
	if err != nil {
		return "marshal-error:" + err.Error()
	}
	return fmt.Sprintf("%x", b)
}

func canonicalStatus(s *sessionv1.TymuxRolloutStatus) string {
	if s == nil {
		return "<nil>"
	}
	c := proto.Clone(s).(*sessionv1.TymuxRolloutStatus)
	ovs := append([]*sessionv1.TymuxSessionOverrideEntry(nil), c.SessionOverrides...)
	sort.Slice(ovs, func(i, j int) bool { return ovs[i].SessionName < ovs[j].SessionName })
	c.SessionOverrides = ovs
	if c.RollbackRehearsalCompletedAt != nil {
		c.RollbackRehearsalCompletedAt = nil
		return "rehearsal=<set> " + wireHex(c)
	}
	return "rehearsal=<unset> " + wireHex(c)
}

func tymuxScenario(g *goldenOut) {
	g.line("# tymux rollout")
	ctx := context.Background()
	svctest.WithIsolatedStateDir(g.t)
	s := NewTymuxRolloutService()
	info := tymux.DaemonVersionInfo{}
	s.detectDaemon = func(context.Context, tymux.DaemonConfig) tymux.DaemonVersionInfo { return info }
	var gotAllow bool
	var replaceErr error
	s.replaceDaemon = func(_ context.Context, _ tymux.DaemonConfig, allow bool) (tymux.DaemonVersionInfo, error) {
		gotAllow = allow
		return tymux.DaemonVersionInfo{Running: true, PID: 7, RunningVersion: "2.0.0", InstalledVersion: "2.0.0", Direction: tymux.VersionSame}, replaceErr
	}
	get := func(label string) {
		resp, err := s.GetTymuxRolloutStatus(ctx, connect.NewRequest(&sessionv1.GetTymuxRolloutStatusRequest{}))
		if err != nil {
			g.line("get[%s] err=%v code=%v", label, err, connect.CodeOf(err))
			return
		}
		g.line("get[%s] %s", label, canonicalStatus(resp.Msg))
	}

	get("initial")
	for _, d := range []tymux.VersionDirection{tymux.VersionSame, tymux.VersionUpgrade, tymux.VersionDowngrade, tymux.VersionDirection(99)} {
		info = tymux.DaemonVersionInfo{Running: true, PID: 1 << 20, RunningVersion: "1.0.0", InstalledVersion: "1.1.0", Direction: d}
		get(fmt.Sprintf("daemon direction %v", d))
	}
	info = tymux.DaemonVersionInfo{PID: 1 << 31}
	get("pid overflow")
	info = tymux.DaemonVersionInfo{}

	tr, fa := true, false
	setGlobal := func(label string, v *bool) {
		resp, err := s.SetTymuxGlobalOverride(ctx, connect.NewRequest(&sessionv1.SetTymuxGlobalOverrideRequest{ForceTymux: v}))
		if err != nil {
			g.line("setGlobal[%s] err=%v code=%v", label, err, connect.CodeOf(err))
			return
		}
		g.line("setGlobal[%s] %s", label, canonicalStatus(resp.Msg))
	}
	setGlobal("true", &tr)
	setGlobal("false", &fa)
	get("after global false")
	setGlobal("clear", nil)

	setSession := func(label, name string, v *bool) {
		resp, err := s.SetTymuxSessionOverride(ctx, connect.NewRequest(&sessionv1.SetTymuxSessionOverrideRequest{SessionName: name, ForceTymux: v}))
		if err != nil {
			g.line("setSession[%s] err=%v code=%v", label, err, connect.CodeOf(err))
			return
		}
		g.line("setSession[%s] %s", label, canonicalStatus(resp.Msg))
	}
	setSession("b true", "b", &tr)
	setSession("a false", "a", &fa)
	setSession("c true", "c", &tr)
	setSession("b clear", "b", nil)
	setSession("missing clear", "zzz", nil)
	setSession("empty name", "", &tr)
	setSession("empty name clear", "", nil)
	get("after sessions")

	resp, err := s.CompleteTymuxRollbackRehearsal(ctx, connect.NewRequest(&sessionv1.CompleteTymuxRollbackRehearsalRequest{}))
	require.NoError(g.t, err)
	g.line("rehearsal %s", canonicalStatus(resp.Msg))
	get("after rehearsal")

	errs := []error{nil, errors.New("plain"), tymux.ErrLiveSessionsWouldBeInterrupted, tymux.ErrHandoffRefused, tymux.ErrHandoffUnavailable, tymux.ErrHandoffVerifyFailed,
		fmt.Errorf("wrapped: %w", tymux.ErrHandoffRefused), fmt.Errorf("wrapped: %w", tymux.ErrHandoffVerifyFailed)}
	for _, allow := range []bool{false, true} {
		for _, e := range errs {
			replaceErr, gotAllow = e, !allow
			resp, err := s.ReplaceTymuxd(ctx, connect.NewRequest(&sessionv1.ReplaceTymuxdRequest{AllowInterruptLiveSessions: allow}))
			if err != nil {
				g.line("replace[allow=%v err=%v] -> code=%v msg=%q gotAllow=%v", allow, e, connect.CodeOf(err), err.Error(), gotAllow)
				continue
			}
			g.line("replace[allow=%v err=%v] -> %s gotAllow=%v", allow, e, canonicalStatus(resp.Msg), gotAllow)
		}
	}

	// Over HTTP through the generated Connect handler. At most one override keeps the map-order-free JSON deterministic.
	setSession("a clear", "a", nil)
	setSession("c clear", "c", nil)
	mux := http.NewServeMux()
	mux.Handle(sessionv1connect.NewTymuxRolloutServiceHandler(s))
	for _, tc := range []struct{ label, proc, body string }{
		{"http get json", sessionv1connect.TymuxRolloutServiceGetTymuxRolloutStatusProcedure, `{}`},
		{"http set global json", sessionv1connect.TymuxRolloutServiceSetTymuxGlobalOverrideProcedure, `{"forceTymux":true}`},
		{"http set global clear json", sessionv1connect.TymuxRolloutServiceSetTymuxGlobalOverrideProcedure, `{}`},
		{"http replace refused json", sessionv1connect.TymuxRolloutServiceReplaceTymuxdProcedure, `{}`},
		{"http bad body json", sessionv1connect.TymuxRolloutServiceSetTymuxSessionOverrideProcedure, `{`},
	} {
		replaceErr = tymux.ErrLiveSessionsWouldBeInterrupted
		req := httptest.NewRequest(http.MethodPost, tc.proc, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		body := compactJSON(rec.Body.String())
		g.line("%s -> %d content-type=%s body=%s", tc.label, rec.Code, rec.Header().Get("Content-Type"), scrubTimes(body))
	}
	g.do("http GET not allowed", mux, http.MethodGet, sessionv1connect.TymuxRolloutServiceGetTymuxRolloutStatusProcedure, "")
}

// compactJSON drops insignificant whitespace: protojson inserts a per-binary random space.
func compactJSON(body string) string {
	var out bytes.Buffer
	if err := json.Compact(&out, []byte(body)); err != nil {
		return body
	}
	return out.String()
}

// scrubTimes hides the one wall-clock value (the rehearsal timestamp) in a JSON body.
func scrubTimes(body string) string {
	// protojson's error text carries a per-binary random space; keep the code, mask the wording.
	if i := strings.Index(body, `"message":"unmarshal message:`); i >= 0 {
		return body[:i] + `"message":"<unmarshal error>"}`
	}
	if i := strings.Index(body, `"rollbackRehearsalCompletedAt":"`); i >= 0 {
		j := strings.Index(body[i+32:], `"`)
		return body[:i+32] + "<ts>" + body[i+32+j:]
	}
	return body
}

func TestAdminMiscGolden(t *testing.T) {
	g := &goldenOut{t: t}
	logs := captureLogs(t)
	circuitBreakerScenario(g)
	clientLogScenario(g, logs)
	logLevelScenario(t, g, logs)
	tymuxScenario(g)

	got := g.b.String()
	if os.Getenv("ADMINMISC_GOLDEN_RECORD") == "1" {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
