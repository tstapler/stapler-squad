package access

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// goldenPath holds every allow/deny decision of the package over an input
// matrix. It was recorded from the pre-move code in server/services (origin/main
// at a73494b4a), so a pass means the move changed no decision byte for byte.
// Regenerate (only for an intended behaviour change) with ACCESS_GOLDEN_OUT=<path>.
const goldenPath = "testdata/decisions.golden.gz"

func TestDecisions_ShouldMatchTheRecordedGolden_WhenTheMatrixIsReplayed(t *testing.T) {
	got := dumpDecisions(t)
	if out := os.Getenv("ACCESS_GOLDEN_OUT"); out != "" {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, err := zw.Write([]byte(got))
		require.NoError(t, err)
		require.NoError(t, zw.Close())
		require.NoError(t, os.WriteFile(out, buf.Bytes(), 0o644))
		return
	}
	f, err := os.Open(goldenPath)
	require.NoError(t, err)
	defer f.Close()
	zr, err := gzip.NewReader(f)
	require.NoError(t, err)
	want, err := io.ReadAll(zr)
	require.NoError(t, err)
	if string(want) != got {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("decision drift at line %d:\n got: %s\nwant: %s", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("decision count drift: got %d lines, want %d", len(gl), len(wl))
	}
}

type dumper struct {
	b strings.Builder
}

func (d *dumper) p(format string, args ...any) { fmt.Fprintf(&d.b, format+"\n", args...) }

func dumpDecisions(t *testing.T) string {
	t.Helper()
	d := &dumper{}
	d.verdicts(t)
	d.requestRecords()
	d.terminalAccess(t)
	d.paneWriter()
	d.nudgeGuard()
	d.humanActivity(t)
	d.inputMetrics()
	d.tmuxSubprocesses(t)
	return d.b.String()
}

type dimsPM struct {
	instance.ProcessManager
	cols, rows int
	err        error
}

func (p dimsPM) GetSessionIdentifier() string     { return "ssq_dims" }
func (p dimsPM) GetPanePID() (int32, error)       { return 0, errors.New("no pane") }
func (p dimsPM) HasSession() bool                 { return true }
func (p dimsPM) IsAlive() bool                    { return true }
func (p dimsPM) HasMeaningfulContent(string) bool { return false }
func (p dimsPM) GetPaneDimensions() (int, int, error) {
	return p.cols, p.rows, p.err
}

// tmuxSubprocesses replays the send-keys and resize helpers against a fake
// tmux binary that logs its argv, so the exact command lines are pinned.
func (d *dumper) tmuxSubprocesses(t *testing.T) {
	t.Helper()
	d.p("# tmux subprocesses")
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")
	script := filepath.Join(dir, "fake-tmux")
	body := "#!/bin/sh\necho \"$@\" >> \"$FAKE_TMUX_LOG\"\n[ -n \"$FAKE_TMUX_FAIL\" ] && exit 1\nexit 0\n"
	require.NoError(t, os.WriteFile(script, []byte(body), 0o755))
	t.Setenv("TMUX_BIN", script)
	t.Setenv("FAKE_TMUX_LOG", logPath)
	t.Setenv("STAPLER_SQUAD_TMUX_SOCKET", "")
	drain := func() string {
		raw, err := os.ReadFile(logPath)
		if os.IsNotExist(err) {
			return "<no tmux call>"
		}
		require.NoError(t, err)
		require.NoError(t, os.Remove(logPath))
		return isolatedSocketPID.ReplaceAllString(strings.TrimRight(string(raw), "\n"), "test-isolated-N")
	}
	for _, fail := range []string{"", "1"} {
		t.Setenv("FAKE_TMUX_FAIL", fail)
		for _, sock := range []string{"", "ssq-golden-sock"} {
			for _, data := range [][]byte{nil, []byte("hi"), {0x00, 0xff, 0x1b}} {
				err := sendInputToTmux(sock, "ssq_a", data)
				d.p("sendInputToTmux fail=%q sock=%q data=%x => err=%v argv=%q", fail, sock, data, err, drain())
				err = realTmuxInputSender{}.SendInput(sock, "ssq_a", data)
				d.p("realSender.SendInput fail=%q sock=%q data=%x => err=%v argv=%q", fail, sock, data, err, drain())
			}
		}
	}
	for _, dims := range []struct {
		name       string
		cols, rows int
		err        error
	}{{"match", 100, 30, nil}, {"mismatch", 80, 24, nil}, {"verify-error", 0, 0, errors.New("no pane")}} {
		inst := instance.NewStartedInstanceForTest(t, "golden-resize", dimsPM{cols: dims.cols, rows: dims.rows, err: dims.err})
		for _, fail := range []string{"", "1"} {
			t.Setenv("FAKE_TMUX_FAIL", fail)
			target := ExternalPaneTarget{Instance: inst, Snap: &domain.InstanceSnapshot{TmuxServerSocket: "ssq-golden-sock"}, TmuxSession: "ext_1", SessionID: "sid"}
			realTmuxInputSender{}.Resize(target, 100, 30)
			d.p("resize %s fail=%q => argv=%q", dims.name, fail, drain())
		}
	}
}

func goldenVerdictConfigs() map[string]middleware.VerdictConfig {
	mk := func(listen string, verified []string) middleware.VerdictConfig {
		return middleware.VerdictConfig{
			VerifiedHosts:  func() []string { return verified },
			ListenAddr:     func() string { return listen },
			AllowedOrigins: func() []string { return []string{"https://ui.example"} },
			LocalIPs:       func() []net.IP { return []net.IP{net.ParseIP("192.168.1.50")} },
		}
	}
	return map[string]middleware.VerdictConfig{
		"wildcard-verified": mk("0.0.0.0:8543", []string{"onyx.lan"}),
		"localhost-bind":    mk("localhost:8543", nil),
		"loopback-9000":     mk("127.0.0.1:9000", []string{"onyx.lan", "host.example"}),
	}
}

func stamp(cfg *middleware.VerdictConfig, listener string, requiresAuth bool, host, origin string, proxied bool) context.Context {
	var got context.Context
	var h http.Handler = http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.Context() })
	if cfg != nil {
		h = WithLocalWriteVerdictConfig(*cfg)(h)
	}
	h = WithRequestRecord(listener, requiresAuth)(h)
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if proxied {
		r.Header.Set("X-Forwarded-For", "203.0.113.7")
	}
	h.ServeHTTP(httptest.NewRecorder(), r)
	return got
}

