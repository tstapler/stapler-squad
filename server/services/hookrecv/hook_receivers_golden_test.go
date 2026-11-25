package hookrecv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/require"
)

// The golden was recorded from the pre-move handlers in server/services (origin/main)
// with HOOKRECV_GOLDEN_RECORD=1 and drives only the exported API, so the same file
// replays against either package. Hook endpoints are a trust surface: any diff in
// status, headers, body or observer calls is a behaviour change.
const goldenPath = "testdata/hook_receivers.golden"

type goldenOut struct {
	b strings.Builder
}

func (g *goldenOut) line(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	g.b.WriteString(s)
	g.b.WriteString("\n")
}

func serve(h *HookReceiver, method, path string, headers map[string]string, body io.Reader) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(method, path, body)
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		req.Header.Set(k, headers[k])
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func (g *goldenOut) record(label string, rec *httptest.ResponseRecorder) {
	hk := make([]string, 0, len(rec.Header()))
	for k, v := range rec.Header() {
		hk = append(hk, k+"="+strings.Join(v, "|"))
	}
	sort.Strings(hk)
	g.line("%s -> %d headers=[%s] body=%q", label, rec.Code, strings.Join(hk, ";"), rec.Body.String())
}

func goldenSimpleEndpoints(g *goldenOut) {
	paths := []string{
		"/api/hooks/stop", "/api/hooks/pre-tool-use", "/api/hooks/post-tool-use",
		"/api/hooks/prompt-submit", "/api/hooks/post-tool-use-drift-check",
		"/api/hooks/unknown", "/api/hooks/stop/", "/api/hooks/permission-request",
	}
	bodies := map[string]string{
		"empty": "", "small": `{"tool_name":"Bash"}`, "badjson": "{not json",
		"big": strings.Repeat("a", 70000), "exactcap": strings.Repeat("b", 64*1024),
	}
	bnames := []string{"empty", "small", "badjson", "big", "exactcap"}
	for _, p := range paths {
		for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodHead} {
			for _, bn := range bnames {
				for _, hdrs := range []map[string]string{nil, {"X-CS-Session-ID": "s1", "X-CS-Tool-Name": "Read"}} {
					h := NewHookReceiver()
					h.SetDriftCheckFn(func(string, string) (int, error) { return 0, nil })
					g.record(fmt.Sprintf("%s %s body=%s hdr=%d", m, p, bn, len(hdrs)),
						serve(h, m, p, hdrs, strings.NewReader(bodies[bn])))
				}
			}
		}
	}
}

type obsCall struct{ session, tool string }

func goldenPostToolUse(g *goldenOut) {
	type sc struct {
		name    string
		headers map[string]string
		body    string
	}
	scs := []sc{
		{"hdr-only", map[string]string{"X-CS-Session-ID": "a", "X-CS-Tool-Name": "Bash"}, ""},
		{"body-only", map[string]string{"X-CS-Session-ID": "a"}, `{"tool_name":"AskUserQuestion","tool_use_id":"t"}`},
		{"both-hdr-wins", map[string]string{"X-CS-Session-ID": "a", "X-CS-Tool-Name": "Bash"}, `{"tool_name":"Read"}`},
		{"badjson", map[string]string{"X-CS-Session-ID": "a"}, "not json"},
		{"tool-not-string", map[string]string{"X-CS-Session-ID": "a"}, `{"tool_name":7}`},
		{"tool-null", map[string]string{"X-CS-Session-ID": "a"}, `{"tool_name":null}`},
		{"no-session", nil, `{"tool_name":"Edit"}`},
		{"empty-both", nil, ""},
		{"truncated-past-cap", map[string]string{"X-CS-Session-ID": "z"}, `{"tool_name":"Early","pad":"` + strings.Repeat("x", 70000) + `"}`},
		{"tool-after-cap", map[string]string{"X-CS-Session-ID": "z"}, `{"pad":"` + strings.Repeat("x", 65000) + `","tool_name":"Late"}`},
		{"array-body", map[string]string{"X-CS-Session-ID": "z"}, `[{"tool_name":"X"}]`},
	}
	for _, s := range scs {
		var mu sync.Mutex
		var calls []obsCall
		h := NewHookReceiver()
		h.SetPostToolUseObserver(func(_ context.Context, sess, tool string) {
			mu.Lock()
			calls = append(calls, obsCall{sess, tool})
			mu.Unlock()
		})
		rec := serve(h, http.MethodPost, "/api/hooks/post-tool-use", s.headers, strings.NewReader(s.body))
		g.record("post-tool-use/"+s.name, rec)
		g.line("  observer calls: %v", calls)
		h2 := NewHookReceiver()
		g.record("post-tool-use/"+s.name+"/no-observer", serve(h2, http.MethodPost, "/api/hooks/post-tool-use", s.headers, strings.NewReader(s.body)))
	}
}

