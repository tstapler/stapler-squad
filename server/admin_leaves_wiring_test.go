package server

import (
	"context"
	"encoding/json"
	"go/ast"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// Serial: HOME and the state directory are process environment.
func TestNewServerWithDeps_should_WireCaptureTapHostAndDiagnosticPorts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	envtest.NewIsolatedStateDir(t)
	core, err := BuildCoreDeps()
	require.NoError(t, err)
	sessions, err := services.NewSessionServiceFromConfig(flagsvc.FlagRegistry())
	require.NoError(t, err)
	core.SessionService = sessions
	svc, err := BuildServiceDeps(core)
	require.NoError(t, err)
	rt, err := BuildRuntimeDeps(tmux.TmuxServerReady{}, svc, config.LoadConfig())
	require.NoError(t, err)
	if gate := sessions.DeliveryGate(); gate != nil {
		t.Cleanup(gate.Stop)
	}
	deps := rt.ToServerDeps()
	srv := newShutdownOnCleanupServer(t, "localhost:0", deps)

	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Host, req.RemoteAddr = "localhost:8543", "127.0.0.1:4242"
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		return rec
	}

	// Without capturetap.WithRequestHost the handler sees no Host and refuses.
	tap := post("/api/session.v1.SessionService/GetCaptureTap", "{}")
	assert.Equal(t, http.StatusOK, tap.Code, tap.Body.String())

	// An unknown item reaches storage only when a spawner is wired; nothing is spawned.
	dispatch := post("/api/session.v1.DiagnosticService/DispatchDiagnose", `{"itemId":"00000000-0000-0000-0000-000000000001"}`)
	assert.Contains(t, dispatch.Body.String(), `"code":"not_found"`, "the diagnostic service must be given the session spawner")

	ctx := context.Background()
	item, err := deps.Storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "wired", Status: string(domain.BacklogStatusReview)})
	require.NoError(t, err)
	_, err = deps.Storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: "wired-uuid", SessionRole: instance.SessionRoleDiagnose})
	require.NoError(t, err)
	live := &instance.Instance{UUID: "wired-uuid", Title: "wired-session", Program: "wired-program", Status: domain.Paused}
	deps.ReviewQueuePoller.AddInstance(live)
	t.Cleanup(func() { deps.ReviewQueuePoller.RemoveInstance(live.Title) })

	rec := post("/api/session.v1.DiagnosticService/AssembleDiagnosticBundle", `{"itemId":"`+item.ID+`"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var bundle struct {
		Prompt string `json:"prompt"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &bundle))
	assert.Contains(t, bundle.Prompt, "program=wired-program", "the diagnostic service must see the live poller")
}

// External discovery has no seam to add a session from here, so the call is pinned in source.
func TestWireDepsIntoServer_should_GiveDiagnosticServiceExternalDiscovery(t *testing.T) {
	t.Parallel()
	var args []string
	ast.Inspect(parseFunc(t, "server.go", "wireDepsIntoServer").Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "SetExternalDiscovery" {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		if !ok || recv.Name != "diagnosticSvc" {
			return true
		}
		if arg, ok := call.Args[0].(*ast.SelectorExpr); ok {
			args = append(args, arg.Sel.Name)
		}
		return true
	})
	assert.Equal(t, []string{"ExternalDiscovery"}, args)
}