func (d *dumper) verdicts(t *testing.T) {
	t.Helper()
	cfgs := goldenVerdictConfigs()
	d.p("# EvaluateLocalWrite")
	d.p("no-record %+v", mustVerdict(EvaluateLocalWrite(context.Background(), "127.0.0.1:1", LocalWriteRebinding)))
	rec := WithRequestRecord("local", false)
	var noCfg context.Context
	rec(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { noCfg = r.Context() })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	d.p("no-config %+v", mustVerdict(EvaluateLocalWrite(noCfg, "127.0.0.1:1", LocalWriteLocalCaller)))
	names := []string{"wildcard-verified", "localhost-bind", "loopback-9000"}
	hosts := []string{"localhost:8543", "127.0.0.1:8543", "[::1]:8543", "onyx.lan:8543", "host.example:9000", "evil.example:8543", "203.0.113.9:8543", "192.168.1.50:8543", ""}
	origins := []string{"", "https://ui.example", "https://evil.example"}
	peers := []string{"127.0.0.1:50000", "[::1]:50000", "192.168.1.20:50000", "garbage"}
	chains := []struct {
		listener string
		auth     bool
	}{{"local", false}, {"local", true}, {"remote", true}}
	profiles := []LocalWriteProfile{LocalWriteRebinding, LocalWriteLocalCaller, LocalWriteProfile(7)}
	for _, name := range names {
		cfg := cfgs[name]
		for _, c := range chains {
			for _, host := range hosts {
				for _, origin := range origins {
					for _, proxied := range []bool{false, true} {
						ctx := stamp(&cfg, c.listener, c.auth, host, origin, proxied)
						for _, peer := range peers {
							for _, prof := range profiles {
								out, err := EvaluateLocalWrite(ctx, peer, prof)
								d.p("%s|%s|auth=%t|host=%q|origin=%q|proxied=%t|peer=%q|profile=%d => %+v allowed=%t err=%s",
									name, c.listener, c.auth, host, origin, proxied, peer, prof, out, out.Allowed(), errString(err))
							}
						}
					}
				}
			}
		}
	}
}