type driftCase struct{ cwd, tool, cmd string }

func driftBody(c driftCase, raw string) []byte {
	if raw != "" {
		return []byte(raw)
	}
	in, _ := json.Marshal(map[string]string{"command": c.cmd})
	b, _ := json.Marshal(map[string]any{
		"session_id": "sid", "cwd": c.cwd, "tool_name": c.tool, "tool_input": json.RawMessage(in),
	})
	return b
}

func goldenDrift(g *goldenOut, repoDir, plainDir string) {
	cmds := []string{
		`git commit -m "x"`, "git push origin b", "git status", `git -C /w commit -m x`, `git -c a=b push`,
		"git add -A && git commit -m x && git push", "git add .; git commit", "git log | head && git diff",
		"echo git commit", "GIT_AUTHOR_NAME=x git commit", "git --git-dir /g commit", "git --no-pager commit",
		"git\ncommit", "git status\ngit push", "cat x || git push", "", "   ", "git", "git -C", "ls -la",
		"git --work-tree /w --namespace n commit", "git commit-tree abc", "git pushall",
	}
	for _, cwd := range []string{repoDir, plainDir, "/nonexistent/worktree"} {
		for _, behind := range []int{0, 1, 19, 20, 21, 500} {
			for _, cmd := range cmds {
				h := NewHookReceiver()
				h.SetDriftThreshold(20)
				h.SetDriftCheckMinInterval(time.Nanosecond)
				var gotCwd, gotBranch string
				h.SetDriftCheckFn(func(w, b string) (int, error) { gotCwd, gotBranch = w, b; return behind, nil })
				c := driftCase{cwd: cwd, tool: "Bash", cmd: cmd}
				rec := serve(h, http.MethodPost, "/api/hooks/post-tool-use-drift-check",
					map[string]string{"X-CS-Session-ID": "sid"}, bytes.NewReader(driftBody(c, "")))
				g.record(fmt.Sprintf("drift cwd=%s behind=%d cmd=%q", cwd, behind, cmd), rec)
				g.line("  checkFn saw cwd=%s branch=%s", gotCwd, gotBranch)
			}
		}
	}
	// Payload shape matrix.
	raws := []string{
		"", "not json", `{}`, `{"tool_name":"Bash"}`, `{"tool_name":"Bash","cwd":"/c"}`,
		`{"tool_name":"Bash","cwd":"/c","tool_input":null}`, `{"tool_name":"Bash","cwd":"/c","tool_input":"git commit"}`,
		`{"tool_name":"Bash","cwd":"/c","tool_input":{"command":7}}`, `{"tool_name":"Bash","cwd":"/c","tool_input":{}}`,
		`{"tool_name":"Edit","cwd":"/c","tool_input":{"command":"git commit"}}`, `{"tool_name":"bash","cwd":"/c","tool_input":{"command":"git commit"}}`,
		`{"tool_name":"Bash","cwd":"/c","tool_input":{"command":"git commit"}}`, `{"tool_name":"Bash","cwd":7}`, `{"tool_name":"Bash","tool_input":{"command":"git commit"}}`, `{"tool_name":"Bash","cwd":"","tool_input":{"command":"git push"}}`,
		`[]`, strings.Repeat("z", 70000),
		`{"tool_name":"Bash","cwd":"/c","tool_input":{"command":"git commit","pad":"` + strings.Repeat("p", 70000) + `"}}`,
	}
	for _, raw := range raws {
		h := NewHookReceiver()
		h.SetDriftThreshold(5)
		var calls int
		h.SetDriftCheckFn(func(string, string) (int, error) { calls++; return 9, nil })
		rec := serve(h, http.MethodPost, "/api/hooks/post-tool-use-drift-check", nil, strings.NewReader(raw))
		short := raw
		if len(short) > 60 {
			short = short[:60]
		}
		g.record(fmt.Sprintf("drift raw=%q(len %d)", short, len(raw)), rec)
		g.line("  checkFn calls=%d", calls)
	}
	// Check error fails open; thresholds.
	for _, thr := range []int{0, -3, 1, 9, 10} {
		h := NewHookReceiver()
		h.SetDriftThreshold(thr)
		h.SetDriftCheckFn(func(string, string) (int, error) { return 10, nil })
		c := driftCase{cwd: "/nx", tool: "Bash", cmd: "git commit"}
		g.record(fmt.Sprintf("drift threshold=%d behind=10", thr),
			serve(h, http.MethodPost, "/api/hooks/post-tool-use-drift-check", nil, bytes.NewReader(driftBody(c, ""))))
	}
	h := NewHookReceiver()
	h.SetDriftCheckFn(func(string, string) (int, error) { return 0, errors.New("fetch failed") })
	c := driftCase{cwd: "/nx", tool: "Bash", cmd: "git push"}
	g.record("drift check error", serve(h, http.MethodPost, "/api/hooks/post-tool-use-drift-check", nil, bytes.NewReader(driftBody(c, ""))))
	// nil checkFn falls back to the real one against a nonexistent dir (fails open) and a zero-value receiver works.
	zero := &HookReceiver{}
	g.record("drift zero-value receiver nil fn", serve(zero, http.MethodPost, "/api/hooks/post-tool-use-drift-check", nil, bytes.NewReader(driftBody(driftCase{cwd: "/nonexistent/zz", tool: "Bash", cmd: "git push"}, ""))))
	nilFn := NewHookReceiver()
	nilFn.SetDriftCheckFn(nil)
	g.record("drift SetDriftCheckFn(nil)", serve(nilFn, http.MethodPost, "/api/hooks/post-tool-use-drift-check", nil, bytes.NewReader(driftBody(driftCase{cwd: "/nonexistent/zz", tool: "Bash", cmd: "git push"}, ""))))
}

