package jules

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// fakeTokenSource is a JulesTokenSource test double returning a fixed key.
type fakeTokenSource struct {
	key JulesAPIKey
	err error
}

func (f fakeTokenSource) APIKey(_ context.Context) (JulesAPIKey, error) {
	return f.key, f.err
}

// newTestClient builds a Client pointed at server, with baseURL carrying the
// same "/v1alpha" suffix the real Jules API base URL does — so path
// assertions below (e.g. "/v1alpha/sessions/abc") match production shape.
func newTestClient(t *testing.T, server *httptest.Server, key JulesAPIKey) *Client {
	t.Helper()
	return NewClient(fakeTokenSource{key: key}, WithBaseURL(server.URL+"/v1alpha"))
}

func TestJulesClient_GetSession_should_SendGoogApiKeyHeader_When_Called(t *testing.T) {
	var gotMethod, gotPath, gotAPIKeyHeader, gotAuthHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAPIKeyHeader = r.Header.Get("x-goog-api-key")
		gotAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"sessions/abc","state":"QUEUED"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server, JulesAPIKey("test-key-123"))
	_, err := client.GetSession(context.Background(), JulesSessionName("sessions/abc"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1alpha/sessions/abc" {
		t.Errorf("path = %q, want /v1alpha/sessions/abc", gotPath)
	}
	if gotAPIKeyHeader != "test-key-123" {
		t.Errorf("x-goog-api-key = %q, want test-key-123", gotAPIKeyHeader)
	}
	if gotAuthHeader != "" {
		t.Errorf("Authorization header = %q, want empty", gotAuthHeader)
	}
}

func TestJulesClient_CreateSession_should_SendFireAndForgetBody_When_Called(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"sessions/new","state":"QUEUED"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server, JulesAPIKey("test-key"))
	_, err := client.CreateSession(context.Background(), CreateSessionRequest{
		Prompt:         "Fix the flaky poller test",
		Source:         JulesSourceName("sources/github-tstapler-stapler-squad"),
		StartingBranch: GitHubBranchRef("backlog/fix-flaky-poller"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("failed to decode observed request body %q: %v", gotBody, err)
	}
	want := map[string]any{
		"prompt": "Fix the flaky poller test",
		"sourceContext": map[string]any{
			"source": "sources/github-tstapler-stapler-squad",
			"githubRepoContext": map[string]any{
				"startingBranch": "backlog/fix-flaky-poller",
			},
		},
		"requirePlanApproval": false,
		"automationMode":      "AUTO_CREATE_PR",
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(decoded)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("request body = %s, want %s", gotBody, wantJSON)
	}
}

func TestJulesClient_GetSession_should_ParsePullRequestOutput_When_SessionCompleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"sessions/abc","state":"COMPLETED","outputs":[{"pullRequest":{"url":"https://github.com/tstapler/stapler-squad/pull/700","title":"Fix flaky poller test"}}]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server, JulesAPIKey("test-key"))
	session, err := client.GetSession(context.Background(), JulesSessionName("sessions/abc"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.State != JulesStateCompleted {
		t.Fatalf("State = %v, want JulesStateCompleted", session.State)
	}
	if len(session.Outputs) != 1 || session.Outputs[0].PullRequest == nil {
		t.Fatalf("Outputs = %+v, want one entry with a PullRequest", session.Outputs)
	}
	if got := session.Outputs[0].PullRequest.URL; got != "https://github.com/tstapler/stapler-squad/pull/700" {
		t.Fatalf("Outputs[0].PullRequest.URL = %q", got)
	}
}

func TestJulesClient_GetSession_should_ArmLimiterAndExposeRetryAfter_When_ServerReturns429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := newTestClient(t, server, JulesAPIKey("test-key"))
	// Inject a fixed clock so RetryAfter() is deterministic regardless of
	// wall-clock time elapsed between arming and assertion.
	fakeNow := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	client.limiter.now = func() time.Time { return fakeNow }

	_, err := client.GetSession(context.Background(), JulesSessionName("sessions/abc"))
	if !errors.Is(err, ErrJulesRateLimited) {
		t.Fatalf("expected ErrJulesRateLimited, got %v", err)
	}
	if !client.IsLimited() {
		t.Fatal("expected IsLimited() == true after a 429 response")
	}
	if got := client.RetryAfter(); got != 120*time.Second {
		t.Fatalf("RetryAfter() = %v, want 120s", got)
	}

	// Advance the injected clock past the Retry-After window: the limiter
	// disarms.
	client.limiter.now = func() time.Time { return fakeNow.Add(121 * time.Second) }
	if client.IsLimited() {
		t.Fatal("expected IsLimited() == false after the window passes")
	}
}

// TestJulesPackage_should_NotImportSessionOrServer_When_DepsListed is the
// fitness function enforcing the Definition of Done constraint: the jules/
// package must never import session/ or server/, so alpha-API churn stays
// confined to this directory.
func TestJulesPackage_should_NotImportSessionOrServer_When_DepsListed(t *testing.T) {
	// Use the fully-qualified import path (rather than "./jules") so this
	// resolves correctly regardless of the test binary's working directory
	// — "go test" runs with cwd set to this package's own source directory,
	// where a relative "./jules" would not exist.
	out, err := safeexec.CommandContext(context.Background(), "go", "list", "-deps", "github.com/tstapler/stapler-squad/jules").Output()
	if err != nil {
		t.Fatalf("go list -deps failed: %v", err)
	}
	deps := string(out)
	for _, forbidden := range []string{
		"github.com/tstapler/stapler-squad/session",
		"github.com/tstapler/stapler-squad/server",
	} {
		if strings.Contains(deps, forbidden) {
			t.Errorf("jules package deps unexpectedly include %q", forbidden)
		}
	}
}