func mustVerdict(out LocalWriteRefusal, err error) string {
	return fmt.Sprintf("%+v err=%s", out, errString(err))
}

func errString(err error) string {
	if err == nil {
		return "nil"
	}
	return fmt.Sprintf("%s:%q", connect.CodeOf(err), err.Error())
}

func (d *dumper) requestRecords() {
	d.p("# RequestRecord")
	_, ok := RequestRecordFrom(context.Background())
	d.p("unstamped ok=%t", ok)
	for _, l := range []string{ListenerLocal, ListenerRemote, "other"} {
		for _, auth := range []bool{false, true} {
			for _, origin := range []string{"", "https://ui.example"} {
				for _, hdr := range []string{"", "X-Forwarded-For", "Forwarded", "X-Real-Ip", "Via"} {
					var got RequestRecord
					var gotOK bool
					h := WithRequestRecord(l, auth)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got, gotOK = RequestRecordFrom(r.Context()) }))
					r := httptest.NewRequest(http.MethodGet, "/", nil)
					r.Host = "onyx.lan:8444"
					if origin != "" {
						r.Header.Set("Origin", origin)
					}
					if hdr != "" {
						r.Header.Set(hdr, "x")
					}
					h.ServeHTTP(httptest.NewRecorder(), r)
					d.p("%s|auth=%t|origin=%q|hdr=%q => %+v ok=%t", l, auth, origin, hdr, got, gotOK)
				}
			}
		}
	}
	d.p("consts %q %q %q %q", ListenerLocal, ListenerRemote, AuthModeNone, AuthModeRequired)
}

type goldenFlag struct{ on bool }

func (f goldenFlag) GuardsEnabled() bool { return f.on }

func goldenInstance(t *testing.T, hidden bool) *instance.Instance {
	t.Helper()
	inst, err := instance.NewInstance(instance.InstanceOptions{Title: "golden", Path: t.TempDir(), Program: "claude", Hidden: hidden})
	require.NoError(t, err)
	return inst
}

func (d *dumper) terminalAccess(t *testing.T) {
	t.Helper()
	d.p("# TerminalAccess")
	var zero TerminalAccess
	d.p("zero=%d readonly=%d readwrite=%d", zero, TerminalReadOnly, TerminalReadWrite)
	d.p("AccessFor(nil)=%d hidden=%d visible=%d", AccessFor(nil), AccessFor(goldenInstance(t, true)), AccessFor(goldenInstance(t, false)))
	var nilFlag *UnaryGuardsFlag
	off := &UnaryGuardsFlag{}
	off.SetEnabled(false)
	on := &UnaryGuardsFlag{}
	on.SetEnabled(true)
	flags := []struct {
		name string
		flag GuardsFlag
	}{
		{"nil-interface", nil}, {"typed-nil", nilFlag}, {"zero", &UnaryGuardsFlag{}}, {"off", off}, {"on", on},
		{"func-true", goldenFlag{on: true}}, {"func-false", goldenFlag{on: false}},
	}
	insts := []struct {
		name string
		inst *instance.Instance
	}{{"nil", nil}, {"hidden", goldenInstance(t, true)}, {"visible", goldenInstance(t, false)}}
	for _, in := range insts {
		for _, f := range flags {
			d.p("AccessForUnary(%s,%s)=%d", in.name, f.name, AccessForUnary(in.inst, f.flag))
		}
	}
	d.p("GuardsEnabled nil=%t zero=%t off=%t on=%t", nilFlag.GuardsEnabled(), (&UnaryGuardsFlag{}).GuardsEnabled(), off.GuardsEnabled(), on.GuardsEnabled())
	for _, a := range []TerminalAccess{TerminalReadOnly, TerminalReadWrite, TerminalAccess(9)} {
		w, err := a.Writer(nil)
		d.p("Writer(%d) nil=%t err=%v readOnly=%t", a, w == nil, err, errors.Is(err, ErrReadOnly))
	}
}

type goldenSender struct {
	mu      sync.Mutex
	inputs  []string
	resizes [][2]int
	err     error
}