func goldenRateLimit(g *goldenOut) {
	type step struct{ cwd string }
	cases := []struct {
		name     string
		interval time.Duration
		steps    []step
	}{
		{"default-interval-same", 0, []step{{"/a"}, {"/a"}, {"/b"}, {"/a"}, {"/b"}}},
		{"negative-interval", -time.Second, []step{{"/a"}, {"/a"}}},
		{"long-interval", time.Hour, []step{{"/a"}, {"/a"}, {"/b"}, {"/b"}}},
		{"tiny-interval", time.Nanosecond, []step{{"/a"}, {"/a"}}},
	}
	for _, rc := range cases {
		h := NewHookReceiver()
		h.SetDriftThreshold(1)
		h.SetDriftCheckMinInterval(rc.interval)
		var calls []string
		h.SetDriftCheckFn(func(w, _ string) (int, error) { calls = append(calls, w); return 3, nil })
		for i, st := range rc.steps {
			c := driftCase{cwd: st.cwd, tool: "Bash", cmd: "git commit"}
			rec := serve(h, http.MethodPost, "/api/hooks/post-tool-use-drift-check", nil, bytes.NewReader(driftBody(c, "")))
			g.line("ratelimit %s step %d cwd=%s -> %d len(body)>0=%v", rc.name, i, st.cwd, rec.Code, rec.Body.Len() > 0)
		}
		g.line("  ratelimit %s checkFn calls=%v", rc.name, calls)
	}
	// Concurrent same-worktree burst: exactly one check should run.
	h := NewHookReceiver()
	h.SetDriftThreshold(1)
	h.SetDriftCheckMinInterval(time.Hour)
	var n atomic.Int64
	h.SetDriftCheckFn(func(string, string) (int, error) { n.Add(1); return 3, nil })
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := driftCase{cwd: "/burst", tool: "Bash", cmd: "git commit"}
			serve(h, http.MethodPost, "/api/hooks/post-tool-use-drift-check", nil, bytes.NewReader(driftBody(c, "")))
		}()
	}
	wg.Wait()
	g.line("burst checkFn calls=%d", n.Load())
}

func makeGitRepoOrPlain(t *testing.T) (repoDir, plainDir string) {
	t.Helper()
	repoDir = filepath.Join(t.TempDir(), "repo")
	plainDir = filepath.Join(t.TempDir(), "plain")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	require.NoError(t, os.MkdirAll(plainDir, 0o755))
	repo, err := gogit.PlainInit(repoDir, false)
	require.NoError(t, err)
	require.NoError(t, repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/feature-x")))
	return repoDir, plainDir
}

func TestHookReceivers_should_MatchGolden(t *testing.T) {
	repoDir, plainDir := makeGitRepoOrPlain(t)
	g := &goldenOut{}
	norm := strings.NewReplacer(repoDir, "<REPO>", plainDir, "<PLAIN>")

	goldenSimpleEndpoints(g)
	goldenPostToolUse(g)
	goldenDrift(g, repoDir, plainDir)
	goldenRateLimit(g)

	got := norm.Replace(g.b.String())
	if os.Getenv("HOOKRECV_GOLDEN_RECORD") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
