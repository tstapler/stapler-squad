package ssqcli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/localtoken"
)

type recorded struct {
	mu     sync.Mutex
	path   string
	auth   string
	body   string
	header http.Header
}

func fakeServer(t *testing.T, status int, reply string) (*httptest.Server, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.path, rec.auth, rec.body, rec.header = r.URL.Path, r.Header.Get("Authorization"), string(b), r.Header.Clone()
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func run(t *testing.T, env map[string]string, configDir string, args ...string) (stdout string, err error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	root := NewRootCommand(Env{
		Getenv:    func(k string) string { return env[k] },
		ConfigDir: func() (string, error) { return configDir, nil },
		Stdin:     strings.NewReader(""),
		Stdout:    &out,
		Stderr:    &errBuf,
	})
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), err
}

func TestKebab_should_SplitWordsAndAcronyms(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "list-sessions", kebab("ListSessions"))
	assert.Equal(t, "get-vcs-status", kebab("GetVCSStatus"))
	assert.Equal(t, "llm-backend", kebab("LLMBackend"))
}

func TestDiscoverMethods_should_CoverSessionServiceUnaryRPCsOnly(t *testing.T) {
	t.Parallel()
	byProc := map[string]Method{}
	seen := map[string]bool{}
	for _, m := range DiscoverMethods() {
		byProc[m.Procedure] = m
		key := m.Group + "/" + m.Name
		assert.False(t, seen[key], "duplicate command %s", key)
		seen[key] = true
	}
	assert.Contains(t, byProc, sessionSvc+"ListSessions")
	assert.NotContains(t, byProc, sessionSvc+"WatchSessions", "server-streaming RPC must be skipped")
	assert.NotContains(t, byProc, sessionSvc+"StreamTerminal", "bidi RPC must be skipped")
}

func TestAliases_should_TargetExistingRPCsWithIDField(t *testing.T) {
	t.Parallel()
	byProc := map[string]Method{}
	for _, m := range DiscoverMethods() {
		byProc[m.Procedure] = m
	}
	for _, a := range sessionAliases() {
		m, ok := byProc[a.procedure]
		require.True(t, ok, "alias %q targets missing RPC %s", a.use, a.procedure)
		if a.positional != "" {
			assert.NotNil(t, m.Input.Fields().ByName("id"), "alias %q positional field", a.use)
		}
	}
}

func TestCall_should_PostFlagsAsJSONWithBearerToken(t *testing.T) {
	t.Parallel()
	srv, rec := fakeServer(t, 200, `{"sessions":[]}`)
	out, err := run(t, nil, t.TempDir(), "--url", srv.URL, "--token", "tok", "--compact",
		"session", "list-sessions", "--category", "work", "--hide-paused", "--status", "paused")
	require.NoError(t, err)
	assert.Equal(t, "/api/session.v1.SessionService/ListSessions", rec.path)
	assert.Equal(t, "Bearer tok", rec.auth)
	assert.JSONEq(t, `{"category":"work","hidePaused":true,"status":"SESSION_STATUS_PAUSED"}`, rec.body)
	assert.Equal(t, "{\"sessions\":[]}\n", out)
	assert.NotEmpty(t, rec.header.Get("Connect-Timeout-Ms"))
}

func TestAlias_should_FillIDFromPositionalArg(t *testing.T) {
	t.Parallel()
	srv, rec := fakeServer(t, 200, `{}`)
	_, err := run(t, nil, t.TempDir(), "--url", srv.URL, "sessions", "restart", "my-session", "--preserve-output")
	require.NoError(t, err)
	assert.Equal(t, "/api/session.v1.SessionService/RestartSession", rec.path)
	assert.JSONEq(t, `{"id":"my-session","preserveOutput":true}`, rec.body)
}

func TestAlias_should_FailFast_When_IDMissing(t *testing.T) {
	t.Parallel()
	_, err := run(t, nil, t.TempDir(), "--url", "http://127.0.0.1:1", "sessions", "get")
	assert.ErrorContains(t, err, "missing <id>")
}

func TestJSONFlag_should_SeedBodyAndBeOverriddenByFlags(t *testing.T) {
	t.Parallel()
	srv, rec := fakeServer(t, 200, `{}`)
	_, err := run(t, nil, t.TempDir(), "--url", srv.URL, "--json", `{"id":"a","force":false}`,
		"session", "delete-session", "--force")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"a","force":true}`, rec.body)
}

func TestJSONFlag_should_RejectBodyNotMatchingRequestType(t *testing.T) {
	t.Parallel()
	_, err := run(t, nil, t.TempDir(), "--url", "http://127.0.0.1:1", "--json", `{"nope":1}`, "session", "get-session")
	assert.ErrorContains(t, err, "--json does not match")
}

func TestToken_should_FallBackToEnvThenLocalTokenFile(t *testing.T) {
	t.Parallel()
	srv, rec := fakeServer(t, 200, `{}`)
	dir := t.TempDir()
	fileTok, err := localtoken.LoadOrCreate(localtoken.Path(dir))
	require.NoError(t, err)

	_, err = run(t, nil, dir, "--url", srv.URL, "sessions", "list")
	require.NoError(t, err)
	assert.Equal(t, "Bearer "+fileTok, rec.auth)

	_, err = run(t, map[string]string{"STAPLER_SQUAD_TOKEN": "envtok"}, dir, "--url", srv.URL, "sessions", "list")
	require.NoError(t, err)
	assert.Equal(t, "Bearer envtok", rec.auth)
}

func TestURL_should_DefaultFromEnv(t *testing.T) {
	t.Parallel()
	srv, rec := fakeServer(t, 200, `{}`)
	_, err := run(t, map[string]string{"STAPLER_SQUAD_URL": srv.URL}, t.TempDir(), "sessions", "list")
	require.NoError(t, err)
	assert.Contains(t, rec.path, "ListSessions")
}

func TestCall_should_DecodeConnectError(t *testing.T) {
	t.Parallel()
	srv, _ := fakeServer(t, 401, `{"code":"unauthenticated","message":"missing token"}`)
	_, err := run(t, nil, t.TempDir(), "--url", srv.URL, "sessions", "list")
	require.Error(t, err)
	assert.ErrorContains(t, err, "unauthenticated: missing token")
	assert.ErrorContains(t, err, "--token")
}

func TestCall_should_TimeOutWithClearMessage_When_ServerHangs(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(func() { close(release); srv.Close() })
	start := time.Now()
	_, err := run(t, nil, t.TempDir(), "--url", srv.URL, "--timeout", "100ms", "sessions", "list")
	require.Error(t, err)
	assert.ErrorContains(t, err, "timed out after 100ms")
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestCall_should_ExplainUnreachableServer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, err := run(t, nil, t.TempDir(), "--url", url, "sessions", "list")
	assert.ErrorContains(t, err, "is stapler-squad running?")
}

func TestJSONFlag_should_ReadBodyFromFile(t *testing.T) {
	t.Parallel()
	srv, rec := fakeServer(t, 200, `{}`)
	path := filepath.Join(t.TempDir(), "body.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"id":"from-file"}`), 0o600))
	_, err := run(t, nil, t.TempDir(), "--url", srv.URL, "--json", "@"+path, "session", "get-session")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"from-file"}`, rec.body)
}