func (s *goldenSender) SendInput(sock, sess string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputs = append(s.inputs, sock+"/"+sess+"/"+string(data))
	return s.err
}

func (s *goldenSender) Resize(_ ExternalPaneTarget, cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resizes = append(s.resizes, [2]int{cols, rows})
}

type goldenControlMode struct {
	err error
}

func (g goldenControlMode) SendInputViaControlMode(context.Context, []byte) error { return g.err }

type goldenGatedControlMode struct {
	goldenControlMode
	reachable bool
}

func (g goldenGatedControlMode) CanFallBackToTmuxSendKeys() bool { return g.reachable }

func (d *dumper) paneWriter() {
	d.p("# paneWriter")
	cmErr := errors.New("queue full")
	senderErr := errors.New("tmux gone")
	modes := []struct {
		name string
		cm   controlModeInput
	}{
		{"cm-ok", goldenControlMode{}},
		{"cm-err", goldenControlMode{err: cmErr}},
		{"gated-reachable-err", goldenGatedControlMode{goldenControlMode{err: cmErr}, true}},
		{"gated-unreachable-err", goldenGatedControlMode{goldenControlMode{err: cmErr}, false}},
		{"gated-unreachable-ok", goldenGatedControlMode{goldenControlMode{}, false}},
	}
	for _, m := range modes {
		for _, sErr := range []error{nil, senderErr} {
			s := &goldenSender{err: sErr}
			clock := time.Unix(1_800_000_000, 0)
			w := &paneWriter{sender: s, activity: NewHumanActivity(func() time.Time { return clock })}
			err := w.SendInput(context.Background(), PaneInput{ControlMode: m.cm, TmuxSocket: "sock", TmuxSession: "ssq_a", LogPrefix: "[golden]", Data: []byte("abc")})
			d.p("%s|senderErr=%t => err=%v fallbackCalls=%v activity=%t", m.name, sErr != nil, err, s.inputs, w.activity.Within("ssq_a", humanActiveWindow))
		}
	}
	w, _ := TerminalReadWrite.Writer(nil)
	d.p("default sender is real=%t", w.Sender() != nil)
	s := &goldenSender{}
	w2, _ := TerminalReadWrite.Writer(s)
	d.p("custom sender kept=%t", w2.Sender() == TmuxInputSender(s))
}

func (d *dumper) nudgeGuard() {
	d.p("# SessionNudgeGuard")
	d.p("ReasonSetSignature %q %q %q %q", ReasonSetSignature(), ReasonSetSignature("b", "a", "b"), ReasonSetSignature("ci", "conflict", "ci"), ReasonSetSignature(""))
	for _, pr := range []githubpkg.LinkKey{"", "o/r#1"} {
		d.p("GuardSignature(%q)=%q", pr, GuardSignature(pr, "ci,review"))
	}
	now := time.Unix(1_800_000_000, 0)
	g := &SessionNudgeGuard{now: func() time.Time { return now }}
	step := func(label string, id, sig string, success bool, advance time.Duration) {
		now = now.Add(advance)
		release, out := g.TryBegin(id, sig)
		d.p("%s: TryBegin(%s,%s)=%d", label, id, sig, out)
		release(success)
		release(!success) // a second release must be a no-op
	}
	step("first", "s1", "sig", true, 0)
	step("dup-immediate", "s1", "sig", true, time.Second)
	step("other-sig", "s1", "sig2", false, time.Second)
	step("cooldown-same-sig", "s1", "sig2", true, 5*time.Second)
	step("after-cooldown", "s1", "sig2", true, 6*time.Second)
	step("dup-window-edge", "s1", "sig2", true, 59*time.Second)
	step("after-window", "s1", "sig2", true, 2*time.Second)
	step("other-session", "s2", "sig", true, 0)
	// in-flight: hold the first claim open
	rel, out := g.TryBegin("s3", "x")
	d.p("inflight first=%d", out)
	_, busy := g.TryBegin("s3", "x")
	d.p("inflight second=%d", busy)
	g2 := &SessionNudgeGuard{now: func() time.Time { return now }}
	_, o1 := g2.TryBegin("s4", "x")
	d.p("pre-abandon=%d", o1)
	g2.Abandon("s4")
	rel2, o2 := g2.TryBegin("s4", "x")
	d.p("post-abandon=%d (abandon records nothing)", o2)
	rel2(true)
	rel(true)
	// TTL + capacity eviction
	g3 := &SessionNudgeGuard{now: func() time.Time { return now }}
	for i := 0; i < nudgeMaxRecords+10; i++ {
		now = now.Add(time.Millisecond)
		r, _ := g3.TryBegin(fmt.Sprintf("e%04d", i), "sig")
		r(true)
	}
	d.p("records after cap fill=%d", len(g3.last))
	now = now.Add(nudgeRecordTTL + time.Second)
	r, o := g3.TryBegin("late", "sig")
	r(true)
	d.p("after ttl: outcome=%d records=%d", o, len(g3.last))
	var zero SessionNudgeGuard
	z, zo := zero.TryBegin("z", "s")
	z(false)
	d.p("zero value ready: %d", zo)
}

