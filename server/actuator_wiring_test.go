package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/actuator"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/server/prwatchhost"
	"github.com/tstapler/stapler-squad/server/services/wsconn"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

func TestActuatorWorkflowEngineAndNudgeEndpoints_Return401OnRemoteChain_AndHostGuardRejectsForeignHostOnLocal(t *testing.T) {
	t.Parallel()
	// Only /actuator/workflow-engine exists in this story; the nudge-outcomes
	// endpoint named in the test title arrives with its own story and is added
	// to the paths slice there.
	paths := []string{"/actuator/workflow-engine"}
	srv, _ := newChainTestServer(t, "localhost:8543")
	srv.registerActuatorRoutes()
	srv.SetWorkflowEngineSoakSource(wiredSoakSource())
	get := func(h http.Handler, host, path string) int {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	remote := srv.remoteChain(middleware.Auth(&typedNilValidator{}), true) // validator rejects every token
	for _, p := range paths {
		if got := get(remote, "onyx.lan:8444", p); got != http.StatusUnauthorized {
			t.Errorf("remote %s without credentials = %d, want 401 (handler must not be reached)", p, got)
		}
		if got := get(srv.localChain(), "evil.example.com", p); got != http.StatusForbidden {
			t.Errorf("local %s with a foreign Host = %d, want 403 from HostGuard", p, got)
		}
		if got := get(srv.localChain(), "localhost:8543", p); got != http.StatusOK {
			t.Errorf("local %s with the real Host = %d, want 200", p, got)
		}
	}
}

func TestRegisterActuatorRoutes_ShouldServeWsUnary_WhenBridgeStoredAfterRegistration(t *testing.T) {
	s := &Server{mux: http.NewServeMux()}
	s.registerActuatorRoutes()
	s.wsUnaryBridge.Store(wsconn.NewUnaryWsBridge(func(string) http.Handler { return nil }))

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/actuator/ws-unary", nil))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ws_unary_inflight":0`) {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func getInboxPRWatch(t *testing.T, query string) *httptest.ResponseRecorder {
	t.Helper()
	srv := &Server{mux: http.NewServeMux()}
	srv.registerActuatorRoutes()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/actuator/inbox-pr-watch"+query, nil))
	return rec
}

func TestActuatorInboxPRWatch_should_ServeJSONAndTextFormat_When_Requested(t *testing.T) {
	prwatchhost.SetStatusProvider(func() prwatch.Status {
		return prwatch.Status{Enabled: true, Polls: true, InboxState: prwatch.InboxStateOK, LoginResolved: true,
			FirstRunLine: "Not watching any PRs."}
	})
	t.Cleanup(func() { prwatchhost.SetStatusProvider(nil) })

	rec := getInboxPRWatch(t, "")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("json: code=%d type=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got["enabled"] != true {
		t.Fatalf("json body = %s (err %v)", rec.Body.String(), err)
	}

	rec = getInboxPRWatch(t, "?format=text")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("text: code=%d type=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, want := range []string{"Feature flag: on", "This process polls: yes", "Not watching any PRs."} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("text missing %q in:\n%s", want, rec.Body.String())
		}
	}
}

func TestActuatorInboxPRWatch_should_AnswerFeatureFlagOffNotNotFound_When_NoProviderOrFlagOff(t *testing.T) {
	prwatchhost.SetStatusProvider(nil)

	rec := getInboxPRWatch(t, "?format=text")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("no provider text: code=%d", rec.Code)
	}
	for _, want := range []string{"Feature flag: off", "PR watch is not running in this process", "feature_flags.inbox_pr_watch"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	rec = getInboxPRWatch(t, "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"enabled":false}` {
		t.Fatalf("no provider json: code=%d body=%s", rec.Code, rec.Body.String())
	}

	prwatchhost.SetStatusProvider(func() prwatch.Status { return prwatch.Status{Enabled: false, Polls: false} })
	t.Cleanup(func() { prwatchhost.SetStatusProvider(nil) })
	rec = getInboxPRWatch(t, "?format=text")
	body = rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Feature flag: off") || !strings.Contains(body, "This process polls: no") {
		t.Fatalf("flag off live: code=%d body=%s", rec.Code, body)
	}
}

func TestRegisterActuatorRoutes_ShouldServeTerminalInput_When_RoutesRegistered(t *testing.T) {
	t.Parallel()
	s := &Server{mux: http.NewServeMux()}
	s.registerActuatorRoutes()
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/actuator/terminal-input", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, json.Valid(rec.Body.Bytes()))
}

// wiredSoakSource is the least a soak source needs for the route to answer 200.
func wiredSoakSource() *actuator.WorkflowEngineSoakSource {
	return &actuator.WorkflowEngineSoakSource{Ledger: func() (workflowengine.ShadowLedger, error) { return workflowengine.ShadowLedger{Version: 1}, nil }}
}
