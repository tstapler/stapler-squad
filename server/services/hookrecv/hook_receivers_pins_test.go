package hookrecv

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/tstapler/stapler-squad/session/gitwiring" // registers the provider the real drift check needs
	"github.com/tstapler/stapler-squad/testutil/gitfixture"
)

const bodyCap = 64 * 1024

// jsonOfLen returns a valid JSON object of exactly n bytes: prefix + padding + suffix.
func jsonOfLen(t *testing.T, prefix, suffix string, n int) string {
	t.Helper()
	pad := n - len(prefix) - len(suffix)
	require.Positive(t, pad)
	s := prefix + strings.Repeat("x", pad) + suffix
	require.Len(t, s, n)
	require.True(t, json.Valid([]byte(s)))
	return s
}

func TestHandlePostToolUse_should_ParseBody_OnlyUpToExactly64KiB(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		n    int
		want string
	}{{bodyCap, "Edit"}, {bodyCap + 1, ""}} {
		h := NewHookReceiver()
		var got string
		h.SetPostToolUseObserver(func(_ context.Context, _, tool string) { got = tool })
		postToolUse(t, h, nil, jsonOfLen(t, `{"tool_name":"Edit","pad":"`, `"}`, tc.n))
		assert.Equal(t, tc.want, got, "body length %d", tc.n)
	}
}

const driftPrefix = `{"tool_name":"Bash","cwd":"/nx","tool_input":{"command":"git commit -m x"},"pad":"`

func driftResponse(h *HookReceiver, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/post-tool-use-drift-check", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandlePostToolUseDriftCheck(rec, req)
	return rec
}

func TestHandlePostToolUseDriftCheck_should_ParseBody_OnlyUpToExactly64KiB(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		n      int
		speaks bool
	}{{bodyCap, true}, {bodyCap + 1, false}} {
		h := NewHookReceiver()
		h.SetDriftCheckFn(func(string, string) (int, error) { return 1000, nil })
		rec := driftResponse(h, jsonOfLen(t, driftPrefix, `"}`, tc.n))
		assert.Equal(t, tc.speaks, rec.Body.Len() > 0, "body length %d", tc.n)
	}
}

func TestHandlePostToolUseDriftCheck_should_UseSteeringThreshold20_When_NoneConfigured(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		behind int
		speaks bool
	}{{19, false}, {20, true}, {49, true}} {
		h := NewHookReceiver()
		h.SetDriftCheckFn(func(string, string) (int, error) { return tc.behind, nil })
		rec := driftResponse(h, driftPrefix+`"}`)
		assert.Equal(t, tc.speaks, rec.Body.Len() > 0, "behind %d", tc.behind)
		if tc.speaks {
			assert.Contains(t, rec.Body.String(), "steering threshold: 20 commits")
		}
	}
}

// Exercises the production check (a real fetch through BehindOriginMain) end to end.
func TestHandlePostToolUseDriftCheck_should_InjectContext_When_RealCheckSeesUpstreamCommits(t *testing.T) {
	t.Parallel()
	origin := filepath.Join(t.TempDir(), "origin")
	require.NoError(t, os.MkdirAll(origin, 0o755))
	gitfixture.Run(t, origin, "init", "-q", "-b", "main")
	gitfixture.ConfigureNativeIdentity(t, origin)
	commit := func(dir, name string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644))
		gitfixture.Run(t, dir, "add", name)
		gitfixture.Run(t, dir, "commit", "-q", "-m", name)
	}
	commit(origin, "base.txt")
	work := filepath.Join(t.TempDir(), "work")
	gitfixture.Run(t, filepath.Dir(work), "clone", "-q", origin, work)
	gitfixture.ConfigureNativeIdentity(t, work)
	gitfixture.Run(t, work, "checkout", "-q", "-b", "feature")
	for _, n := range []string{"u1", "u2", "u3"} {
		commit(origin, n)
	}

	h := NewHookReceiver() // production driftCheckFn
	h.SetDriftThreshold(3)
	payload := `{"tool_name":"Bash","cwd":"` + work + `","tool_input":{"command":"git push"}}`
	rec := driftResponse(h, payload)

	var resp struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
	assert.Contains(t, resp.HookSpecificOutput.AdditionalContext, "(feature) is 3 commits behind origin/main")
}