type goldenPM struct {
	instance.ProcessManager
	name string
}

func (n goldenPM) GetSessionIdentifier() string     { return n.name }
func (n goldenPM) GetPanePID() (int32, error)       { return 0, errors.New("no pane") }
func (n goldenPM) HasSession() bool                 { return true }
func (n goldenPM) IsAlive() bool                    { return true }
func (n goldenPM) HasMeaningfulContent(string) bool { return false }

func (d *dumper) humanActivity(t *testing.T) {
	t.Helper()
	d.p("# HumanActivity")
	now := time.Unix(1_800_000_000, 0)
	act := NewHumanActivity(func() time.Time { return now })
	act.Record("ssq_a")
	for _, adv := range []time.Duration{0, 4 * time.Second, time.Second, time.Nanosecond, time.Second, 54 * time.Second, time.Second} {
		now = now.Add(adv)
		d.p("t+%s within5=%t within1m=%t other=%t", adv, act.Within("ssq_a", humanActiveWindow), act.Within("ssq_a", time.Minute), act.Within("ssq_b", time.Minute))
	}
	for i := 0; i < 3; i++ {
		act.Record(fmt.Sprintf("ssq_%d", i))
		now = now.Add(humanActivityKeepAfter)
	}
	act.Record("ssq_late")
	d.p("after prune: a=%t late=%t entries=%d", act.Within("ssq_a", 24*time.Hour), act.Within("ssq_late", time.Second), len(act.last))
	d.p("window=%s keep=%s", humanActiveWindow, humanActivityKeepAfter)

	inst := instance.NewStartedInstanceForTest(t, "golden-human", goldenPM{name: "ssq_a"})
	nameless := instance.NewStartedInstanceForTest(t, "golden-nameless", goldenPM{name: ""})
	base := time.Unix(1_800_000_000, 0)
	cases := []struct {
		name    string
		clients []tmux.ClientActivity
		err     error
		nilList bool
		ours    bool
		record  bool
		inst    *instance.Instance
	}{
		{name: "none"},
		{name: "web-signal", record: true},
		{name: "human-attach", clients: []tmux.ClientActivity{{Session: "ssq_a", Activity: base.Add(-2 * time.Second)}}},
		{name: "idle-attach", clients: []tmux.ClientActivity{{Session: "ssq_a", Activity: base.Add(-6 * time.Second)}}},
		{name: "edge-5s", clients: []tmux.ClientActivity{{Session: "ssq_a", Activity: base.Add(-5 * time.Second)}}},
		{name: "control-mode", clients: []tmux.ClientActivity{{Session: "ssq_a", ControlMode: true, Activity: base.Add(-time.Second)}}},
		{name: "other-session", clients: []tmux.ClientActivity{{Session: "ssq_b", Activity: base.Add(-time.Second)}}},
		{name: "ours", clients: []tmux.ClientActivity{{Session: "ssq_a", Activity: base.Add(-time.Second)}}, ours: true},
		{name: "list-error", err: errors.New("no server")},
		{name: "list-error-with-web-signal", err: errors.New("no server"), record: true},
		{name: "nil-lister", nilList: true, clients: []tmux.ClientActivity{{Session: "ssq_a", Activity: base}}},
		{name: "nameless", clients: []tmux.ClientActivity{{Session: "", Activity: base}}, inst: nameless},
	}
	for _, c := range cases {
		act := NewHumanActivity(func() time.Time { return base })
		if c.record {
			act.Record("ssq_a")
		}
		var lister tmuxClientLister
		if !c.nilList {
			clients, err := c.clients, c.err
			lister = func(context.Context, string) ([]tmux.ClientActivity, error) { return clients, err }
		}
		g := NewHumanActiveGuard(act, lister)
		ours := c.ours
		g.ours = func(tmux.ClientActivity) bool { return ours }
		target := inst
		if c.inst != nil {
			target = c.inst
		}
		before := HumanActiveRefusals()
		got := g.Active(context.Background(), target)
		d.p("guard %s => active=%t refusalsDelta=%d", c.name, got, HumanActiveRefusals()-before)
	}
}

