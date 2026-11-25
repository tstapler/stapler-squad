package piextension

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// The golden was recorded from the pre-move code in server/services (origin/main)
// with PIEXT_GOLDEN_RECORD=1; this file is identical there except for the package
// clause. Any diff in health transitions, status codes, headers or bodies is a
// behaviour change.
const goldenPath = "testdata/pi_extension.golden"

type goldenOut struct{ b strings.Builder }

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

func healthScenario(g *goldenOut) {
	g.line("# constants")
	g.line("reping=%s grace=%s", piExtensionRepingInterval, piExtensionHealthGraceWindow)
	for _, h := range []PiExtensionHealth{PiExtensionHealthUnknown, PiExtensionHealthLoaded, PiExtensionHealthFailed, PiExtensionHealth(-1), PiExtensionHealth(99)} {
		g.line("String(%d)=%s", int(h), h)
	}

	g.line("# default grace window boundary (graceWindow unset)")
	tr, clock := newTestPiExtensionHealthTracker(0)
	g.line("first=%s", tr.HealthFor("a"))
	clock.Advance(piExtensionHealthGraceWindow)
	g.line("at-grace=%s", tr.HealthFor("a"))
	clock.Advance(time.Nanosecond)
	g.line("past-grace=%s", tr.HealthFor("a"))
	g.line("stays-failed=%s", tr.HealthFor("a"))
	tr.RecordPing("a")
	g.line("after-ping=%s", tr.HealthFor("a"))

	g.line("# custom grace window, loaded->failed->loaded")
	tr, clock = newTestPiExtensionHealthTracker(time.Minute)
	tr.RecordPing("s")
	g.line("loaded=%s", tr.HealthFor("s"))
	clock.Advance(time.Minute)
	g.line("loaded-at-grace=%s", tr.HealthFor("s"))
	clock.Advance(time.Nanosecond)
	g.line("loaded-past-grace=%s", tr.HealthFor("s"))
	tr.RecordPing("s")
	g.line("recovered=%s", tr.HealthFor("s"))
	tr.RecordPing("s")
	clock.Advance(30 * time.Second)
	tr.RecordPing("s")
	clock.Advance(45 * time.Second)
	g.line("reping-extends=%s", tr.HealthFor("s"))

	g.line("# ping first keeps firstObservedAt")
	tr, clock = newTestPiExtensionHealthTracker(time.Minute)
	tr.RecordPing("p")
	clock.Advance(10 * time.Second)
	tr.RecordPing("p")
	e, ok := tr.sessions.Load("p")
	g.line("entry ok=%v state=%s first=%s last=%s", ok, e.state, e.firstObservedAt.Format(time.RFC3339Nano), e.lastPingAt.Format(time.RFC3339Nano))

	g.line("# isolation, empty id, forget")
	tr, clock = newTestPiExtensionHealthTracker(time.Minute)
	tr.RecordPing("x")
	clock.Advance(2 * time.Minute)
	g.line("x=%s y=%s empty=%q", tr.HealthFor("x"), tr.HealthFor("y"), tr.HealthFor(""))
	tr.RecordPing("")
	tr.Forget("")
	tr.Forget("nope")
	tr.Forget("x")
	_, ok = tr.sessions.Load("x")
	g.line("x-forgotten=%v size=%d", !ok, tr.sessions.Size())
	g.line("x-after-forget=%s", tr.HealthFor("x"))

	g.line("# real clock, default window")
	realClock := NewPiExtensionHealthTracker()
	g.line("real-first=%s", realClock.HealthFor("r"))
	realClock.RecordPing("r")
	g.line("real-loaded=%s", realClock.HealthFor("r"))
}

func statusScenario(t *testing.T, g *goldenOut) {
	svc := NewPiExtensionStatusService()
	call := func(label, method string) {
		req := httptest.NewRequest(method, "/api/pi-extension-status", nil)
		rec := httptest.NewRecorder()
		svc.HandlePiExtensionStatus(rec, req)
		g.record(label+" "+method, rec)
	}
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodHead, http.MethodPatch}

	g.line("# absent")
	svctest.WithFakeHome(t)
	for _, m := range methods {
		call("absent", m)
	}

	g.line("# present file, then directory at the same path")
	home := t.TempDir()
	svctest.WithFakeHomePath(t, home)
	ext := filepath.Join(home, piApprovalExtensionRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(ext), 0o755))
	require.NoError(t, os.WriteFile(ext, []byte("// stub"), 0o644))
	for _, m := range methods {
		call("file", m)
	}
	require.NoError(t, os.Remove(ext))
	require.NoError(t, os.Mkdir(ext, 0o755))
	call("dir", http.MethodGet)

	g.line("# unresolvable home")
	svctest.WithFakeHomePath(t, "")
	for _, m := range methods {
		call("nohome", m)
	}
}

func TestPiExtensionGolden(t *testing.T) {
	var g goldenOut
	healthScenario(&g)
	statusScenario(t, &g)
	got := g.b.String()
	if os.Getenv("PIEXT_GOLDEN_RECORD") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