func (d *dumper) inputMetrics() {
	d.p("# terminal input metrics")
	for b := 0; b < 256; b++ {
		d.p("classify %#02x => %s", b, classifyInput([]byte{byte(b)}))
	}
	long := strings.Repeat("a", terminalInputClassifyLimit+5)
	samples := []string{"", "hello", "héllo", "\x1b[A", "a\x1b[A", "\xff", "ab\x00", "日本語", strings.Repeat("a", terminalInputClassifyLimit), long, long[:terminalInputClassifyLimit-1] + "日", "a\u0085b", "\u00a0", "\u009f", "line\r\n\ttab\x7f"}
	for _, s := range samples {
		d.p("classify %q => %s", s, classifyInput([]byte(s)))
	}
	for _, l := range []string{ListenerLocal, ListenerRemote, "unknown", "", "attacker", "LOCAL"} {
		d.p("listener %q => %d", l, normalizeInputListener(l))
	}
	d.p("names %v %v %v %v", inputClassNames, inputPathNames, inputStageNames, inputListenerNames)

	var mu sync.Mutex
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	var logs []string
	m := newTerminalInputMetrics(func() time.Time { mu.Lock(); defer mu.Unlock(); return now }, func(msg string, args ...any) {
		logs = append(logs, fmt.Sprint(append([]any{msg}, args...)...))
	})
	adv := func(s time.Duration) { mu.Lock(); now = now.Add(s); mu.Unlock() }
	paths := []InputPath{PathCM, PathShell, PathCapture, PathStream}
	frames := [][]byte{[]byte("x"), []byte("\x1b[B"), nil, []byte("\xff\xfe")}
	for si, sess := range []string{"s-a", "s-b"} {
		for _, p := range paths {
			src := m.source(p, []string{ListenerLocal, ListenerRemote}[si], sess)
			for i, f := range frames {
				adv(20 * time.Second)
				m.record(src, f, StageReceived)
				if i%2 == 0 {
					m.record(src, f, StageForwarded)
				}
			}
		}
	}
	m.record(nil2(m), []byte("x"), StageReceived)
	for _, class := range []InputClass{InputClassText, InputClassEscape, InputClassOther} {
		for _, l := range []string{ListenerLocal, ListenerRemote, "weird"} {
			for _, p := range paths {
				for _, st := range []InputStage{StageReceived, StageForwarded} {
					f, by := m.count(class, l, p, st)
					d.p("count %s|%s|%s|%s => %d %d", class, l, p, st, f, by)
				}
			}
		}
	}
	for _, row := range m.tally.snapshot() {
		d.p("tally %+v", row)
	}
	d.p("render:\n%s", RenderTerminalInputText(m.tally.snapshot()))
	d.p("render empty: %q", RenderTerminalInputText(nil))
	for _, l := range logs {
		d.p("log %s", l)
	}
	RecordTerminalInput(nil, []byte("x"), StageReceived)
	d.p("source listener names %s %s", NewInputSource(PathCM, ListenerRemote, "g").ListenerName(), NewInputSource(PathCM, "zzz", "g").ListenerName())
}

func nil2(m *terminalInputMetrics) *InputSource { return m.source(PathCM, ListenerLocal, "s-extra") }

// isolatedSocketPID masks the per-process suffix of the default test-mode tmux socket.
var isolatedSocketPID = regexp.MustCompile(`test-isolated-[0-9]+`)
